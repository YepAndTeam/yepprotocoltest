package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"yep-protocol/internal/auth"
	"yep-protocol/internal/config"
	"yep-protocol/internal/storage"
	"yep-protocol/internal/transport/ws"
)

func main() {
	// Загружаем конфиг
	cfg := config.Load()

	fmt.Println("🔹 DATABASE_URL:", cfg.DBConn)
	fmt.Println("🔹 MONGO_URI:", cfg.MongoURI)

	// PostgreSQL
	db, err := storage.NewDB(cfg.DBConn)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer db.Close()

	// MongoDB
	mongodb, err := storage.NewMongoDB(cfg.MongoURI)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer mongodb.Close()

	// Сервисы
	authService := auth.NewService(db, mongodb)
	telegramHandler := auth.NewTelegramVerifyHandler(db, authService)

	// WS handler
	wsHandler := ws.NewHandler(authService, db, mongodb)

	// HTTP роуты
	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/ws", wsHandler.HandleWebSocket)
	http.HandleFunc("/api/telegram/save-code", telegramHandler.HandleSaveCode)
	http.HandleFunc("/api/telegram/check", telegramHandler.HandleTelegramCheck)

	// API для истории сообщений
	http.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		yui := r.URL.Query().Get("yui")
		messages, err := mongodb.GetMessageHistory(yui, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(messages)
	})

	// Определяем порт
	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	addr := fmt.Sprintf(":%s", port)

	fmt.Printf("🚀 YEP Protocol v0.3\n")
	fmt.Printf("🌐 Server listening on %s\n", addr)

	log.Fatal(http.ListenAndServe(addr, nil))
}

// serveHTML отдает статический фронт
func serveHTML(w http.ResponseWriter, r *http.Request) {
	html, err := os.ReadFile("web/index.html")
	if err != nil {
		log.Printf("Error reading HTML file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}
