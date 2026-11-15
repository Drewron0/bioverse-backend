package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// GenerateTOTPSecret generates a secure 20-byte random secret encoded as Base32 without padding.
func GenerateTOTPSecret() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes for TOTP secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), nil
}

// GenerateTOTPCode generates the 6-digit TOTP code for the given secret at time t.
func GenerateTOTPCode(secretBase32 string, t time.Time) (string, error) {
	cleanSecret := strings.ToUpper(strings.ReplaceAll(secretBase32, " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(cleanSecret)
	if err != nil {
		// Try standard encoding with padding if no-padding failed
		key, err = base32.StdEncoding.DecodeString(cleanSecret)
		if err != nil {
			return "", fmt.Errorf("invalid base32 secret: %w", err)
		}
	}

	counter := uint64(t.Unix() / 30)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code := truncated % 1000000

	return fmt.Sprintf("%06d", code), nil
}

// VerifyTOTPCode checks whether the provided 6-digit TOTP code matches the secret within +/- 1 step (30 seconds drift).
func VerifyTOTPCode(secretBase32, code string, t time.Time) bool {
	cleanCode := strings.TrimSpace(code)
	if len(cleanCode) != 6 {
		return false
	}

	steps := []time.Time{
		t.Add(-30 * time.Second),
		t,
		t.Add(30 * time.Second),
	}

	for _, stepTime := range steps {
		expected, err := GenerateTOTPCode(secretBase32, stepTime)
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(cleanCode), []byte(expected)) == 1 {
			return true
		}
	}

	return false
}

// GenerateTOTPURI creates an otpauth:// URI for authenticator app enrollment (QR codes).
func GenerateTOTPURI(secretBase32, email, issuer string) string {
	if issuer == "" {
		issuer = "BioVerse"
	}
	label := fmt.Sprintf("%s:%s", issuer, email)
	u := url.URL{
		Scheme: "otpauth",
		Host:   "totp",
		Path:   label,
	}
	q := u.Query()
	q.Set("secret", secretBase32)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	u.RawQuery = q.Encode()

	return u.String()
}
