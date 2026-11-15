package crypto

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTOTPGenerationAndVerification(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("unexpected error generating secret: %v", err)
	}

	if len(secret) == 0 {
		t.Fatal("secret should not be empty")
	}

	now := time.Now().UTC()
	code, err := GenerateTOTPCode(secret, now)
	if err != nil {
		t.Fatalf("unexpected error generating code: %v", err)
	}

	if len(code) != 6 {
		t.Fatalf("expected 6-digit code, got %s", code)
	}

	if !VerifyTOTPCode(secret, code, now) {
		t.Fatalf("expected code %s to be valid at now", code)
	}

	// Within 30 seconds drift:
	if !VerifyTOTPCode(secret, code, now.Add(25*time.Second)) {
		t.Fatalf("expected code to be valid with +25s drift")
	}

	// Past drift window (90s):
	if VerifyTOTPCode(secret, code, now.Add(90*time.Second)) {
		t.Fatalf("expected code to be invalid after 90s")
	}

	// Invalid code:
	if VerifyTOTPCode(secret, "000000", now) && code != "000000" {
		t.Fatalf("expected wrong code to fail verification")
	}

	uri := GenerateTOTPURI(secret, "admin@bioverse.id", "BioVerse")
	if len(uri) == 0 {
		t.Fatalf("expected valid otpauth URI")
	}
}

func TestAdminJWTGenerationAndValidation(t *testing.T) {
	adminID := uuid.New()
	secret := "test-admin-secret-key-must-be-long-enough"
	email := "admin@bioverse.id"
	role := "superadmin"

	token, err := GenerateAdminAccessToken(adminID, email, role, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate admin access token: %v", err)
	}

	claims, err := ValidateAdminAccessToken(token, secret)
	if err != nil {
		t.Fatalf("failed to validate admin access token: %v", err)
	}

	if claims.AdminID != adminID {
		t.Fatalf("expected adminID %s, got %s", adminID, claims.AdminID)
	}
	if claims.Role != role {
		t.Fatalf("expected role %s, got %s", role, claims.Role)
	}
	if claims.TokenType != "admin_access" {
		t.Fatalf("expected token_type admin_access, got %s", claims.TokenType)
	}

	// Normal user validation should fail for admin token or vice versa
	userToken, err := GenerateAccessToken(adminID, email, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate user token: %v", err)
	}

	_, err = ValidateAdminAccessToken(userToken, secret)
	if err == nil {
		t.Fatalf("expected user token to fail admin token validation")
	}
}
