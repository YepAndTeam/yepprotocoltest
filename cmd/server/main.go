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
	// Загружаем конфиг из переменных окружения
	cfg := config.Load()

	// Проверяем, что переменные окружения подхватились
	fmt.Println("🔹 DATABASE_URL:", cfg.DBConn)
	fmt.Println("🔹 MONGO_URI:", cfg.MongoURI)

	// Подключение к PostgreSQL
	db, err := storage.NewDB(cfg.DBConn)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer db.Close()

	// Подключение к MongoDB
	mongodb, err := storage.NewMongoDB(cfg.MongoURI)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer mongodb.Close()

	// Сервисы
	authService := auth.NewService(db, mongodb)
	telegramHandler := auth.NewTelegramVerifyHandler(db, authService)

	// WebSocket handler
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

	// Запуск сервера
	addr := fmt.Sprintf(":%s", cfg.Port)
	fmt.Printf("🚀 YEP Protocol v0.3\n")
	fmt.Printf("📡 WebSocket: ws://localhost%s/ws\n", addr)
	fmt.Printf("🌐 Test page: http://localhost%s/\n", addr)
	fmt.Printf("💾 MongoDB: Connected for messages\n")
	fmt.Printf("🐘 PostgreSQL: Connected for users\n")

	log.Fatal(http.ListenAndServe(addr, nil))
}

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
