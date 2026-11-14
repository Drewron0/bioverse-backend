package crypto_test

import (
	"testing"

	"github.com/Drewron01/bioverse-backend/internal/platform/crypto"
)

func TestAES256GCMEncryptionAndDecryption(t *testing.T) {
	keyHex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	secretPlaintext := "3201234567890001" // 16-digit Indonesian NIK

	ciphertext, err := crypto.Encrypt([]byte(secretPlaintext), keyHex)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if len(ciphertext) <= len(secretPlaintext) {
		t.Errorf("expected ciphertext with nonce and auth tag to be longer than plaintext")
	}

	decrypted, err := crypto.Decrypt(ciphertext, keyHex)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if string(decrypted) != secretPlaintext {
		t.Errorf("expected %s, got %s", secretPlaintext, string(decrypted))
	}
}

func TestSearchableHashDeterministic(t *testing.T) {
	hmacSecret := "test_hmac_secret_for_searchable_hashes"
	nik := "3201234567890001"

	hash1 := crypto.GenerateSearchableHash(nik, hmacSecret)
	hash2 := crypto.GenerateSearchableHash(nik, hmacSecret)

	if hash1 != hash2 {
		t.Errorf("HMAC must be deterministic, got %s != %s", hash1, hash2)
	}
	if len(hash1) != 64 {
		t.Errorf("expected 64 character hex string, got %d", len(hash1))
	}
}
