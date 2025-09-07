package storage

import (
	"database/sql"
	"fmt"
	"time"

	"yep-protocol/internal/core"

	_ "github.com/lib/pq"
)

type DB struct {
	conn *sql.DB
}

func NewDB(connStr string) (*DB, error) {
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	if err = conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err = db.createTables(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) createTables() error {
	query := `
    CREATE TABLE IF NOT EXISTS users (
        yui VARCHAR(50) PRIMARY KEY,
        email VARCHAR(255) UNIQUE NOT NULL,
        phone VARCHAR(20),
        phone_hash VARCHAR(64), -- добавь это, чтобы искать по хешу
        password_hash VARCHAR(255) NOT NULL,
        level CHAR(1) DEFAULT 'C',
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        last_login TIMESTAMP,
        is_active BOOLEAN DEFAULT true
    );

    CREATE TABLE IF NOT EXISTS messages (
        id SERIAL PRIMARY KEY,
        from_yui VARCHAR(50),
        to_yui VARCHAR(50),
        content TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS otp_codes (
        phone_hash VARCHAR(64) PRIMARY KEY,
        code VARCHAR(6) NOT NULL,
        expires_at TIMESTAMP NOT NULL,
        telegram_id BIGINT
    );
    `
	_, err := db.conn.Exec(query)
	return err
}

func (db *DB) CreateUser(user *core.User) error {
	query := `
        INSERT INTO users (yui, email, phone, phone_hash, password_hash, level, is_active)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING created_at`

	return db.conn.QueryRow(
		query,
		user.YUI, user.Email, user.Phone, user.PhoneHash,
		user.PasswordHash, user.Level, user.IsActive,
	).Scan(&user.CreatedAt)
}
func (db *DB) GetUserByEmail(email string) (*core.User, error) {
	user := &core.User{}
	query := `
        SELECT yui, email, phone, password_hash, level, created_at, last_login, is_active
        FROM users
        WHERE email = $1 AND is_active = true`

	err := db.conn.QueryRow(query, email).Scan(
		&user.YUI, &user.Email, &user.Phone,
		&user.PasswordHash, &user.Level,
		&user.CreatedAt, &user.LastLogin, &user.IsActive,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}

	return user, err
}

func (db *DB) UpdateLastLogin(yui string) error {
	_, err := db.conn.Exec(
		"UPDATE users SET last_login = $1 WHERE yui = $2",
		time.Now(), yui,
	)
	return err
}

func (db *DB) SaveMessage(fromYUI, content string) error {
	_, err := db.conn.Exec(
		"INSERT INTO messages (from_yui, content) VALUES ($1, $2)",
		fromYUI, content,
	)
	return err
}

func (db *DB) Close() error {
	return db.conn.Close()
}
func (db *DB) GetUserByPhoneHash(phoneHash string) (*core.User, error) {
	user := &core.User{}
	query := `
        SELECT yui, email, phone, level, created_at
        FROM users
        WHERE phone_hash = $1 AND is_active = true`

	err := db.conn.QueryRow(query, phoneHash).Scan(
		&user.YUI, &user.Email, &user.Phone,
		&user.Level, &user.CreatedAt,
	)

	return user, err
}
func (db *DB) SaveOTP(phoneHash, code string, telegramID int64) error {
	_, err := db.conn.Exec(`
        INSERT INTO otp_codes (phone_hash, code, expires_at, telegram_id)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (phone_hash) DO UPDATE
        SET code = $2, expires_at = $3, telegram_id = $4
    `, phoneHash, code, time.Now().Add(5*time.Minute), telegramID)
	return err
}

// CheckOTPCode проверяет, совпадает ли код и не истёк ли он
func (db *DB) CheckOTPCode(phoneHash, code string) bool {
	var storedCode string
	var expiresAt time.Time

	err := db.conn.QueryRow(
		"SELECT code, expires_at FROM otp_codes WHERE phone_hash = $1",
		phoneHash,
	).Scan(&storedCode, &expiresAt)

	if err != nil {
		return false
	}

	if time.Now().After(expiresAt) {
		return false
	}

	return storedCode == code
}
func (db *DB) ActivateUserByPhoneHash(phoneHash string) error {
	_, err := db.conn.Exec(
		"UPDATE users SET is_active = true WHERE phone_hash = $1",
		phoneHash,
	)
	return err
}
