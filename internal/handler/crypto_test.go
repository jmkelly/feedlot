package handler

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes for AES-256
	plaintext := []byte("sk-test-api-key-12345")

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypt returned empty string")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Decrypted = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptProducesDifferentOutputs(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	plaintext := []byte("same-key")

	out1, _ := Encrypt(plaintext, key)
	out2, _ := Encrypt(plaintext, key)

	if out1 == out2 {
		t.Error("Encrypt should produce different outputs for the same input (random nonce)")
	}
}

func TestEncryptEmptyKey(t *testing.T) {
	_, err := Encrypt([]byte("data"), []byte{})
	if err == nil {
		t.Error("Encrypt should fail with empty key")
	}
}

func TestDecryptEmptyKey(t *testing.T) {
	_, err := Decrypt("nonce$ciphertext", []byte{})
	if err == nil {
		t.Error("Decrypt should fail with empty key")
	}
}

func TestDecryptInvalidFormat(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")

	// No separator
	_, err := Decrypt("invalidformat", key)
	if err == nil {
		t.Error("Decrypt should fail with no separator")
	}

	// Invalid hex nonce
	_, err = Decrypt("zz$ciphertext", key)
	if err == nil {
		t.Error("Decrypt should fail with invalid hex nonce")
	}

	// Invalid hex ciphertext
	_, err = Decrypt("abcdef01$zz", key)
	if err == nil {
		t.Error("Decrypt should fail with invalid hex ciphertext")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	plaintext := []byte("secret-api-key")

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with the last character
	tampered := encrypted[:len(encrypted)-1] + "0"

	_, err = Decrypt(tampered, key)
	if err == nil {
		t.Error("Decrypt should fail with tampered ciphertext (GCM authentication)")
	}
}

func TestEncryptDecryptWithDifferentKey(t *testing.T) {
	key1 := []byte("0123456789abcdef0123456789abcdef")
	key2 := []byte("fedcba9876543210fedcba9876543210")

	encrypted, err := Encrypt([]byte("test-data"), key1)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(encrypted, key2)
	if err == nil {
		t.Error("Decrypt should fail when using a different key")
	}
}

func TestEncryptDecryptEmptyData(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")

	encrypted, err := Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("Encrypt empty data failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt empty data failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("Decrypted length = %d, want 0", len(decrypted))
	}
}

func TestEncryptDecryptLongData(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	plaintext := make([]byte, 10000)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt long data failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt long data failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("Decrypted long data does not match original")
	}
}

func TestEncryptKeyNot32Bytes(t *testing.T) {
	// AES-256 requires 32 bytes, but Go's AES implementation will panic on wrong size
	// We test that it returns an error, not a panic
	key := []byte("too-short")

	_, err := Encrypt([]byte("data"), key)
	if err == nil {
		t.Error("Encrypt should fail with key that is not 32 bytes")
	}
}
