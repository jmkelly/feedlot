package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"
)

const (
	hashTime    = 1
	hashMemory  = 64 * 1024
	hashThreads = 4
	hashKeyLen  = 32
	saltLen     = 16
)

type Auth struct {
	jwtSecret []byte
}

func New(jwtSecret string) *Auth {
	return &Auth{jwtSecret: []byte(jwtSecret)}
}

func (a *Auth) HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, hashTime, hashMemory, hashThreads, hashKeyLen)
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(hash), nil
}

func (a *Auth) VerifyPassword(password, encoded string) bool {
	parts := []byte(encoded)
	sep := -1
	for i, b := range parts {
		if b == '$' {
			sep = i
			break
		}
	}
	if sep < 0 {
		return false
	}
	salt, err := hex.DecodeString(string(parts[:sep]))
	if err != nil {
		return false
	}
	expectedHash, err := hex.DecodeString(string(parts[sep+1:]))
	if err != nil {
		return false
	}
	computedHash := argon2.IDKey([]byte(password), salt, hashTime, hashMemory, hashThreads, hashKeyLen)
	if len(computedHash) != len(expectedHash) {
		return false
	}
	for i := range computedHash {
		if computedHash[i] != expectedHash[i] {
			return false
		}
	}
	return true
}

func (a *Auth) GenerateToken(userID int64) (string, time.Time, error) {
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     expiresAt.Unix(),
		"iat":     time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(a.jwtSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return tokenStr, expiresAt, nil
}

func (a *Auth) ValidateToken(tokenStr string) (int64, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return a.jwtSecret, nil
	})
	if err != nil {
		return 0, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return 0, fmt.Errorf("invalid token claims")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, fmt.Errorf("invalid user_id in token")
	}

	return int64(userIDFloat), nil
}

func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
