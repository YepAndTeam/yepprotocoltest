package chat

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var clients = make(map[*websocket.Conn]bool)
var broadcast = make(chan string)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // временно разрешаем всех
}

func HandleChat(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade error: %v", err)
		return
	}
	defer ws.Close()

	clients[ws] = true
	log.Println("✅ New chat client connected")

	for {
		var msg string
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("❌ Error reading message: %v", err)
			delete(clients, ws)
			break
		}
		broadcast <- msg
	}
}

func StartBroadcaster() {
	for {
		msg := <-broadcast
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("❌ Error writing to client: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}
