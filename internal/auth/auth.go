package auth

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
	"yep-protocol/internal/core"
	"yep-protocol/internal/storage"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	db                   *storage.DB
	mongodb              *storage.MongoDB
	otpCodes             map[string]string // phoneHash -> code
	pendingVerifications map[string]*PendingUser
	mu                   sync.Mutex
}

type PendingUser struct {
	User      *core.User
	Verified  bool
	CreatedAt time.Time
}

func NewService(db *storage.DB, mongodb *storage.MongoDB) *Service {
	return &Service{
		db:                   db,
		mongodb:              mongodb,
		otpCodes:             make(map[string]string),
		pendingVerifications: make(map[string]*PendingUser),
	}
}

func HashPhone(phone string) string {
	cleaned := strings.ReplaceAll(phone, "+", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")

	h := sha256.New()
	h.Write([]byte(cleaned + "yep-salt-2024"))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (s *Service) Register(email, phone, password, level string) (*core.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	phoneHash := ""
	if phone != "" {
		phoneHash = HashPhone(phone)
		fmt.Printf("DEBUG: Phone: %s, Hash: %s\n", phone, phoneHash) // Для отладки
	}

	user := &core.User{
		YUI:          fmt.Sprintf("yep_%d", time.Now().UnixNano()),
		Email:        email,
		Phone:        "",
		PhoneHash:    phoneHash, // Важно!
		PasswordHash: string(hash),
		Level:        level,
		IsActive:     false,
	}

	if err := s.db.CreateUser(user); err != nil {
		return nil, err
	}

	s.pendingVerifications[user.YUI] = &PendingUser{
		User:      user,
		Verified:  false,
		CreatedAt: time.Now(),
	}

	return user, nil
}

func (s *Service) Login(email, password string) (*core.User, error) {
	user, err := s.db.GetUserByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	s.db.UpdateLastLogin(user.YUI)
	return user, nil
}

// Сохраняем OTP по phone_hash
func (s *Service) CreatePendingUser(email, phone string) (string, error) {
	phoneHash := HashPhone(phone)
	yui := fmt.Sprintf("yep_%d", time.Now().UnixNano())

	s.mu.Lock()
	s.pendingVerifications[yui] = &PendingUser{
		User: &core.User{
			YUI:       yui,
			Email:     email,
			PhoneHash: phoneHash,
			IsActive:  false,
		},
		Verified:  false,
		CreatedAt: time.Now(),
	}
	s.mu.Unlock()

	return yui, nil
}

// Сохраняем OTP по phone_hash
func (s *Service) StoreOTP(phoneHash, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.otpCodes[phoneHash] = code
}

// Верификация OTP
func (s *Service) VerifyOTP(phoneHash, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.otpCodes[phoneHash]
	if !ok || stored != code {
		return fmt.Errorf("invalid code")
	}

	delete(s.otpCodes, phoneHash)

	// Активируем пользователя
	for _, pending := range s.pendingVerifications {
		if pending.User.PhoneHash == phoneHash {
			pending.User.IsActive = true
			s.db.CreateUser(pending.User) // сохраняем в БД
			delete(s.pendingVerifications, pending.User.YUI)
			return nil
		}
	}

	return fmt.Errorf("user not found")
}

// Проверяем OTP по phone_hash
func (s *Service) VerifyCodeByPhoneHash(phoneHash, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.otpCodes[phoneHash]
	if !ok {
		return fmt.Errorf("no code found")
	}

	if stored != code {
		return fmt.Errorf("invalid code")
	}

	// Удаляем использованный код
	delete(s.otpCodes, phoneHash)

	// Активируем пользователя
	return s.db.ActivateUserByPhoneHash(phoneHash)
}
