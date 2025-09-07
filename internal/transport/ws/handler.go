package ws

import (
	"fmt"
	"log"
	"net/http"
	"sync"
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
	mu       sync.RWMutex // Добавим mutex для безопасной работы с clients
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

	// Ждём авторизацию или токен
	var authMsg map[string]interface{}
	if err := conn.ReadJSON(&authMsg); err != nil {
		conn.WriteJSON(core.YepMessage{
			Type:    "ERROR",
			Content: "Authentication required",
		})
		return
	}

	var user *core.User
	needsVerification := false

	// Проверяем токен сначала
	if token, ok := authMsg["token"].(string); ok && token != "" {
		// Авторизация по токену
		claims, err := auth.ValidateToken(token)
		if err == nil {
			// Токен валидный, получаем пользователя
			user, err = h.db.GetUserByEmail(claims.Email)
			if err == nil && user.IsActive {
				// Успешная авторизация по токену
				h.addClient(user, conn)
				return
			}
		}
		// Если токен невалидный, продолжаем обычную авторизацию
		conn.WriteJSON(core.YepMessage{
			Type:    "TOKEN_EXPIRED",
			Content: "Token expired, please login again",
		})
		return
	}

	// Обычная авторизация
	email, _ := authMsg["email"].(string)
	password, _ := authMsg["password"].(string)
	phone, _ := authMsg["phone"].(string)
	level, _ := authMsg["level"].(string)
	isLogin, _ := authMsg["is_login"].(bool)

	if !isLogin {
		// Регистрация
		user, err = h.auth.Register(email, phone, password, level)
		if err != nil {
			conn.WriteJSON(core.YepMessage{
				Type:    "ERROR",
				Content: err.Error(),
			})
			return
		}

		if !user.IsActive {
			needsVerification = true
		}
	} else {
		// Логин
		user, err = h.auth.Login(email, password)
		if err != nil {
			conn.WriteJSON(core.YepMessage{
				Type:    "ERROR",
				Content: "Login failed: " + err.Error(),
			})
			return
		}

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
		h.addClient(user, conn)
	}
}

func (h *Handler) addClient(user *core.User, conn *websocket.Conn) {
	// Генерируем JWT токен
	token, err := auth.GenerateToken(user.YUI, user.Email, user.Level)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		token = "" // Продолжаем без токена
	}

	// Отправляем успешную авторизацию с токеном
	conn.WriteJSON(core.YepMessage{
		Type:      "AUTH_SUCCESS",
		YUI:       user.YUI,
		Level:     user.Level,
		Content:   fmt.Sprintf("Welcome to YEP! Level: %s", user.Level),
		Token:     token,
		Timestamp: time.Now().Unix(),
	})

	client := &Client{
		conn:     conn,
		user:     user,
		verified: true,
	}

	// Добавляем клиента в список
	h.mu.Lock()
	h.clients[user.YUI] = client
	clientCount := len(h.clients)
	h.mu.Unlock()

	// Уведомляем всех о входе
	h.broadcast(core.YepMessage{
		Type:      "USER_JOIN",
		Content:   fmt.Sprintf("%s joined the chat", user.Email),
		YUI:       "SYSTEM",
		Level:     "S",
		Timestamp: time.Now().Unix(),
	}, "")

	// Отправляем список онлайн пользователей новому клиенту
	h.sendOnlineUsers(client)

	log.Printf("[JOIN] %s (%s) - Total online: %d", user.YUI, user.Email, clientCount)

	// Обрабатываем сообщения
	h.handleMessages(client)
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

			// Переходим к обычной авторизации
			h.addClient(client.user, client.conn)
			return
		}
	}
}

func (h *Handler) handleMessages(client *Client) {
	defer func() {
		// При выходе
		h.mu.Lock()
		delete(h.clients, client.user.YUI)
		clientCount := len(h.clients)
		h.mu.Unlock()

		// Уведомляем всех о выходе
		h.broadcast(core.YepMessage{
			Type:      "USER_LEAVE",
			Content:   fmt.Sprintf("%s left the chat", client.user.Email),
			YUI:       "SYSTEM",
			Level:     "S",
			Timestamp: time.Now().Unix(),
		}, "")

		log.Printf("[LEAVE] %s - Total online: %d", client.user.YUI, clientCount)
		client.conn.Close()
	}()

	for {
		var msg core.YepMessage
		if err := client.conn.ReadJSON(&msg); err != nil {
			break
		}

		// Обработка разных типов сообщений
		switch msg.Type {
		case "MESSAGE":
			h.handleChatMessage(client, msg)
		case "PING":
			// Отвечаем на пинг для поддержания соединения
			client.conn.WriteJSON(core.YepMessage{
				Type:      "PONG",
				Timestamp: time.Now().Unix(),
			})
		default:
			// Неизвестный тип сообщения
		}
	}
}

func (h *Handler) handleChatMessage(client *Client, msg core.YepMessage) {
	msg.YUI = client.user.YUI
	msg.Level = client.user.Level
	msg.Timestamp = time.Now().Unix()

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

	// Отправляем всем КРОМЕ отправителя
	h.broadcast(response, client.user.YUI)
}

func (h *Handler) processMessage(msg core.YepMessage, user *core.User) core.YepMessage {
	// Форматируем сообщение с префиксом
	prefix := fmt.Sprintf("[%s | Level %s]", user.Email, user.Level)

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

func (h *Handler) broadcast(msg core.YepMessage, excludeYUI string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for yui, client := range h.clients {
		// Пропускаем отправителя если указан
		if excludeYUI != "" && yui == excludeYUI {
			continue
		}

		if err := client.conn.WriteJSON(msg); err != nil {
			log.Printf("Error sending to %s: %v", yui, err)
			// Не удаляем клиента здесь, так как это может вызвать deadlock
		}
	}
}

func (h *Handler) sendOnlineUsers(client *Client) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var users []string
	for _, c := range h.clients {
		users = append(users, c.user.Email)
	}

	client.conn.WriteJSON(core.YepMessage{
		Type:      "ONLINE_USERS",
		Content:   fmt.Sprintf("Online: %d users", len(users)),
		Data:      users,
		Timestamp: time.Now().Unix(),
	})
}
