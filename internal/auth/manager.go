package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const (
	issuer      = "ais"
	tokenTTL    = 15 * time.Minute
	redisPrefix = "ais:auth:session:"
)

type Session struct {
	Service    string `json:"service"`
	Subject    string `json:"subject"`
	Strategy   string `json:"strategy"`
	Credential string `json:"credential"`
}

type Claims struct {
	Service string `json:"svc"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	redis  *redis.Client
}

func NewManager(secret, redisAddr, redisPassword string, redisDB int) (*Manager, error) {
	if len(secret) < 32 {
		return nil, errors.New("AIS_JWT_SECRET must contain at least 32 bytes")
	}
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect to Redis: %w", err)
	}

	return &Manager{secret: []byte(secret), redis: client}, nil
}

func (m *Manager) Close() error {
	return m.redis.Close()
}

// CreateSession persists the backend credential and returns a client-facing JWT.
func (m *Manager) CreateSession(ctx context.Context, session Session) (string, error) {
	if session.Service == "" || session.Subject == "" || session.Credential == "" {
		return "", errors.New("service, subject, and backend credential are required")
	}

	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", fmt.Errorf("generate session ID: %w", err)
	}
	id := hex.EncodeToString(idBytes)
	expiresAt := time.Now().Add(tokenTTL)

	claims := Claims{
		Service: session.Service,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   session.Subject,
			Audience:  jwt.ClaimStrings{session.Service},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        id,
		},
	}

	raw, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("encode session: %w", err)
	}
	if err := m.redis.Set(ctx, redisPrefix+id, raw, time.Until(expiresAt)).Err(); err != nil {
		return "", fmt.Errorf("persist session in Redis: %w", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		_ = m.redis.Del(ctx, redisPrefix+id).Err()
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return signed, nil
}

// Resolve validates the JWT and loads its service-specific backend credential from Redis.
func (m *Manager) Resolve(ctx context.Context, rawToken, expectedService string) (*Session, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, errors.New("missing JWT")
	}

	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected JWT signing method: %s", token.Method.Alg())
		}
		return m.secret, nil
	}, jwt.WithIssuer(issuer), jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid or expired JWT")
	}
	if claims.Service != expectedService || claims.Subject == "" || claims.ID == "" {
		return nil, errors.New("JWT is not valid for this service")
	}

	audienceMatches := false
	for _, audience := range claims.Audience {
		if audience == expectedService {
			audienceMatches = true
			break
		}
	}
	if !audienceMatches {
		return nil, errors.New("JWT audience mismatch")
	}

	value, err := m.redis.Get(ctx, redisPrefix+claims.ID).Bytes()
	if err != nil {
		return nil, errors.New("session not found or expired")
	}
	var session Session
	if err := json.Unmarshal(value, &session); err != nil {
		return nil, fmt.Errorf("decode Redis session: %w", err)
	}
	if session.Service != expectedService || session.Subject != claims.Subject {
		return nil, errors.New("session does not match JWT")
	}
	return &session, nil
}
