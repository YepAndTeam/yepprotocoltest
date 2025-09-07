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
	Type      string `json:"type"`
	YUI       string `json:"yui"`
	Level     string `json:"level"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

type YepAuth struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
	Level    string `json:"level"`
	IsLogin  bool   `json:"is_login"`
}
