package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

// Секретный ключ (потом в ENV переменную!)
var jwtSecret = []byte("yep-secret-key-change-this-in-production")

type TokenClaims struct {
	YUI   string `json:"yui"`
	Email string `json:"email"`
	Level string `json:"level"`
	jwt.RegisteredClaims
}

// Создаём токен
func GenerateToken(yui, email, level string) (string, error) {
	claims := TokenClaims{
		YUI:   yui,
		Email: email,
		Level: level,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "yep-protocol",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// Проверяем токен
func ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// Refresh token (живёт дольше)
func GenerateRefreshToken(yui string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   yui,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)), // 7 дней
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "yep-protocol-refresh",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}
