package identity

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/crypto"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/email"
	"github.com/Drewron0/bioverse-backend/internal/platform/notification"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/api/idtoken"
)

var (
	// ErrEmailAlreadyExists indicates the email is already registered.
	ErrEmailAlreadyExists = errors.New("email is already registered")
	// ErrInvalidCredentials indicates incorrect email or password.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrTokenRevoked indicates a revoked refresh token was used.
	ErrTokenRevoked = errors.New("token has been revoked")
	// ErrTokenExpired indicates a refresh token has expired.
	ErrTokenExpired = errors.New("token has expired")
	// ErrInvalidRefreshToken indicates invalid refresh token.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrRefreshTokenExpired indicates expired refresh token.
	ErrRefreshTokenExpired = ErrTokenExpired
	// ErrUserNotFound indicates the user does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrMfaRequired indicates admin requires TOTP MFA verification.
	ErrMfaRequired = errors.New("mfa totp code required")
	// ErrMfaAlreadyEnabled indicates admin already has MFA activated.
	ErrMfaAlreadyEnabled = errors.New("MFA is already enabled on this account")
	// ErrAccountLocked indicates the admin account is temporarily locked due to brute-force attempts.
	ErrAccountLocked = errors.New("admin account is temporarily locked due to repeated failed logins")
	// ErrInvalidTOTPCode indicates an invalid two-factor authentication code.
	ErrInvalidTOTPCode = errors.New("invalid two-factor authentication code")
)

// RegisterInput represents the payload required to register a new user.
type RegisterInput struct {
	FullName              string
	Email                 string
	Password              string
	PhoneNumber           string
	Timezone              string
	PreferredCurrencyCode string
	Gender                string
	DateOfBirth           string
	ReferralCode          string
}

// LoginInput represents credentials for user authentication.
type LoginInput struct {
	Email      string
	Password   string
	DeviceInfo string
}

// RefreshInput represents parameters for token rotation.
type RefreshInput struct {
	RefreshToken string
	DeviceInfo   string
}

// UpdateProfileInput represents fields that can be updated on user profile.
type UpdateProfileInput struct {
	FullName              *string
	PhoneNumber           *string
	Gender                *string
	DateOfBirth           *string
	Timezone              *string
	PreferredCurrencyCode *string
	AvatarURL             *string
}

// AuthResult holds authentication tokens and basic user profile information.
type AuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         UserSummary
}

// AdminAuthResult holds tokens and role for authenticated administrator.
type AdminAuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	AdminRole    string
}

// UserSummary contains essential public fields of an authenticated user.
type UserSummary struct {
	ID                    uuid.UUID
	FullName              string
	Email                 string
	PhoneNumber           *string
	Gender                *string
	DateOfBirth           *string
	Timezone              string
	PreferredCurrencyCode string
	AvatarURL             *string
	ReferralCode          string
	PointsBalance         int64
	LockedPointsBalance   int64
	IsEmailVerified       bool
	CurrentStreak         int
	CreatedAt             time.Time
}

// GoogleTokenVerifier defines a function to validate Google ID tokens.
type GoogleTokenVerifier func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error)

// Service provides business logic for identity, authentication, and user accounts.
type Service struct {
	pool           *pgxpool.Pool
	store          store.Querier
	config         *config.Config
	emailSender    email.Sender
	pushNotifier   notification.Dispatcher
	googleVerifier GoogleTokenVerifier
}

// NewService constructs a new identity service.
func NewService(pool *pgxpool.Pool, q store.Querier, cfg *config.Config, emailSender email.Sender, pushNotifier notification.Dispatcher) *Service {
	return &Service{
		pool:           pool,
		store:          q,
		config:         cfg,
		emailSender:    emailSender,
		pushNotifier:   pushNotifier,
		googleVerifier: idtoken.Validate,
	}
}

// SetGoogleTokenVerifier allows injecting a custom or mock GoogleTokenVerifier (useful for tests).
func (s *Service) SetGoogleTokenVerifier(verifier GoogleTokenVerifier) {
	s.googleVerifier = verifier
}

// Register creates a new user account, streak record, and initial tokens.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if _, err := mail.ParseAddress(in.Email); err != nil {
		return nil, fmt.Errorf("invalid email address format: %w", err)
	}
	if len(in.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters long")
	}
	if strings.TrimSpace(in.FullName) == "" {
		return nil, errors.New("full name must not be empty")
	}

	timezone := in.Timezone
	if strings.TrimSpace(timezone) == "" {
		timezone = "Asia/Jakarta"
	}

	_, err := s.store.GetUserByEmail(ctx, in.Email)
	if err == nil {
		return nil, ErrEmailAlreadyExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing email: %w", err)
	}

	passwordHash, err := crypto.HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	userID, err := uuid.NewV7()
	if err != nil {
		userID = uuid.New()
	}

	referralCode, err := crypto.GenerateReferralCode()
	if err != nil {
		return nil, fmt.Errorf("failed to generate referral code: %w", err)
	}

	rawRefreshToken, err := crypto.GenerateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	tokenHash := crypto.HashToken(rawRefreshToken)

	tokenID, err := uuid.NewV7()
	if err != nil {
		tokenID = uuid.New()
	}

	refreshExpiry := time.Now().UTC().Add(s.config.Auth.RefreshExpiry)

	var createdUser store.CreateUserRow
	var createdStreak store.UserStreaks

	currency := in.PreferredCurrencyCode
	if strings.TrimSpace(currency) == "" {
		currency = "USD"
	}

	var phoneText pgtype.Text
	if strings.TrimSpace(in.PhoneNumber) != "" {
		phoneText = pgtype.Text{String: strings.TrimSpace(in.PhoneNumber), Valid: true}
	}

	var genderEnum store.NullUserGender
	if strings.TrimSpace(in.Gender) != "" {
		genderEnum = store.NullUserGender{UserGender: store.UserGender(strings.TrimSpace(in.Gender)), Valid: true}
	}

	var dob pgtype.Date
	if strings.TrimSpace(in.DateOfBirth) != "" {
		if parsedDate, err := time.Parse("2006-01-02", strings.TrimSpace(in.DateOfBirth)); err == nil {
			dob = pgtype.Date{Time: parsedDate, Valid: true}
		}
	}

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		var referredByUUID pgtype.UUID
		if strings.TrimSpace(in.ReferralCode) != "" {
			refCode := pgtype.Text{String: strings.ToUpper(strings.TrimSpace(in.ReferralCode)), Valid: true}
			if referrer, err := txStore.GetUserByReferralCode(ctx, refCode); err == nil {
				referredByUUID = pgtype.UUID{Bytes: referrer.ID, Valid: true}
			}
		}

		u, err := txStore.CreateUser(ctx, store.CreateUserParams{
			ID:                    userID,
			FullName:              in.FullName,
			Email:                 in.Email,
			PasswordHash:          pgtype.Text{String: passwordHash, Valid: true},
			PhoneNumber:           phoneText,
			Gender:                genderEnum,
			DateOfBirth:           dob,
			Timezone:              timezone,
			PreferredCurrencyCode: currency,
			ReferralCode:          pgtype.Text{String: referralCode, Valid: true},
			ReferredByUserID:      referredByUUID,
		})
		if err != nil {
			return fmt.Errorf("failed to insert user record: %w", err)
		}
		createdUser = u

		_ = txStore.LinkCompanyMemberUserByEmail(ctx, store.LinkCompanyMemberUserByEmailParams{
			InvitedEmail: in.Email,
			UserID:       pgtype.UUID{Bytes: userID, Valid: true},
		})

		st, err := txStore.CreateUserStreak(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to initialize user streak: %w", err)
		}
		createdStreak = st

		_, err = txStore.CreateUserToken(ctx, store.CreateUserTokenParams{
			ID:         tokenID,
			UserID:     userID,
			TokenType:  store.TokenTypeEnumRefresh,
			TokenHash:  tokenHash,
			DeviceInfo: pgtype.Text{String: "Initial Registration", Valid: true},
			ExpiresAt:  pgtype.Timestamptz{Time: refreshExpiry, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to store refresh token: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("registration transaction failed: %w", err)
	}

	accessToken, err := crypto.GenerateAccessToken(
		userID,
		createdUser.Email,
		s.config.Auth.JWTSecret,
		s.config.Auth.AccessExpiry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	if s.emailSender != nil {
		verifyToken, _ := crypto.GenerateRandomString(32)
		vTokenHash := crypto.HashToken(verifyToken)
		vTokenID, _ := uuid.NewV7()
		_, _ = s.store.CreateUserToken(ctx, store.CreateUserTokenParams{
			ID:        vTokenID,
			UserID:    userID,
			TokenType: store.TokenTypeEnumEmailVerification,
			TokenHash: vTokenHash,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true},
		})
		verifyURL := fmt.Sprintf("https://bioverse.id/verify-email?token=%s", verifyToken)
		_ = s.emailSender.SendVerificationEmail(ctx, in.Email, in.FullName, verifyURL)
	}

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		ExpiresIn:    int(s.config.Auth.AccessExpiry.Seconds()),
		User: UserSummary{
			ID:                    createdUser.ID,
			FullName:              createdUser.FullName,
			Email:                 createdUser.Email,
			Timezone:              createdUser.Timezone,
			PreferredCurrencyCode: createdUser.PreferredCurrencyCode,
			PointsBalance:         createdUser.PointsBalance,
			CurrentStreak:         int(createdStreak.CurrentStreak),
		},
	}, nil
}

// Login verifies credentials and issues a fresh access token and rotating refresh token.
func (s *Service) Login(ctx context.Context, in LoginInput) (*AuthResult, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}

	if !user.PasswordHash.Valid {
		return nil, ErrInvalidCredentials
	}

	if err := crypto.VerifyPassword(in.Password, user.PasswordHash.String); err != nil {
		return nil, ErrInvalidCredentials
	}

	rawRefreshToken, err := crypto.GenerateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	tokenHash := crypto.HashToken(rawRefreshToken)

	tokenID, err := uuid.NewV7()
	if err != nil {
		tokenID = uuid.New()
	}
	refreshExpiry := time.Now().UTC().Add(s.config.Auth.RefreshExpiry)

	_, err = s.store.CreateUserToken(ctx, store.CreateUserTokenParams{
		ID:         tokenID,
		UserID:     user.ID,
		TokenType:  store.TokenTypeEnumRefresh,
		TokenHash:  tokenHash,
		DeviceInfo: pgtype.Text{String: in.DeviceInfo, Valid: in.DeviceInfo != ""},
		ExpiresAt:  pgtype.Timestamptz{Time: refreshExpiry, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store login refresh token: %w", err)
	}

	accessToken, err := crypto.GenerateAccessToken(
		user.ID,
		user.Email,
		s.config.Auth.JWTSecret,
		s.config.Auth.AccessExpiry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	streak, err := s.store.GetUserStreakByUserID(ctx, user.ID)
	currentStreak := 0
	if err == nil {
		currentStreak = int(streak.CurrentStreak)
	}

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		ExpiresIn:    int(s.config.Auth.AccessExpiry.Seconds()),
		User: UserSummary{
			ID:                    user.ID,
			FullName:              user.FullName,
			Email:                 user.Email,
			Timezone:              user.Timezone,
			PreferredCurrencyCode: user.PreferredCurrencyCode,
			PointsBalance:         user.PointsBalance,
			CurrentStreak:         currentStreak,
		},
	}, nil
}

// RefreshToken validates an existing refresh token and performs atomic token rotation.
func (s *Service) RefreshToken(ctx context.Context, in RefreshInput) (*AuthResult, error) {
	tokenHash := crypto.HashToken(in.RefreshToken)

	var tokenRecord store.UserTokens
	var user store.Users
	var rawNewRefreshToken string

	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		rec, err := txStore.GetUserTokenByHashForUpdate(ctx, store.GetUserTokenByHashForUpdateParams{
			TokenHash: tokenHash,
			TokenType: store.TokenTypeEnumRefresh,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrTokenRevoked
			}
			return fmt.Errorf("failed to lookup token: %w", err)
		}
		tokenRecord = rec

		if tokenRecord.RevokedAt.Valid {
			// Breach detection: A revoked token is being used again! Revoke all refresh tokens for this user.
			_ = txStore.RevokeAllUserTokensForUser(ctx, store.RevokeAllUserTokensForUserParams{
				UserID:    tokenRecord.UserID,
				TokenType: store.TokenTypeEnumRefresh,
			})
			return ErrTokenRevoked
		}

		if time.Now().UTC().After(tokenRecord.ExpiresAt.Time) {
			return ErrTokenExpired
		}

		var genErr error
		rawNewRefreshToken, genErr = crypto.GenerateRandomString(32)
		if genErr != nil {
			return fmt.Errorf("failed to generate rotated refresh token: %w", genErr)
		}
		newTokenHash := crypto.HashToken(rawNewRefreshToken)
		newTokenID, genErr := uuid.NewV7()
		if genErr != nil {
			newTokenID = uuid.New()
		}
		newRefreshExpiry := time.Now().UTC().Add(s.config.Auth.RefreshExpiry)

		if err := txStore.MarkUserTokenUsed(ctx, tokenRecord.ID); err != nil {
			return fmt.Errorf("failed to mark token as used: %w", err)
		}
		if err := txStore.RevokeUserToken(ctx, tokenRecord.ID); err != nil {
			return fmt.Errorf("failed to revoke old token: %w", err)
		}

		_, err = txStore.CreateUserToken(ctx, store.CreateUserTokenParams{
			ID:         newTokenID,
			UserID:     tokenRecord.UserID,
			TokenType:  store.TokenTypeEnumRefresh,
			TokenHash:  newTokenHash,
			DeviceInfo: pgtype.Text{String: in.DeviceInfo, Valid: in.DeviceInfo != ""},
			ExpiresAt:  pgtype.Timestamptz{Time: newRefreshExpiry, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to insert new refresh token: %w", err)
		}

		u, err := txStore.GetUserByID(ctx, tokenRecord.UserID)
		if err != nil {
			return fmt.Errorf("failed to get user info: %w", err)
		}
		user = u

		return nil
	})
	if err != nil {
		if errors.Is(err, ErrTokenRevoked) || errors.Is(err, ErrTokenExpired) {
			return nil, err
		}
		return nil, fmt.Errorf("token rotation transaction failed: %w", err)
	}

	accessToken, err := crypto.GenerateAccessToken(
		user.ID,
		user.Email,
		s.config.Auth.JWTSecret,
		s.config.Auth.AccessExpiry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	streak, err := s.store.GetUserStreakByUserID(ctx, user.ID)
	currentStreak := 0
	if err == nil {
		currentStreak = int(streak.CurrentStreak)
	}

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawNewRefreshToken,
		ExpiresIn:    int(s.config.Auth.AccessExpiry.Seconds()),
		User: UserSummary{
			ID:                    user.ID,
			FullName:              user.FullName,
			Email:                 user.Email,
			Timezone:              user.Timezone,
			PreferredCurrencyCode: user.PreferredCurrencyCode,
			PointsBalance:         user.PointsBalance,
			CurrentStreak:         currentStreak,
		},
	}, nil
}

// GetUserProfile retrieves the full profile of an authenticated user.
func (s *Service) GetUserProfile(ctx context.Context, userID uuid.UUID) (*UserSummary, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to load user profile: %w", err)
	}

	streak, err := s.store.GetUserStreakByUserID(ctx, user.ID)
	currentStreak := 0
	if err == nil {
		currentStreak = int(streak.CurrentStreak)
	}

	var phone *string
	if user.PhoneNumber.Valid {
		phone = &user.PhoneNumber.String
	}
	var avatar *string
	if user.AvatarUrl.Valid {
		avatar = &user.AvatarUrl.String
	}
	var genderStr *string
	if user.Gender.Valid {
		g := string(user.Gender.UserGender)
		genderStr = &g
	}
	var dobStr *string
	if user.DateOfBirth.Valid {
		d := user.DateOfBirth.Time.Format("2006-01-02")
		dobStr = &d
	}
	referral := ""
	if user.ReferralCode.Valid {
		referral = user.ReferralCode.String
	}

	return &UserSummary{
		ID:                    user.ID,
		FullName:              user.FullName,
		Email:                 user.Email,
		PhoneNumber:           phone,
		Gender:                genderStr,
		DateOfBirth:           dobStr,
		Timezone:              user.Timezone,
		PreferredCurrencyCode: user.PreferredCurrencyCode,
		AvatarURL:             avatar,
		ReferralCode:          referral,
		PointsBalance:         user.PointsBalance,
		LockedPointsBalance:   user.LockedPointsBalance,
		IsEmailVerified:       user.IsEmailVerified,
		CurrentStreak:         currentStreak,
		CreatedAt:             user.CreatedAt.Time,
	}, nil
}

// validateGoogleIDToken cryptographically validates a Google OIDC ID token structure, signature, and claims.
func (s *Service) validateGoogleIDToken(ctx context.Context, idToken string) (string, string, error) {
	if strings.TrimSpace(idToken) == "" {
		return "", "", errors.New("google ID token must not be empty")
	}

	verifier := s.googleVerifier
	if verifier == nil {
		verifier = idtoken.Validate
	}

	payload, err := verifier(ctx, idToken, s.config.OAuth.GoogleClientID)
	if err != nil {
		return "", "", fmt.Errorf("cryptographic Google ID token verification failed: %w", err)
	}
	if payload == nil {
		return "", "", errors.New("empty Google ID token payload")
	}

	// Verify issuer
	if payload.Issuer != "accounts.google.com" && payload.Issuer != "https://accounts.google.com" {
		return "", "", errors.New("invalid Google ID token issuer")
	}

	// Verify expiration
	if payload.Expires > 0 && time.Now().UTC().Unix() > payload.Expires {
		return "", "", errors.New("google ID token has expired")
	}

	// If GoogleClientID is configured, verify audience matches
	if s.config.OAuth.GoogleClientID != "" && payload.Audience != s.config.OAuth.GoogleClientID {
		return "", "", errors.New("google ID token audience mismatch")
	}

	emailRaw, ok := payload.Claims["email"].(string)
	if !ok || strings.TrimSpace(emailRaw) == "" {
		return "", "", errors.New("google ID token missing email claim")
	}
	email := strings.ToLower(strings.TrimSpace(emailRaw))
	if _, err := mail.ParseAddress(email); err != nil {
		return "", "", fmt.Errorf("invalid email address in Google ID token: %w", err)
	}

	if ev, ok := payload.Claims["email_verified"]; ok {
		switch v := ev.(type) {
		case bool:
			if !v {
				return "", "", errors.New("google email is not verified")
			}
		case string:
			if strings.ToLower(v) != "true" {
				return "", "", errors.New("google email is not verified")
			}
		}
	} else {
		return "", "", errors.New("google ID token missing email_verified claim")
	}

	fullName, _ := payload.Claims["name"].(string)
	if strings.TrimSpace(fullName) == "" {
		fullName = "Google User"
	}

	return email, fullName, nil
}

// LoginWithGoogle performs authentication via Google ID token.
func (s *Service) LoginWithGoogle(ctx context.Context, idToken string, deviceInfo string) (*AuthResult, error) {
	if strings.TrimSpace(idToken) == "" {
		return nil, errors.New("id_token must not be empty")
	}

	email, fullName, err := s.validateGoogleIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("google token validation failed: %w", err)
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return s.Register(ctx, RegisterInput{
			FullName:              fullName,
			Email:                 email,
			Password:              uuid.New().String() + "Aa1!",
			Timezone:              "Asia/Jakarta",
			PreferredCurrencyCode: "USD",
		})
	} else if err != nil {
		return nil, fmt.Errorf("failed to query google user: %w", err)
	}

	rawRefreshToken, err := crypto.GenerateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	tokenHash := crypto.HashToken(rawRefreshToken)
	tokenID, _ := uuid.NewV7()
	refreshExpiry := time.Now().UTC().Add(s.config.Auth.RefreshExpiry)

	_, err = s.store.CreateUserToken(ctx, store.CreateUserTokenParams{
		ID:         tokenID,
		UserID:     user.ID,
		TokenType:  store.TokenTypeEnumRefresh,
		TokenHash:  tokenHash,
		DeviceInfo: pgtype.Text{String: deviceInfo, Valid: deviceInfo != ""},
		ExpiresAt:  pgtype.Timestamptz{Time: refreshExpiry, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store google refresh token: %w", err)
	}

	accessToken, err := crypto.GenerateAccessToken(user.ID, user.Email, s.config.Auth.JWTSecret, s.config.Auth.AccessExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	streak, _ := s.store.GetUserStreakByUserID(ctx, user.ID)

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
		ExpiresIn:    int(s.config.Auth.AccessExpiry.Seconds()),
		User: UserSummary{
			ID:                    user.ID,
			FullName:              user.FullName,
			Email:                 user.Email,
			Timezone:              user.Timezone,
			PreferredCurrencyCode: user.PreferredCurrencyCode,
			PointsBalance:         user.PointsBalance,
			LockedPointsBalance:   user.LockedPointsBalance,
			IsEmailVerified:       user.IsEmailVerified,
			CurrentStreak:         int(streak.CurrentStreak),
			CreatedAt:             user.CreatedAt.Time,
		},
	}, nil
}

// ForgotPassword generates a password reset token for the specified email.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		// Silently succeed to prevent user enumeration
		return nil
	}

	resetToken, _ := crypto.GenerateRandomString(32)
	tokenHash := crypto.HashToken(resetToken)
	tokenID, _ := uuid.NewV7()

	_, err = s.store.CreateUserToken(ctx, store.CreateUserTokenParams{
		ID:        tokenID,
		UserID:    user.ID,
		TokenType: store.TokenTypeEnumPasswordReset,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(1 * time.Hour), Valid: true},
	})
	if err != nil {
		return err
	}

	if s.emailSender != nil {
		resetURL := fmt.Sprintf("https://bioverse.id/reset-password?token=%s", resetToken)
		_ = s.emailSender.SendPasswordResetEmail(ctx, user.Email, resetURL)
	}

	return nil
}

// ResetPassword resets password using a valid password_reset token.
func (s *Service) ResetPassword(ctx context.Context, token string, newPassword string) error {
	if len(newPassword) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	tokenHash := crypto.HashToken(token)
	tokenRec, err := s.store.GetUserTokenByHash(ctx, store.GetUserTokenByHashParams{
		TokenHash: tokenHash,
		TokenType: store.TokenTypeEnumPasswordReset,
	})
	if err != nil {
		return errors.New("invalid or expired reset token")
	}

	if tokenRec.RevokedAt.Valid || tokenRec.UsedAt.Valid || time.Now().UTC().After(tokenRec.ExpiresAt.Time) {
		return errors.New("reset token is expired or has already been used")
	}

	newHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)
		if err := txStore.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{
			ID:           tokenRec.UserID,
			PasswordHash: pgtype.Text{String: newHash, Valid: true},
		}); err != nil {
			return err
		}
		return txStore.MarkUserTokenUsed(ctx, tokenRec.ID)
	})
}

// VerifyEmail marks user email as verified using a verification token.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	tokenHash := crypto.HashToken(token)
	tokenRec, err := s.store.GetUserTokenByHash(ctx, store.GetUserTokenByHashParams{
		TokenHash: tokenHash,
		TokenType: store.TokenTypeEnumEmailVerification,
	})
	if err != nil {
		return errors.New("invalid or expired email verification token")
	}

	if tokenRec.RevokedAt.Valid || tokenRec.UsedAt.Valid || time.Now().UTC().After(tokenRec.ExpiresAt.Time) {
		return errors.New("verification token is expired or has already been used")
	}

	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)
		if err := txStore.VerifyUserEmail(ctx, tokenRec.UserID); err != nil {
			return err
		}
		return txStore.MarkUserTokenUsed(ctx, tokenRec.ID)
	})
}

// Logout revokes a user's refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := crypto.HashToken(refreshToken)
	tokenRec, err := s.store.GetUserTokenByHash(ctx, store.GetUserTokenByHashParams{
		TokenHash: tokenHash,
		TokenType: store.TokenTypeEnumRefresh,
	})
	if err != nil {
		return nil
	}
	return s.store.RevokeUserToken(ctx, tokenRec.ID)
}

// UpdateUserProfile updates editable personal details of an authenticated user.
func (s *Service) UpdateUserProfile(ctx context.Context, userID uuid.UUID, in UpdateProfileInput) (*UserSummary, error) {
	existingUser, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	fullName := existingUser.FullName
	if in.FullName != nil && strings.TrimSpace(*in.FullName) != "" {
		fullName = strings.TrimSpace(*in.FullName)
	}
	timezone := existingUser.Timezone
	if in.Timezone != nil && strings.TrimSpace(*in.Timezone) != "" {
		timezone = strings.TrimSpace(*in.Timezone)
	}
	currency := existingUser.PreferredCurrencyCode
	if in.PreferredCurrencyCode != nil && strings.TrimSpace(*in.PreferredCurrencyCode) != "" {
		currency = strings.TrimSpace(*in.PreferredCurrencyCode)
	}

	params := store.UpdateUserProfileParams{
		ID:                    userID,
		FullName:              fullName,
		Timezone:              timezone,
		PreferredCurrencyCode: currency,
		PhoneNumber:           existingUser.PhoneNumber,
		Gender:                existingUser.Gender,
		DateOfBirth:           existingUser.DateOfBirth,
		AvatarUrl:             existingUser.AvatarUrl,
	}

	if in.PhoneNumber != nil {
		params.PhoneNumber = pgtype.Text{String: strings.TrimSpace(*in.PhoneNumber), Valid: strings.TrimSpace(*in.PhoneNumber) != ""}
	}
	if in.Gender != nil {
		params.Gender = store.NullUserGender{UserGender: store.UserGender(strings.TrimSpace(*in.Gender)), Valid: strings.TrimSpace(*in.Gender) != ""}
	}
	if in.DateOfBirth != nil {
		if parsedDate, err := time.Parse("2006-01-02", strings.TrimSpace(*in.DateOfBirth)); err == nil {
			params.DateOfBirth = pgtype.Date{Time: parsedDate, Valid: true}
		}
	}
	if in.AvatarURL != nil {
		params.AvatarUrl = pgtype.Text{String: strings.TrimSpace(*in.AvatarURL), Valid: strings.TrimSpace(*in.AvatarURL) != ""}
	}

	_, err = s.store.UpdateUserProfile(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update user profile: %w", err)
	}

	return s.GetUserProfile(ctx, userID)
}

// RegisterFCMToken records or updates an FCM device token.
func (s *Service) RegisterFCMToken(ctx context.Context, userID uuid.UUID, fcmToken, deviceType string) error {
	tokenHash := crypto.HashToken(fcmToken)
	tokenID, _ := uuid.NewV7()

	err := s.store.UpsertFCMDeviceToken(ctx, store.UpsertFCMDeviceTokenParams{
		ID:             tokenID,
		UserID:         userID,
		TokenHash:      tokenHash,
		FcmDeviceToken: pgtype.Text{String: fcmToken, Valid: true},
		DeviceInfo:     pgtype.Text{String: deviceType, Valid: true},
	})
	if err != nil {
		return err
	}

	if s.pushNotifier != nil {
		_ = s.pushNotifier.SendPush(ctx, fcmToken, "Device Connected", "Your mobile device has been linked to BioVerse.", nil)
	}

	return nil
}

// AdminLogin authenticates platform back-office administrators with brute-force protection and real TOTP verification.
func (s *Service) AdminLogin(ctx context.Context, email, password, totpCode string) (*AdminAuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	admin, err := s.store.GetPlatformAdminByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !admin.IsActive {
		return nil, ErrInvalidCredentials
	}

	if admin.LockedUntil.Valid && time.Now().UTC().Before(admin.LockedUntil.Time) {
		return nil, ErrAccountLocked
	}

	if err := crypto.VerifyPassword(password, admin.PasswordHash); err != nil {
		_, _ = s.store.RecordAdminLoginFailure(ctx, admin.ID)
		return nil, ErrInvalidCredentials
	}

	if admin.MfaEnabled {
		if totpCode == "" {
			return nil, ErrMfaRequired
		}
		if len(admin.MfaSecretEncrypted) == 0 {
			return nil, fmt.Errorf("admin has MFA enabled but no secret on record")
		}
		secretBytes, err := crypto.Decrypt(admin.MfaSecretEncrypted, s.config.Auth.AppEncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt MFA secret: %w", err)
		}
		if !crypto.VerifyTOTPCode(string(secretBytes), totpCode, time.Now().UTC()) {
			_, _ = s.store.RecordAdminLoginFailure(ctx, admin.ID)
			return nil, ErrInvalidTOTPCode
		}
	}

	// Login succeeded: reset failure counter and update last_login_at
	_ = s.store.RecordAdminLoginSuccess(ctx, admin.ID)

	accessToken, err := crypto.GenerateAdminAccessToken(admin.ID, admin.Email, string(admin.Role), s.config.Auth.JWTSecret, s.config.Auth.AccessExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to sign admin access token: %w", err)
	}

	rawRefresh, _ := crypto.GenerateRandomString(32)
	refreshHash := crypto.HashToken(rawRefresh)
	adminTokenID, _ := uuid.NewV7()

	_, err = s.store.CreateAdminToken(ctx, store.CreateAdminTokenParams{
		ID:        adminTokenID,
		AdminID:   admin.ID,
		TokenType: store.TokenTypeEnumRefresh,
		TokenHash: refreshHash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(s.config.Auth.RefreshExpiry), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to store admin refresh token: %w", err)
	}

	return &AdminAuthResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    int(s.config.Auth.AccessExpiry.Seconds()),
		AdminRole:    string(admin.Role),
	}, nil
}

// AdminRefresh rotates an admin refresh token and issues a new access token.
func (s *Service) AdminRefresh(ctx context.Context, rawRefreshToken string) (*AdminAuthResult, error) {
	if rawRefreshToken == "" {
		return nil, ErrInvalidRefreshToken
	}

	tokenHash := crypto.HashToken(rawRefreshToken)

	var res *AdminAuthResult
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)
		tokenRecord, err := txStore.GetAdminTokenByHashForUpdate(ctx, store.GetAdminTokenByHashForUpdateParams{
			TokenHash: tokenHash,
			TokenType: store.TokenTypeEnumRefresh,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalidRefreshToken
			}
			return fmt.Errorf("failed to lookup admin token: %w", err)
		}

		if tokenRecord.RevokedAt.Valid || tokenRecord.UsedAt.Valid {
			return ErrInvalidRefreshToken
		}

		if time.Now().UTC().After(tokenRecord.ExpiresAt.Time) {
			return ErrRefreshTokenExpired
		}

		if err := txStore.MarkAdminTokenUsed(ctx, tokenRecord.ID); err != nil {
			return err
		}
		if err := txStore.RevokeAdminToken(ctx, tokenHash); err != nil {
			return err
		}

		accessToken, err := crypto.GenerateAdminAccessToken(tokenRecord.AdminID, tokenRecord.Email, string(tokenRecord.Role), s.config.Auth.JWTSecret, s.config.Auth.AccessExpiry)
		if err != nil {
			return fmt.Errorf("failed to sign admin token: %w", err)
		}

		newRawRefresh, err := crypto.GenerateRandomString(32)
		if err != nil {
			return fmt.Errorf("failed to generate new admin refresh token: %w", err)
		}
		newRefreshHash := crypto.HashToken(newRawRefresh)
		newTokenID, err := uuid.NewV7()
		if err != nil {
			newTokenID = uuid.New()
		}

		_, err = txStore.CreateAdminToken(ctx, store.CreateAdminTokenParams{
			ID:        newTokenID,
			AdminID:   tokenRecord.AdminID,
			TokenType: store.TokenTypeEnumRefresh,
			TokenHash: newRefreshHash,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(s.config.Auth.RefreshExpiry), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to store rotated admin refresh token: %w", err)
		}

		res = &AdminAuthResult{
			AccessToken:  accessToken,
			RefreshToken: newRawRefresh,
			ExpiresIn:    int(s.config.Auth.AccessExpiry.Seconds()),
			AdminRole:    string(tokenRecord.Role),
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) || errors.Is(err, ErrRefreshTokenExpired) {
			return nil, err
		}
		return nil, fmt.Errorf("admin token refresh failed: %w", err)
	}

	return res, nil
}

// AdminLogout revokes an active admin refresh token.
func (s *Service) AdminLogout(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	tokenHash := crypto.HashToken(rawRefreshToken)
	return s.store.RevokeAdminToken(ctx, tokenHash)
}

// AdminMFASetupResult contains the generated TOTP configuration for enrollment.
type AdminMFASetupResult struct {
	Secret     string `json:"secret"`
	QRCodeURI  string `json:"qr_code_uri"`
	Account    string `json:"account"`
}

// AdminSetupMFA generates a new TOTP secret for an administrator and stores it encrypted.
func (s *Service) AdminSetupMFA(ctx context.Context, adminID uuid.UUID) (*AdminMFASetupResult, error) {
	admin, err := s.store.GetPlatformAdminByID(ctx, adminID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if admin.MfaEnabled {
		return nil, ErrMfaAlreadyEnabled
	}

	secret, err := crypto.GenerateTOTPSecret()
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP secret: %w", err)
	}

	encryptedSecret, err := crypto.Encrypt([]byte(secret), s.config.Auth.AppEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt TOTP secret: %w", err)
	}

	err = s.store.UpdateAdminMFASecret(ctx, store.UpdateAdminMFASecretParams{
		ID:                 adminID,
		MfaSecretEncrypted: encryptedSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save MFA secret: %w", err)
	}

	otpauthURI := crypto.GenerateTOTPURI(secret, admin.Email, "BioVerse")

	return &AdminMFASetupResult{
		Secret:    secret,
		QRCodeURI: otpauthURI,
		Account:   admin.Email,
	}, nil
}

// AdminVerifyMFA confirms enrollment by validating the first TOTP code and activates MFA.
func (s *Service) AdminVerifyMFA(ctx context.Context, adminID uuid.UUID, code string) error {
	admin, err := s.store.GetPlatformAdminByID(ctx, adminID)
	if err != nil {
		return ErrInvalidCredentials
	}

	if len(admin.MfaSecretEncrypted) == 0 {
		return errors.New("MFA has not been setup yet")
	}

	secretBytes, err := crypto.Decrypt(admin.MfaSecretEncrypted, s.config.Auth.AppEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt MFA secret: %w", err)
	}

	if !crypto.VerifyTOTPCode(string(secretBytes), code, time.Now().UTC()) {
		return ErrInvalidTOTPCode
	}

	return s.store.ConfirmAdminMFA(ctx, adminID)
}

// NotificationDTO formats user notifications for mobile and web clients.
type NotificationDTO struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Body             string  `json:"body"`
	NotificationType string  `json:"notification_type"`
	ReferenceType    *string `json:"reference_type,omitempty"`
	ReferenceID      *string `json:"reference_id,omitempty"`
	IsRead           bool    `json:"is_read"`
	ReadAt           *string `json:"read_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// ListUserNotifications returns latest notifications for the user.
func (s *Service) ListUserNotifications(ctx context.Context, userID uuid.UUID, limit int32) ([]NotificationDTO, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.store.ListNotificationsByUserID(ctx, store.ListNotificationsByUserIDParams{
		UserID: userID,
		Limit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list user notifications: %w", err)
	}

	result := make([]NotificationDTO, 0, len(rows))
	for _, r := range rows {
		item := NotificationDTO{
			ID:               r.ID.String(),
			Title:            r.Title,
			Body:             r.Body,
			NotificationType: r.NotificationType,
			IsRead:           r.IsRead,
			CreatedAt:        r.CreatedAt.Time.Format(time.RFC3339),
		}
		if r.ReferenceType.Valid {
			item.ReferenceType = &r.ReferenceType.String
		}
		if r.ReferenceID.Valid {
			refIDStr := uuid.UUID(r.ReferenceID.Bytes).String()
			item.ReferenceID = &refIDStr
		}
		if r.ReadAt.Valid {
			tStr := r.ReadAt.Time.Format(time.RFC3339)
			item.ReadAt = &tStr
		}
		result = append(result, item)
	}
	return result, nil
}

// ListUserNotificationsWithCursor returns paginated notifications for the user using cursor-based pagination.
func (s *Service) ListUserNotificationsWithCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]NotificationDTO, *string, error) {
	if limit <= 0 {
		limit = 20
	}
	var cursorParam pgtype.Timestamptz
	if cursor != nil {
		cursorParam = pgtype.Timestamptz{Time: *cursor, Valid: true}
	}

	fetchLimit := limit + 1
	rows, err := s.store.ListNotificationsByUserIDWithCursor(ctx, store.ListNotificationsByUserIDWithCursorParams{
		UserID:          userID,
		CursorCreatedAt: cursorParam,
		Limit:           int32(fetchLimit),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list user notifications with cursor: %w", err)
	}

	var nextCursor *string
	if len(rows) > limit {
		lastItem := rows[limit-1]
		next := lastItem.CreatedAt.Time.UTC().Format(time.RFC3339Nano)
		nextCursor = &next
		rows = rows[:limit]
	}

	result := make([]NotificationDTO, 0, len(rows))
	for _, r := range rows {
		item := NotificationDTO{
			ID:               r.ID.String(),
			Title:            r.Title,
			Body:             r.Body,
			NotificationType: r.NotificationType,
			IsRead:           r.IsRead,
			CreatedAt:        r.CreatedAt.Time.Format(time.RFC3339),
		}
		if r.ReferenceType.Valid {
			item.ReferenceType = &r.ReferenceType.String
		}
		if r.ReferenceID.Valid {
			refIDStr := uuid.UUID(r.ReferenceID.Bytes).String()
			item.ReferenceID = &refIDStr
		}
		if r.ReadAt.Valid {
			tStr := r.ReadAt.Time.Format(time.RFC3339)
			item.ReadAt = &tStr
		}
		result = append(result, item)
	}
	return result, nextCursor, nil
}

// UserStatsDTO represents aggregate metrics for a user's conservation actions and points.
type UserStatsDTO struct {
	UserID            string `json:"user_id"`
	CurrentPoints     int64  `json:"current_points"`
	CurrentStreak     int32  `json:"current_streak"`
	LongestStreak     int32  `json:"longest_streak"`
	TotalDonationsUSD string `json:"total_donations_usd"`
	TotalDonationsIDR string `json:"total_donations_idr"`
	TotalUnitsAdopted int64  `json:"total_units_adopted"`
	MissionsCompleted int64  `json:"missions_completed"`
}

// GetUserStats returns summary metrics of total donations, adopted units, points, streak, and missions.
func (s *Service) GetUserStats(ctx context.Context, userID uuid.UUID) (*UserStatsDTO, error) {
	row, err := s.store.GetUserStats(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to fetch user stats: %w", err)
	}

	usdVal := decimalutil.FromNumeric(row.TotalDonationsUsd).StringFixed(2)
	idrVal := decimalutil.FromNumeric(row.TotalDonationsIdr).StringFixed(2)

	return &UserStatsDTO{
		UserID:            row.UserID.String(),
		CurrentPoints:     row.CurrentPoints,
		CurrentStreak:     row.CurrentStreak,
		LongestStreak:     row.LongestStreak,
		TotalDonationsUSD: usdVal,
		TotalDonationsIDR: idrVal,
		TotalUnitsAdopted: row.TotalUnitsAdopted,
		MissionsCompleted: row.MissionsCompleted,
	}, nil
}

// MarkNotificationRead marks a specific user notification as read.
func (s *Service) MarkNotificationRead(ctx context.Context, userID, notificationID uuid.UUID) (*NotificationDTO, error) {
	r, err := s.store.MarkNotificationAsRead(ctx, store.MarkNotificationAsReadParams{
		ID:     notificationID,
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to mark notification as read: %w", err)
	}

	item := &NotificationDTO{
		ID:               r.ID.String(),
		Title:            r.Title,
		Body:             r.Body,
		NotificationType: r.NotificationType,
		IsRead:           r.IsRead,
		CreatedAt:        r.CreatedAt.Time.Format(time.RFC3339),
	}
	if r.ReferenceType.Valid {
		item.ReferenceType = &r.ReferenceType.String
	}
	if r.ReferenceID.Valid {
		refIDStr := uuid.UUID(r.ReferenceID.Bytes).String()
		item.ReferenceID = &refIDStr
	}
	if r.ReadAt.Valid {
		tStr := r.ReadAt.Time.Format(time.RFC3339)
		item.ReadAt = &tStr
	}
	return item, nil
}

// MarkAllNotificationsRead marks all unread notifications of the user as read.
func (s *Service) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	return s.store.MarkAllNotificationsAsRead(ctx, userID)
}
