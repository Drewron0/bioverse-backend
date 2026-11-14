package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
)

type contextKey string

const (
	// RequestIDKey is the context key for the HTTP request identifier.
	RequestIDKey contextKey = "request_id"
)

// RequestID injects a UUID into each request context and sets the X-Request-ID header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(RequestIDKey).(string); ok {
		return val
	}
	return ""
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Logger logs each incoming request method, path, status, duration, and request ID.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := GetRequestID(r.Context())

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			duration := time.Since(start)
			log.Info("http request",
				slog.String("request_id", reqID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", duration),
				slog.String("client_ip", r.RemoteAddr),
			)
		})
	}
}

// Recoverer catches panics, logs the failure, and returns an RFC 7807 problem details response.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					reqID := GetRequestID(r.Context())
					log.Error("panic recovered",
						slog.String("request_id", reqID),
						slog.Any("error", rec),
					)
					httperr.InternalServerError(w, r)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// CORSConfig builds a chi CORS handler from allowed origins.
func CORSConfig(allowedOrigins []string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Idempotency-Key", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
