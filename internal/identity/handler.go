package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/middleware"
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
func (h *Handler) RegisterRoutes(r chi.Router, userAuthMiddleware, adminAuthMiddleware func(http.Handler) http.Handler) {
	authLimiter := middleware.RateLimit(30, time.Minute)

	r.Route("/v1/app/auth", func(authRouter chi.Router) {
		authRouter.With(authLimiter).Post("/register", h.handleRegister)
		authRouter.With(authLimiter).Post("/login", h.handleLogin)
		authRouter.With(authLimiter).Post("/google", h.handleGoogleLogin)
		authRouter.With(authLimiter).Post("/refresh", h.handleRefresh)
		authRouter.With(authLimiter).Post("/forgot-password", h.handleForgotPassword)
		authRouter.With(authLimiter).Post("/reset-password", h.handleResetPassword)
		authRouter.With(authLimiter).Post("/verify-email", h.handleVerifyEmail)

		authRouter.Group(func(protected chi.Router) {
			protected.Use(userAuthMiddleware)
			protected.Post("/logout", h.handleLogout)
		})
	})

	r.Route("/v1/app/users", func(userRouter chi.Router) {
		userRouter.Use(userAuthMiddleware)
		userRouter.Get("/me", h.handleGetMe)
		userRouter.Get("/me/stats", h.handleGetUserStats)
		userRouter.Patch("/me", h.handlePatchMe)
		userRouter.Post("/fcm-token", h.handleFCMToken)
	})

	r.Route("/v1/app/notifications", func(notifRouter chi.Router) {
		notifRouter.Use(userAuthMiddleware)
		notifRouter.Get("/", h.handleListNotifications)
		notifRouter.Patch("/{id}/read", h.handleMarkNotificationRead)
		notifRouter.Post("/read-all", h.handleMarkAllNotificationsRead)
	})

	r.Route("/v1/admin/auth", func(adminAuth chi.Router) {
		adminAuth.With(authLimiter).Post("/login", h.handleAdminLogin)
		adminAuth.With(authLimiter).Post("/refresh", h.handleAdminRefresh)

		adminAuth.Group(func(protected chi.Router) {
			protected.Use(adminAuthMiddleware)
			protected.Post("/logout", h.handleAdminLogout)
			protected.Post("/mfa/setup", h.handleAdminMFASetup)
			protected.Post("/mfa/verify", h.handleAdminMFAVerify)
		})
	})
}

type registerRequest struct {
	FullName              string `json:"full_name"`
	Email                 string `json:"email"`
	Password              string `json:"password"`
	PhoneNumber           string `json:"phone_number"`
	Timezone              string `json:"timezone"`
	PreferredCurrencyCode string `json:"preferred_currency_code"`
	Gender                string `json:"gender"`
	DateOfBirth           string `json:"date_of_birth"`
	ReferralCode          string `json:"referral_code"`
}

type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	DeviceInfo string `json:"device_info"`
}

type googleLoginRequest struct {
	IDToken    string `json:"id_token"`
	DeviceInfo string `json:"device_info"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	DeviceInfo   string `json:"device_info"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type updateProfileRequest struct {
	FullName              *string `json:"full_name"`
	PhoneNumber           *string `json:"phone_number"`
	Gender                *string `json:"gender"`
	DateOfBirth           *string `json:"date_of_birth"`
	Timezone              *string `json:"timezone"`
	PreferredCurrencyCode *string `json:"preferred_currency_code"`
	AvatarObjectKey       *string `json:"avatar_object_key"`
}

type fcmTokenRequest struct {
	FCMToken   string `json:"fcm_token"`
	DeviceType string `json:"device_type"`
}

type adminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

type authResponse struct {
	AccessToken  string              `json:"access_token"`
	RefreshToken string              `json:"refresh_token"`
	TokenType    string              `json:"token_type"`
	ExpiresIn    int                 `json:"expires_in"`
	User         userProfileResponse `json:"user"`
}

type adminAuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	AdminRole    string `json:"admin_role"`
}

type userProfileResponse struct {
	ID                    string  `json:"id"`
	FullName              string  `json:"full_name"`
	Email                 string  `json:"email"`
	PhoneNumber           *string `json:"phone_number"`
	Gender                *string `json:"gender"`
	DateOfBirth           *string `json:"date_of_birth"`
	Timezone              string  `json:"timezone"`
	PreferredCurrencyCode string  `json:"preferred_currency_code"`
	AvatarURL             *string `json:"avatar_url"`
	ReferralCode          string  `json:"referral_code"`
	PointsBalance         int64   `json:"points_balance"`
	LockedPointsBalance   int64   `json:"locked_points_balance"`
	IsEmailVerified       bool    `json:"is_email_verified"`
	CurrentStreak         int     `json:"current_streak"`
	CreatedAt             string  `json:"created_at"`
}

func mapUserProfileResponse(u UserSummary) userProfileResponse {
	return userProfileResponse{
		ID:                    u.ID.String(),
		FullName:              u.FullName,
		Email:                 u.Email,
		PhoneNumber:           u.PhoneNumber,
		Gender:                u.Gender,
		DateOfBirth:           u.DateOfBirth,
		Timezone:              u.Timezone,
		PreferredCurrencyCode: u.PreferredCurrencyCode,
		AvatarURL:             u.AvatarURL,
		ReferralCode:          u.ReferralCode,
		PointsBalance:         u.PointsBalance,
		LockedPointsBalance:   u.LockedPointsBalance,
		IsEmailVerified:       u.IsEmailVerified,
		CurrentStreak:         u.CurrentStreak,
		CreatedAt:             u.CreatedAt.Format(time.RFC3339),
	}
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.Register(r.Context(), RegisterInput{
		FullName:              req.FullName,
		Email:                 req.Email,
		Password:              req.Password,
		PhoneNumber:           req.PhoneNumber,
		Timezone:              req.Timezone,
		PreferredCurrencyCode: req.PreferredCurrencyCode,
		Gender:                req.Gender,
		DateOfBirth:           req.DateOfBirth,
		ReferralCode:          req.ReferralCode,
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
		User:         mapUserProfileResponse(result.User),
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
		User:         mapUserProfileResponse(result.User),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	var req googleLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.LoginWithGoogle(r.Context(), req.IDToken, req.DeviceInfo)
	if err != nil {
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	resp := authResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
		User:         mapUserProfileResponse(result.User),
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

	result, err := h.service.RefreshToken(r.Context(), RefreshInput{
		RefreshToken: req.RefreshToken,
		DeviceInfo:   req.DeviceInfo,
	})
	if err != nil {
		if errors.Is(err, ErrTokenRevoked) || errors.Is(err, ErrTokenExpired) {
			httperr.Unauthorized(w, r, "Refresh token is invalid or expired.")
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
		User:         mapUserProfileResponse(result.User),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	_ = h.service.ForgotPassword(r.Context(), req.Email)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "If that email address exists, a password reset link has been dispatched.",
	})
}

func (h *Handler) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	if err := h.service.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password reset successfully.",
	})
}

func (h *Handler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	if err := h.service.VerifyEmail(r.Context(), req.Token); err != nil {
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Email verified successfully.",
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.RefreshToken != "" {
		_ = h.service.Logout(r.Context(), req.RefreshToken)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "User logged out successfully.",
	})
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
			httperr.NotFound(w, r, "User profile not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mapUserProfileResponse(*profile))
}

func (h *Handler) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	updated, err := h.service.UpdateUserProfile(r.Context(), userID, UpdateProfileInput{
		FullName:              req.FullName,
		PhoneNumber:           req.PhoneNumber,
		Gender:                req.Gender,
		DateOfBirth:           req.DateOfBirth,
		Timezone:              req.Timezone,
		PreferredCurrencyCode: req.PreferredCurrencyCode,
		AvatarURL:             req.AvatarObjectKey,
	})
	if err != nil {
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mapUserProfileResponse(*updated))
}

func (h *Handler) handleFCMToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	var req fcmTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FCMToken == "" {
		httperr.BadRequest(w, r, "fcm_token is required.", nil)
		return
	}

	deviceType := strings.ToLower(req.DeviceType)
	if deviceType != "ios" && deviceType != "android" {
		httperr.BadRequest(w, r, "device_type must be either 'ios' or 'android'.", nil)
		return
	}

	if err := h.service.RegisterFCMToken(r.Context(), userID, req.FCMToken, deviceType); err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "FCM device token registered successfully.",
	})
}

func (h *Handler) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req adminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.AdminLogin(r.Context(), req.Email, req.Password, req.TOTPCode)
	if err != nil {
		if errors.Is(err, ErrMfaRequired) {
			httperr.UnauthorizedWithCode(w, r, "MFA_REQUIRED", "Two-factor authentication TOTP code is required.")
			return
		}
		if errors.Is(err, ErrAccountLocked) {
			httperr.UnauthorizedWithCode(w, r, "ACCOUNT_LOCKED", "Admin account is temporarily locked due to repeated failed logins. Please try again after 15 minutes.")
			return
		}
		if errors.Is(err, ErrInvalidTOTPCode) {
			httperr.UnauthorizedWithCode(w, r, "INVALID_TOTP", "Invalid two-factor authentication code.")
			return
		}
		httperr.Unauthorized(w, r, "Invalid admin credentials.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(adminAuthResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
		AdminRole:    result.AdminRole,
	})
}

func (h *Handler) handleAdminMFASetup(w http.ResponseWriter, r *http.Request) {
	adminID, ok := GetAdminIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Admin authentication required.")
		return
	}

	setup, err := h.service.AdminSetupMFA(r.Context(), adminID)
	if err != nil {
		if errors.Is(err, ErrMfaAlreadyEnabled) || strings.Contains(err.Error(), "already enabled") {
			httperr.BadRequest(w, r, "MFA is already enabled on this account.", nil)
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(setup)
}

type mfaVerifyRequest struct {
	TOTPCode string `json:"totp_code"`
}

func (h *Handler) handleAdminMFAVerify(w http.ResponseWriter, r *http.Request) {
	adminID, ok := GetAdminIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Admin authentication required.")
		return
	}

	var req mfaVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.TOTPCode) == "" {
		httperr.BadRequest(w, r, "Valid totp_code is required.", nil)
		return
	}

	if err := h.service.AdminVerifyMFA(r.Context(), adminID, req.TOTPCode); err != nil {
		if errors.Is(err, ErrInvalidTOTPCode) {
			httperr.UnauthorizedWithCode(w, r, "INVALID_TOTP", "Invalid TOTP verification code.")
			return
		}
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Two-factor authentication (MFA) enrolled and activated successfully.",
	})
}

func (h *Handler) handleAdminRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.AdminRefresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) || errors.Is(err, ErrRefreshTokenExpired) {
			httperr.Unauthorized(w, r, "Invalid or expired admin refresh token.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(adminAuthResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    result.ExpiresIn,
		AdminRole:    result.AdminRole,
	})
}

func (h *Handler) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.RefreshToken != "" {
		_ = h.service.AdminLogout(r.Context(), req.RefreshToken)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Admin session revoked successfully.",
	})
}

func (h *Handler) handleGetUserStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	stats, err := h.service.GetUserStats(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			httperr.NotFound(w, r, "User profile not found")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	var cursorTime *time.Time
	cursorStr := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursorStr != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, cursorStr); err == nil {
			cursorTime = &parsed
		} else if parsed, err := time.Parse(time.RFC3339, cursorStr); err == nil {
			cursorTime = &parsed
		}
	}

	notifs, nextCursor, err := h.service.ListUserNotificationsWithCursor(r.Context(), userID, cursorTime, limit)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":       notifs,
		"next_cursor": nextCursor,
	})
}

func (h *Handler) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	notifIDStr := chi.URLParam(r, "id")
	notifID, err := uuid.Parse(notifIDStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid notification UUID", nil)
		return
	}

	notif, err := h.service.MarkNotificationRead(r.Context(), userID, notifID)
	if err != nil {
		httperr.NotFound(w, r, "Notification not found")
		return
	}

	writeJSON(w, http.StatusOK, notif)
}

func (h *Handler) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	if err := h.service.MarkAllNotificationsRead(r.Context(), userID); err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "All notifications marked as read",
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
