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
		DBConn:   getEnv("DATABASE_URL", ""), // пусто по умолчанию, чтобы не использовать localhost на Railway
		MongoURI: getEnv("MONGO_URL", ""),    // пусто по умолчанию
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
