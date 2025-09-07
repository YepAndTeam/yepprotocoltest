package ws

import (
	"fmt"
	"log"
	"net/http"
	"time"
	"yep-protocol/internal/auth"
	"yep-protocol/internal/core"
	"yep-protocol/internal/storage"

	"github.com/gorilla/websocket"
)

type Handler struct {
	auth     *auth.Service
	db       *storage.DB
	mongodb  *storage.MongoDB
	upgrader websocket.Upgrader
	clients  map[string]*Client
}

type Client struct {
	conn     *websocket.Conn
	user     *core.User
	verified bool
}

func NewHandler(authService *auth.Service, db *storage.DB, mongodb *storage.MongoDB) *Handler {
	return &Handler{
		auth:    authService,
		db:      db,
		mongodb: mongodb,
		clients: make(map[string]*Client),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Ждём авторизацию
	var authMsg core.YepAuth
	if err := conn.ReadJSON(&authMsg); err != nil {
		conn.WriteJSON(core.YepMessage{
			Type:    "ERROR",
			Content: "Authentication required",
		})
		return
	}

	var user *core.User
	needsVerification := false

	if !authMsg.IsLogin {
		fmt.Printf("DEBUG: Registration attempt - Email: %s, Phone: %s\n", authMsg.Email, authMsg.Phone)

		// При регистрации используем обычный phone, не hash
		user, err = h.auth.Register(authMsg.Email, authMsg.Phone, authMsg.Password, authMsg.Level)
		if err != nil {
			fmt.Printf("DEBUG: Registration error: %v\n", err)
			conn.WriteJSON(core.YepMessage{
				Type:    "ERROR",
				Content: err.Error(),
			})
			return
		}
		fmt.Printf("DEBUG: User created - YUI: %s, PhoneHash: %s\n", user.YUI, user.PhoneHash)

		// Проверяем активирован ли пользователь
		if !user.IsActive {
			needsVerification = true
		}
	} else {
		// Login: determine identifier (prefer email if provided, else phone)
		identifier := authMsg.Email
		if identifier == "" {
			identifier = authMsg.Phone
		}
		if identifier == "" {
			conn.WriteJSON(core.YepMessage{
				Type:    "ERROR",
				Content: "Email or phone required for login",
			})
			return
		}

		user, err = h.auth.Login(identifier, authMsg.Password)
		if err != nil {
			fmt.Printf("DEBUG: Login error: %v\n", err)
			conn.WriteJSON(core.YepMessage{
				Type:    "ERROR",
				Content: "Login failed: " + err.Error(),
			})
			return
		}

		// Проверяем активирован ли пользователь
		if !user.IsActive {
			needsVerification = true
		}
	}

	client := &Client{
		conn:     conn,
		user:     user,
		verified: !needsVerification,
	}

	if needsVerification {
		// Требуем верификацию
		conn.WriteJSON(core.YepMessage{
			Type:    "VERIFICATION_REQUIRED",
			Content: "Please verify your phone via @YEPVerifyBot on Telegram",
			YUI:     user.YUI,
		})

		// Ждём OTP
		h.waitForOTP(client)
	} else {
		// Сразу успех если уже активирован
		conn.WriteJSON(core.YepMessage{
			Type:      "AUTH_SUCCESS",
			YUI:       user.YUI,
			Level:     user.Level,
			Content:   fmt.Sprintf("Welcome to YEP! Level: %s", user.Level),
			Timestamp: time.Now().Unix(),
		})

		h.clients[user.YUI] = client
		h.handleMessages(client)
	}
}

func (h *Handler) waitForOTP(client *Client) {
	for {
		var msg map[string]interface{}
		if err := client.conn.ReadJSON(&msg); err != nil {
			log.Printf("Error reading OTP: %v", err)
			return
		}

		if msg["type"] == "OTP_VERIFY" {
			code, ok := msg["code"].(string)
			if !ok {
				client.conn.WriteJSON(core.YepMessage{
					Type:    "ERROR",
					Content: "Invalid code format",
				})
				continue
			}

			// ИСПРАВЛЕНО: Используем PhoneHash из базы данных, а не хешируем Phone
			if err := h.auth.VerifyCodeByPhoneHash(client.user.PhoneHash, code); err != nil {
				client.conn.WriteJSON(core.YepMessage{
					Type:    "ERROR",
					Content: "Invalid verification code",
				})
				continue
			}

			// Успешная верификация
			client.verified = true
			client.user.IsActive = true

			client.conn.WriteJSON(core.YepMessage{
				Type:      "AUTH_SUCCESS",
				YUI:       client.user.YUI,
				Level:     client.user.Level,
				Content:   fmt.Sprintf("Verification successful! Welcome to YEP! Level: %s", client.user.Level),
				Timestamp: time.Now().Unix(),
			})

			h.clients[client.user.YUI] = client
			h.handleMessages(client)
			return
		}
	}
}

func (h *Handler) handleMessages(client *Client) {
	log.Printf("[JOIN] %s (%s)", client.user.YUI, client.user.Email)

	for {
		var msg core.YepMessage
		if err := client.conn.ReadJSON(&msg); err != nil {
			log.Printf("[LEAVE] %s", client.user.YUI)
			delete(h.clients, client.user.YUI)
			break
		}

		msg.YUI = client.user.YUI
		msg.Level = client.user.Level
		msg.Timestamp = time.Now().Unix()

		if msg.Type == "MESSAGE" {
			// Сохраняем в MongoDB
			mongoMsg := &storage.MongoMessage{
				FromYUI:   client.user.YUI,
				Content:   msg.Content,
				Level:     client.user.Level,
				Encrypted: false,
				IsRead:    false,
			}

			if err := h.mongodb.SaveMessage(mongoMsg); err != nil {
				log.Printf("Failed to save message: %v", err)
			}

			// Готовим ответ
			response := h.processMessage(msg, client.user)

			// Рассылаем всем клиентам
			for _, c := range h.clients {
				if err := c.conn.WriteJSON(response); err != nil {
					log.Printf("Error sending to %s: %v", c.user.YUI, err)
					c.conn.Close()
					delete(h.clients, c.user.YUI)
				}
			}
		}
	}
}

func (h *Handler) processMessage(msg core.YepMessage, user *core.User) core.YepMessage {
	prefix := fmt.Sprintf("[%s | Level %s]", user.YUI, user.Level)

	// Ограничения по уровню
	if user.Level == "C" && len(msg.Content) > 100 {
		return core.YepMessage{
			Type:    "ERROR",
			Content: "Level C: max 100 chars",
		}
	}

	return core.YepMessage{
		Type:      "MESSAGE",
		Content:   fmt.Sprintf("%s %s", prefix, msg.Content),
		YUI:       user.YUI,
		Level:     user.Level,
		Timestamp: time.Now().Unix(),
	}
}
