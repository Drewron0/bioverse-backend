package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/Drewron01/bioverse-backend/internal/platform/crypto"
	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
)

// AuthMiddleware creates an HTTP middleware that enforces JWT authentication.
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httperr.Unauthorized(w, r, "Authorization header is required.")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httperr.Unauthorized(w, r, "Invalid authorization header format. Expected 'Bearer <token>'.")
				return
			}

			tokenString := strings.TrimSpace(parts[1])
			claims, err := crypto.ValidateAccessToken(tokenString, jwtSecret)
			if err != nil {
				httperr.Unauthorized(w, r, "Invalid or expired access token.")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
			ctx = context.WithValue(ctx, UserEmailContextKey, claims.Email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
