package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/Drewron0/bioverse-backend/internal/platform/crypto"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDContextKey    contextKey = "user_id"
	UserEmailContextKey contextKey = "user_email"
	AdminIDContextKey   contextKey = "admin_id"
	AdminRoleContextKey contextKey = "admin_role"
)

// GetUserIDFromContext retrieves the authenticated user's UUID from the context.
func GetUserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	val, ok := ctx.Value(UserIDContextKey).(uuid.UUID)
	return val, ok
}

// GetAdminIDFromContext retrieves the authenticated admin's UUID from the context.
func GetAdminIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	val, ok := ctx.Value(AdminIDContextKey).(uuid.UUID)
	return val, ok
}

// GetAdminRoleFromContext retrieves the authenticated admin's role from the context.
func GetAdminRoleFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AdminRoleContextKey).(string)
	return val, ok
}

// AuthMiddleware creates an HTTP middleware that enforces JWT authentication for mobile/web users.
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

// AdminAuthMiddleware creates an HTTP middleware that enforces dedicated JWT authentication for platform admins.
func AdminAuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				httperr.Unauthorized(w, r, "Admin authorization header is required.")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httperr.Unauthorized(w, r, "Invalid authorization header format. Expected 'Bearer <token>'.")
				return
			}

			tokenString := strings.TrimSpace(parts[1])
			claims, err := crypto.ValidateAdminAccessToken(tokenString, jwtSecret)
			if err != nil {
				httperr.Unauthorized(w, r, "Invalid, expired, or unauthorized admin access token.")
				return
			}

			ctx := context.WithValue(r.Context(), AdminIDContextKey, claims.AdminID)
			ctx = context.WithValue(ctx, AdminRoleContextKey, claims.Role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdminRole verifies that the authenticated admin possesses one of the allowed roles.
func RequireAdminRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := GetAdminRoleFromContext(r.Context())
			if !ok {
				httperr.Unauthorized(w, r, "Admin authentication required.")
				return
			}

			allowed := false
			for _, ar := range allowedRoles {
				if strings.EqualFold(role, ar) {
					allowed = true
					break
				}
			}

			if !allowed {
				httperr.Forbidden(w, r, "Insufficient admin privileges for this operation.")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
