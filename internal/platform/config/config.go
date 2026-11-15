package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all validated configuration parameters for BioVerse services.
type Config struct {
	App      AppConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Storage      StorageConfig
	Payment      PaymentConfig
	Email        EmailConfig
	Notification NotificationConfig
	OAuth        OAuthConfig
}

// AppConfig defines HTTP server and general application settings.
type AppConfig struct {
	Name           string
	Environment    string
	Port           int
	LogLevel       string
	AllowedOrigins []string
}

// DatabaseConfig defines PostgreSQL connection parameters.
type DatabaseConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// AuthConfig defines JWT, token expiration, and cryptographic secrets.
type AuthConfig struct {
	JWTSecret        string
	AccessExpiry     time.Duration
	RefreshExpiry    time.Duration
	AppEncryptionKey string
	HMACSecret       string
}

// StorageConfig defines S3-compatible object storage settings.
type StorageConfig struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	BucketName string
	Region     string
	UseSSL     bool
}

// PaymentConfig defines payment gateway parameters.
type PaymentConfig struct {
	XenditSecretKey    string
	XenditWebhookToken string
}

// EmailConfig defines transactional email settings.
type EmailConfig struct {
	ResendAPIKey string
	FromEmail    string
}

// NotificationConfig defines push notification credentials.
type NotificationConfig struct {
	FCMCredentialsPath string
}

// OAuthConfig defines OAuth integration credentials.
type OAuthConfig struct {
	GoogleClientID string
}

// Load reads configuration from environment variables and an optional .env file.
func Load() (*Config, error) {
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	port, err := getEnvAsInt("PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}

	maxConns, err := getEnvAsInt("DB_MAX_CONNS", 25)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MAX_CONNS: %w", err)
	}

	minConns, err := getEnvAsInt("DB_MIN_CONNS", 5)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MIN_CONNS: %w", err)
	}

	maxLifetime, err := getEnvAsDuration("DB_MAX_CONN_LIFETIME", time.Hour)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MAX_CONN_LIFETIME: %w", err)
	}

	maxIdleTime, err := getEnvAsDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MAX_CONN_IDLE_TIME: %w", err)
	}

	accessExpiry, err := getEnvAsDuration("JWT_ACCESS_EXPIRY", 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_ACCESS_EXPIRY: %w", err)
	}

	refreshExpiry, err := getEnvAsDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_REFRESH_EXPIRY: %w", err)
	}

	storageSSL, err := getEnvAsBool("STORAGE_USE_SSL", false)
	if err != nil {
		return nil, fmt.Errorf("invalid STORAGE_USE_SSL: %w", err)
	}

	originsRaw := getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173,http://localhost:8080")
	origins := strings.Split(originsRaw, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}

	cfg := &Config{
		App: AppConfig{
			Name:           getEnv("APP_NAME", "BioVerse"),
			Environment:    getEnv("APP_ENV", "development"),
			Port:           port,
			LogLevel:       getEnv("LOG_LEVEL", "debug"),
			AllowedOrigins: origins,
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", "postgres://bioverse:bioverse_dev@127.0.0.1:5432/bioverse?sslmode=disable"),
			MaxConns:        int32(maxConns),
			MinConns:        int32(minConns),
			MaxConnLifetime: maxLifetime,
			MaxConnIdleTime: maxIdleTime,
		},
		Auth: AuthConfig{
			JWTSecret:        getEnv("JWT_SECRET", "super_secret_key_bioverse_2026_change_in_production"),
			AccessExpiry:     accessExpiry,
			RefreshExpiry:    refreshExpiry,
			AppEncryptionKey: getEnv("APP_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
			HMACSecret:       getEnv("HMAC_SECRET", "hmac_secret_key_for_searchable_hashes"),
		},
		Storage: StorageConfig{
			Endpoint:   getEnv("STORAGE_ENDPOINT", "http://127.0.0.1:9000"),
			AccessKey:  getEnv("STORAGE_ACCESS_KEY", "bioverse_minio"),
			SecretKey:  getEnv("STORAGE_SECRET_KEY", "bioverse_minio_secret"),
			BucketName: getEnv("STORAGE_BUCKET_NAME", "bioverse-media"),
			Region:     getEnv("STORAGE_REGION", "us-east-1"),
			UseSSL:     storageSSL,
		},
		Payment: PaymentConfig{
			XenditSecretKey:    getEnv("XENDIT_SECRET_KEY", ""),
			XenditWebhookToken: getEnv("XENDIT_WEBHOOK_TOKEN", ""),
		},
		Email: EmailConfig{
			ResendAPIKey: getEnv("RESEND_API_KEY", ""),
			FromEmail:    getEnv("EMAIL_FROM", "onboarding@resend.dev"),
		},
		Notification: NotificationConfig{
			FCMCredentialsPath: strings.Trim(getEnv("FCM_CREDENTIALS_JSON_PATH", ""), "\""),
		},
		OAuth: OAuthConfig{
			GoogleClientID: getEnv("GOOGLE_CLIENT_ID", ""),
		},
	}

	if cfg.Database.URL == "" {
		return nil, fmt.Errorf("DATABASE_URL must not be empty")
	}

	// In staging and production environments, enforce strict cryptographic secrets
	if cfg.App.Environment == "production" || cfg.App.Environment == "staging" {
		if cfg.Auth.JWTSecret == "super_secret_key_bioverse_2026_change_in_production" || len(cfg.Auth.JWTSecret) < 32 {
			return nil, fmt.Errorf("insecure or default JWT_SECRET configured for %s environment", cfg.App.Environment)
		}
		if cfg.Auth.AppEncryptionKey == "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" || len(cfg.Auth.AppEncryptionKey) != 64 {
			return nil, fmt.Errorf("insecure or default APP_ENCRYPTION_KEY configured for %s environment", cfg.App.Environment)
		}
		if cfg.Auth.HMACSecret == "hmac_secret_key_for_searchable_hashes" || len(cfg.Auth.HMACSecret) < 16 {
			return nil, fmt.Errorf("insecure or default HMAC_SECRET configured for %s environment", cfg.App.Environment)
		}
		if cfg.Payment.XenditSecretKey == "xnd_development_placeholder" {
			return nil, fmt.Errorf("insecure default XENDIT_SECRET_KEY configured for %s environment", cfg.App.Environment)
		}
		if cfg.Payment.XenditWebhookToken == "webhook_verification_token_placeholder" {
			return nil, fmt.Errorf("insecure default XENDIT_WEBHOOK_TOKEN configured for %s environment", cfg.App.Environment)
		}
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) (int, error) {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultVal, nil
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, fmt.Errorf("key %q value %q is not integer: %w", key, valStr, err)
	}
	return val, nil
}

func getEnvAsDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultVal, nil
	}
	val, err := time.ParseDuration(valStr)
	if err != nil {
		return 0, fmt.Errorf("key %q value %q is not valid duration: %w", key, valStr, err)
	}
	return val, nil
}

func getEnvAsBool(key string, defaultVal bool) (bool, error) {
	valStr := getEnv(key, "")
	if valStr == "" {
		return defaultVal, nil
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return false, fmt.Errorf("key %q value %q is not valid boolean: %w", key, valStr, err)
	}
	return val, nil
}
