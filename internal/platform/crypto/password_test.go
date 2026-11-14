package crypto

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestArgon2PasswordHashing(t *testing.T) {
	rawPassword := "SecurePass123!"

	hashed, err := HashPassword(rawPassword)
	if err != nil {
		t.Fatalf("expected no error hashing password, got %v", err)
	}

	if err := VerifyPassword(rawPassword, hashed); err != nil {
		t.Fatalf("expected password verification to succeed, got %v", err)
	}

	if err := VerifyPassword("WrongPassword123!", hashed); err == nil {
		t.Fatal("expected wrong password to fail verification, but it succeeded")
	}
}

func TestReferralCodeGeneration(t *testing.T) {
	code, err := GenerateReferralCode()
	if err != nil {
		t.Fatalf("expected no error generating referral code, got %v", err)
	}

	if len(code) != 8 {
		t.Fatalf("expected 8 characters referral code, got %d (%s)", len(code), code)
	}
}

func TestJWTTokenGenerationAndValidation(t *testing.T) {
	secret := "test-secret-key-1234567890123456"
	userID := uuid.New()
	email := "test@bioverse.id"

	tokenString, err := GenerateAccessToken(userID, email, secret, 5*time.Minute)
	if err != nil {
		t.Fatalf("expected no error generating access token, got %v", err)
	}

	claims, err := ValidateAccessToken(tokenString, secret)
	if err != nil {
		t.Fatalf("expected valid token, got %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, claims.UserID)
	}
	if claims.Email != email {
		t.Errorf("expected email %s, got %s", email, claims.Email)
	}
}
