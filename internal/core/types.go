package core

import (
	"database/sql"
	"time"
)

type User struct {
	YUI          string
	Email        string
	Phone        string
	PhoneHash    string // Добавь это!
	PasswordHash string
	Level        string
	CreatedAt    time.Time
	LastLogin    sql.NullTime
	IsActive     bool
}

type YepMessage struct {
	Type      string      `json:"type"`
	Content   string      `json:"content"`
	YUI       string      `json:"yui,omitempty"`
	Level     string      `json:"level,omitempty"`
	Token     string      `json:"token,omitempty"` // Добавь это
	Data      interface{} `json:"data,omitempty"`  // Добавь это
	Timestamp int64       `json:"timestamp"`
}

type YepAuth struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
	Level    string `json:"level"`
	IsLogin  bool   `json:"is_login"`
}
