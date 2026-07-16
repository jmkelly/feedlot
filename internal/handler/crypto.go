package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
// Returns hex(nonce) + "$" + hex(ciphertext).
func Encrypt(plaintext []byte, key []byte) (string, error) {
	if len(key) == 0 {
		return "", errors.New("encryption key is empty")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return hex.EncodeToString(nonce) + "$" + hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a string created by Encrypt.
// Input format: hex(nonce) + "$" + hex(ciphertext).
func Decrypt(encoded string, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("encryption key is empty")
	}

	parts := []byte(encoded)
	sep := -1
	for i, b := range parts {
		if b == '$' {
			sep = i
			break
		}
	}
	if sep < 0 {
		return nil, errors.New("invalid encrypted format: missing separator")
	}

	nonce, err := hex.DecodeString(string(parts[:sep]))
	if err != nil {
		return nil, fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := hex.DecodeString(string(parts[sep+1:]))
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}
