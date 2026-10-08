package account

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"knowledge-base/internal/platform/httpx"
)

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(secret []byte, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (manager *TokenManager) IssueAccess(userID, sessionID uuid.UUID, now time.Time) (string, error) {
	claims := accessClaims{
		SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(manager.accessTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(manager.secret)
}

func (manager *TokenManager) ParseAccess(raw string) (Identity, error) {
	token, err := jwt.ParseWithClaims(raw, &accessClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return manager.secret, nil
	})
	if err != nil || !token.Valid {
		return Identity{}, ErrSessionInvalid
	}
	claims, ok := token.Claims.(*accessClaims)
	if !ok {
		return Identity{}, ErrSessionInvalid
	}
	userID, userErr := uuid.Parse(claims.Subject)
	sessionID, sessionErr := uuid.Parse(claims.SessionID)
	if userErr != nil || sessionErr != nil {
		return Identity{}, ErrSessionInvalid
	}
	return Identity{UserID: userID, SessionID: sessionID}, nil
}

func (manager *TokenManager) VerifyAccess(raw string) (httpx.Principal, error) {
	identity, err := manager.ParseAccess(raw)
	if err != nil {
		return httpx.Principal{}, err
	}
	return httpx.Principal{UserID: identity.UserID, SessionID: identity.SessionID}, nil
}

func (manager *TokenManager) NewRefresh(now time.Time) (raw, hash string, expiresAt time.Time, err error) {
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		return "", "", time.Time{}, err
	}
	raw = base64.RawURLEncoding.EncodeToString(bytes)
	return raw, HashSecret(raw), now.Add(manager.refreshTTL), nil
}

func (manager *TokenManager) AccessTTL() time.Duration {
	return manager.accessTTL
}

func HashSecret(secret string) string {
	hash := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:])
}
