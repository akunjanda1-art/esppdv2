package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func GetString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func MustString(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required env var %s", key)
	}
	return v, nil
}

func GetInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func GetInt64(key string, def int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return def
}

func GetBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func GetDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}

type Common struct {
	Env              string
	LogLevel         string
	ServiceName      string
	Port             int
	ShutdownTimeout  time.Duration
	RateLimitPerMin  int
	AllowedIPRanges  []string
	TrustedProxies   []string
	EnableHTTPTiming bool
}

func LoadCommon(serviceName string, portEnv string, defaultPort int) Common {
	return Common{
		Env:             GetString("ENV", "development"),
		LogLevel:        GetString("LOG_LEVEL", "info"),
		ServiceName:     serviceName,
		Port:            GetInt(portEnv, defaultPort),
		ShutdownTimeout: GetDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		RateLimitPerMin: GetInt("RATE_LIMIT_PER_MIN", 120),
	}
}

type Postgres struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
	SSLMode  string
	MaxConns int32
}

func LoadPostgres() Postgres {
	return Postgres{
		Host:     GetString("POSTGRES_HOST", "localhost"),
		Port:     GetInt("POSTGRES_PORT", 5432),
		DB:       GetString("POSTGRES_DB", "esppd"),
		User:     GetString("POSTGRES_USER", "esppd"),
		Password: GetString("POSTGRES_PASSWORD", ""),
		SSLMode:  GetString("POSTGRES_SSLMODE", "disable"),
		MaxConns: int32(GetInt("POSTGRES_MAX_CONNS", 20)),
	}
}

func (p Postgres) DSN() string {
	// pgx supports the URL format.
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s", p.User, p.Password, p.Host, p.Port, p.DB, p.SSLMode)
}

type Redis struct {
	Addr     string
	Password string
	DB       int
}

func LoadRedis() Redis {
	return Redis{
		Addr:     GetString("REDIS_ADDR", "localhost:6379"),
		Password: GetString("REDIS_PASSWORD", ""),
		DB:       GetInt("REDIS_DB", 0),
	}
}

type NATS struct {
	URL    string
	Stream string
}

func LoadNATS() NATS {
	return NATS{
		URL:    GetString("NATS_URL", "nats://localhost:4222"),
		Stream: GetString("NATS_STREAM", "esppd"),
	}
}

type MinIO struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

func LoadMinIO() MinIO {
	return MinIO{
		Endpoint:  GetString("MINIO_ENDPOINT", "localhost:9000"),
		AccessKey: GetString("MINIO_ACCESS_KEY", "minioadmin"),
		SecretKey: GetString("MINIO_SECRET_KEY", "minioadmin"),
		Bucket:    GetString("MINIO_BUCKET", "esppd-docs"),
		UseSSL:    GetBool("MINIO_USE_SSL", false),
	}
}

type JWT struct {
	Issuer       string
	Audience     string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	PrivateKey   string
	PublicKey    string
	PrivateKeyPassphrase string
	RequiredKID  string
	EnableKeyGen bool
}

func LoadJWT() JWT {
	return JWT{
		Issuer:     GetString("JWT_ISSUER", "esppd"),
		Audience:   GetString("JWT_AUDIENCE", "esppd-users"),
		AccessTTL:  GetDuration("JWT_ACCESS_TTL", 15*time.Minute),
		RefreshTTL: GetDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		PrivateKey: GetString("JWT_PRIVATE_KEY", "/secrets/jwt_private.pem"),
		PublicKey:  GetString("JWT_PUBLIC_KEY", "/secrets/jwt_public.pem"),
		PrivateKeyPassphrase: GetString("JWT_PRIVATE_KEY_PASSPHRASE", ""),
	}
}

type Crypto struct {
	DataKeyBase64 string
}

func LoadCrypto() Crypto {
	return Crypto{
		DataKeyBase64: GetString("DATA_ENC_KEY_BASE64", ""),
	}
}
