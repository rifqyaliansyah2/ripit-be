package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort            string
	ServerHost            string
	AppEnv                string
	DBHost                string
	DBPort                string
	DBUser                string
	DBPassword            string
	DBName                string
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	DBConnMaxLifetime     time.Duration
	JWTSecret             string
	JWTExpirationHours    int
	CORSAllowedOrigins    []string
	RateLimitReqPerSecond float64
	RateLimitBurst        int
	YouTubeAPIKey         string
}

func LoadConfig() *Config {
	// Attempt loading .env file; if it doesn't exist, proceed with environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("[Config] No .env file found or error reading it, using system environment variables")
	}

	return &Config{
		ServerPort:            getEnv("SERVER_PORT", "8080"),
		ServerHost:            getEnv("SERVER_HOST", "0.0.0.0"),
		AppEnv:                getEnv("APP_ENV", "development"),
		DBHost:                getEnv("DB_HOST", "127.0.0.1"),
		DBPort:                getEnv("DB_PORT", "3306"),
		DBUser:                getEnv("DB_USER", "root"),
		DBPassword:            getEnv("DB_PASSWORD", ""),
		DBName:                getEnv("DB_NAME", "ripit_db"),
		DBMaxOpenConns:        getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:        getEnvAsInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetime:     getEnvAsDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		JWTSecret:             getEnv("JWT_SECRET", "super_secret_jwt_key_change_in_production"),
		JWTExpirationHours:    getEnvAsInt("JWT_EXPIRATION_HOURS", 72),
		CORSAllowedOrigins:    getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:8080"}),
		RateLimitReqPerSecond: getEnvAsFloat("RATE_LIMIT_REQUESTS_PER_SECOND", 20.0),
		RateLimitBurst:        getEnvAsInt("RATE_LIMIT_BURST", 40),
		YouTubeAPIKey:         getEnv("YOUTUBE_API_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}

func getEnvAsFloat(key string, fallback float64) float64 {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
		return value
	}
	return fallback
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	valueStr := getEnv(key, "")
	if value, err := time.ParseDuration(valueStr); err == nil {
		return value
	}
	return fallback
}

func getEnvAsSlice(key string, fallback []string) []string {
	valueStr := getEnv(key, "")
	if valueStr == "" {
		return fallback
	}
	parts := strings.Split(valueStr, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}
