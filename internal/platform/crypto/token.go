package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// GenerateRandomBytes returns securely generated random bytes.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return b, nil
}

// GenerateRandomString returns a secure hex-encoded random string.
func GenerateRandomString(byteLength int) (string, error) {
	b, err := GenerateRandomBytes(byteLength)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken computes a SHA-256 hash of a raw token string for secure database storage.
func HashToken(rawToken string) string {
	hash := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(hash[:])
}

const referralCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateReferralCode generates an 8-character human-readable unique referral code.
func GenerateReferralCode() (string, error) {
	code := make([]byte, 8)
	charsetLen := big.NewInt(int64(len(referralCharset)))

	for i := range code {
		randomIndex, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate referral code character: %w", err)
		}
		code[i] = referralCharset[randomIndex.Int64()]
	}
	return string(code), nil
}
