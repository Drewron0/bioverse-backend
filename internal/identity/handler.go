package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves HTTP endpoints for identity and authentication.
type Handler struct {
	service *Service
}

// NewHandler constructs a new identity HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes registers identity routes on the provided chi router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/v1/app/auth", func(authRouter chi.Router) {
		authRouter.Post("/register", h.handleRegister)
		authRouter.Post("/login", h.handleLogin)
		authRouter.Post("/refresh", h.handleRefresh)
	})

	r.Route("/v1/app/users", func(userRouter chi.Router) {
		userRouter.Use(authMiddleware)
		userRouter.Get("/me", h.handleGetMe)
	})
}

type registerRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Timezone string `json:"timezone"`
}

type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	DeviceInfo string `json:"device_info"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	DeviceInfo   string `json:"device_info"`
}

type authResponse struct {
	AccessToken  string              `json:"access_token"`
	RefreshToken string              `json:"refresh_token"`
	TokenType    string              `json:"token_type"`
	ExpiresIn    int                 `json:"expires_in"`
	User         userProfileResponse `json:"user"`
}

type userProfileResponse struct {
	ID                    string `json:"id"`
	FullName              string `json:"full_name"`
	Email                 string `json:"email"`
	Timezone              string `json:"timezone"`
	PreferredCurrencyCode string `json:"preferred_currency_code"`
	PointsBalance         int64  `json:"points_balance"`
	CurrentStreak         int    `json:"current_streak"`
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.Register(r.Context(), RegisterInput{
		FullName: req.FullName,
		Email:    req.Email,
		Password: req.Password,
		Timezone: req.Timezone,
	})
	if err != nil {
		if errors.Is(err, ErrEmailAlreadyExists) {
			httperr.Conflict(w, r, "Email address is already registered.")
			return
		}
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	resp := authResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
		User: userProfileResponse{
			ID:                    result.User.ID.String(),
			FullName:              result.User.FullName,
			Email:                 result.User.Email,
			Timezone:              result.User.Timezone,
			PreferredCurrencyCode: result.User.PreferredCurrencyCode,
			PointsBalance:         result.User.PointsBalance,
			CurrentStreak:         result.User.CurrentStreak,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.Login(r.Context(), LoginInput{
		Email:      req.Email,
		Password:   req.Password,
		DeviceInfo: req.DeviceInfo,
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			httperr.Unauthorized(w, r, "Invalid email or password.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	resp := authResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
		User: userProfileResponse{
			ID:                    result.User.ID.String(),
			FullName:              result.User.FullName,
			Email:                 result.User.Email,
			Timezone:              result.User.Timezone,
			PreferredCurrencyCode: result.User.PreferredCurrencyCode,
			PointsBalance:         result.User.PointsBalance,
			CurrentStreak:         result.User.CurrentStreak,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}
	if req.RefreshToken == "" {
		httperr.BadRequest(w, r, "refresh_token is required.", nil)
		return
	}

	result, err := h.service.RefreshToken(r.Context(), RefreshInput{
		RefreshToken: req.RefreshToken,
		DeviceInfo:   req.DeviceInfo,
	})
	if err != nil {
		if errors.Is(err, ErrTokenRevoked) || errors.Is(err, ErrTokenExpired) {
			httperr.Unauthorized(w, r, "Refresh token is invalid or has expired.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	resp := authResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
		User: userProfileResponse{
			ID:                    result.User.ID.String(),
			FullName:              result.User.FullName,
			Email:                 result.User.Email,
			Timezone:              result.User.Timezone,
			PreferredCurrencyCode: result.User.PreferredCurrencyCode,
			PointsBalance:         result.User.PointsBalance,
			CurrentStreak:         result.User.CurrentStreak,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	profile, err := h.service.GetUserProfile(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			httperr.NotFound(w, r, "User account not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	resp := userProfileResponse{
		ID:                    profile.ID.String(),
		FullName:              profile.FullName,
		Email:                 profile.Email,
		Timezone:              profile.Timezone,
		PreferredCurrencyCode: profile.PreferredCurrencyCode,
		PointsBalance:         profile.PointsBalance,
		CurrentStreak:         profile.CurrentStreak,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ContextKey defines a custom type for context key identifiers.
type ContextKey string

const (
	// UserIDContextKey is the context key for authenticated user ID.
	UserIDContextKey ContextKey = "authenticated_user_id"
	// UserEmailContextKey is the context key for authenticated user email.
	UserEmailContextKey ContextKey = "authenticated_user_email"
)

// GetUserIDFromContext extracts the authenticated UUID from request context.
func GetUserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(UserIDContextKey).(uuid.UUID)
	return id, ok
}
