package crypto

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	// ErrExpiredToken indicates the token has expired.
	ErrExpiredToken = errors.New("jwt token has expired")
	// ErrInvalidToken indicates the token signature or structure is invalid.
	ErrInvalidToken = errors.New("invalid jwt token")
)

// UserClaims defines JWT claims payload for authenticated users.
type UserClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	jwt.RegisteredClaims
}

// GenerateAccessToken signs a new JWT access token valid for the specified duration.
func GenerateAccessToken(userID uuid.UUID, email string, secret string, expiry time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := UserClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			Issuer:    "bioverse-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}

	return signedToken, nil
}

// ValidateAccessToken parses and validates an HMAC-SHA256 signed JWT token string.
func ValidateAccessToken(tokenString string, secret string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// AdminClaims defines dedicated JWT claims for platform administrators.
type AdminClaims struct {
	AdminID   uuid.UUID `json:"admin_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	TokenType string    `json:"token_type"`
	jwt.RegisteredClaims
}

// GenerateAdminAccessToken signs a new administrator access token.
func GenerateAdminAccessToken(adminID uuid.UUID, email, role, secret string, expiry time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := AdminClaims{
		AdminID:   adminID,
		Email:     email,
		Role:      role,
		TokenType: "admin_access",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   adminID.String(),
			Audience:  jwt.ClaimStrings{"bioverse-admin"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			Issuer:    "bioverse-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign admin access token: %w", err)
	}

	return signedToken, nil
}

// ValidateAdminAccessToken verifies an administrator access token.
func ValidateAdminAccessToken(tokenString string, secret string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AdminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.TokenType != "admin_access" {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
