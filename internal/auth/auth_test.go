package auth

import (
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	a := New("test-secret")

	// Hash and verify a password
	hash, err := a.HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("HashPassword returned empty string")
	}

	if !a.VerifyPassword("correct-password", hash) {
		t.Error("VerifyPassword returned false for correct password")
	}

	if a.VerifyPassword("wrong-password", hash) {
		t.Error("VerifyPassword returned true for wrong password")
	}

	if a.VerifyPassword("", hash) {
		t.Error("VerifyPassword returned true for empty password")
	}
}

func TestVerifyPasswordInvalidFormat(t *testing.T) {
	a := New("test-secret")

	// Test with invalid format (no separator)
	if a.VerifyPassword("password", "invalidhash") {
		t.Error("VerifyPassword should return false for invalid format")
	}

	// Test with invalid hex in salt
	if a.VerifyPassword("password", "xx$abc") {
		t.Error("VerifyPassword should return false for invalid hex salt")
	}

	// Test with invalid hex in hash
	if a.VerifyPassword("password", "abc$zz") {
		t.Error("VerifyPassword should return false for invalid hex hash")
	}
}

func TestPasswordUniqueness(t *testing.T) {
	a := New("test-secret")

	// Same password should produce different hashes (due to random salt)
	hash1, _ := a.HashPassword("same-password")
	hash2, _ := a.HashPassword("same-password")

	if hash1 == hash2 {
		t.Error("Same password should produce different hashes due to random salt")
	}
}

func TestGenerateToken(t *testing.T) {
	a := New("test-secret")

	token, expiresAt, err := a.GenerateToken(42)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken returned empty token")
	}
	if expiresAt.IsZero() {
		t.Fatal("GenerateToken returned zero expiration time")
	}
}

func TestValidateToken(t *testing.T) {
	a := New("test-secret")

	userID, expiresAt, err := a.GenerateToken(123)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}
	_ = expiresAt

	// Valid token
	uid, err := a.ValidateToken(userID)
	if err != nil {
		t.Fatalf("ValidateToken failed for valid token: %v", err)
	}
	if uid != 123 {
		t.Errorf("ValidateToken returned wrong user ID: got %d, want 123", uid)
	}

	// Invalid token
	_, err = a.ValidateToken("invalid-token")
	if err == nil {
		t.Error("ValidateToken should fail for invalid token")
	}

	// Empty token
	_, err = a.ValidateToken("")
	if err == nil {
		t.Error("ValidateToken should fail for empty token")
	}

	// Token with wrong signature
	wrongAuth := New("different-secret")
	wrongToken, _, _ := wrongAuth.GenerateToken(456)
	_, err = a.ValidateToken(wrongToken)
	if err == nil {
		t.Error("ValidateToken should fail for token signed with different secret")
	}
}

func TestValidateTokenExtractsCorrectUserID(t *testing.T) {
	a := New("test-secret")

	testCases := []int64{0, 1, 999, -1}
	for _, id := range testCases {
		token, _, err := a.GenerateToken(id)
		if err != nil {
			t.Fatalf("GenerateToken(%d) failed: %v", id, err)
		}

		uid, err := a.ValidateToken(token)
		if err != nil {
			t.Errorf("ValidateToken failed for userID %d: %v", id, err)
		}
		if uid != id {
			t.Errorf("ValidateToken returned %d for userID %d", uid, id)
		}
	}
}

func TestGenerateSessionToken(t *testing.T) {
	token1, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken failed: %v", err)
	}
	if len(token1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("Session token length: got %d, want 64", len(token1))
	}

	token2, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken failed: %v", err)
	}

	if token1 == token2 {
		t.Error("Consecutive session tokens should be different")
	}
}

func TestAuthWithEmptySecret(t *testing.T) {
	a := New("")
	token, _, err := a.GenerateToken(1)
	if err != nil {
		t.Fatalf("GenerateToken with empty secret failed: %v", err)
	}

	uid, err := a.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken with empty secret failed: %v", err)
	}
	if uid != 1 {
		t.Errorf("Wrong user ID: got %d, want 1", uid)
	}
}

func TestHashConsistency(t *testing.T) {
	a := New("test-secret")

	hashes := make([]string, 3)
	for i := range hashes {
		h, err := a.HashPassword("same-password")
		if err != nil {
			t.Fatalf("HashPassword failed: %v", err)
		}
		hashes[i] = h
	}

	for _, h := range hashes {
		if !a.VerifyPassword("same-password", h) {
			t.Error("A hash should verify against the same password")
		}
	}
}
