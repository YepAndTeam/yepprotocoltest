package config

import "os"

type Config struct {
	Port     string
	DBConn   string
	MongoURI string // Добавь это
	LogLevel string
}

func Load() *Config {
	return &Config{
		Port:     getEnv("PORT", "8080"),
		DBConn:   getEnv("DATABASE_URL", "user=postgres password=Beka2001 dbname=yep_hub host=localhost sslmode=disable"),
		MongoURI: getEnv("MONGO_URI", "mongodb://localhost:27017"), // Добавь это
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
