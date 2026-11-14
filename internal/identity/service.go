package identity

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/platform/config"
	"github.com/Drewron01/bioverse-backend/internal/platform/crypto"
	"github.com/Drewron01/bioverse-backend/internal/platform/database"
	"github.com/Drewron01/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
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
	// ErrUserNotFound indicates the user does not exist.
	ErrUserNotFound = errors.New("user not found")
)

// RegisterInput represents the payload required to register a new user.
type RegisterInput struct {
	FullName string
	Email    string
	Password string
	Timezone string
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

// AuthResult holds authentication tokens and basic user profile information.
type AuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         UserSummary
}

// UserSummary contains essential public fields of an authenticated user.
type UserSummary struct {
	ID                    uuid.UUID
	FullName              string
	Email                 string
	Timezone              string
	PreferredCurrencyCode string
	PointsBalance         int64
	CurrentStreak         int
}

// Service provides business logic for identity, authentication, and user accounts.
type Service struct {
	pool   *pgxpool.Pool
	store  store.Querier
	config *config.Config
}

// NewService constructs a new identity service.
func NewService(pool *pgxpool.Pool, q store.Querier, cfg *config.Config) *Service {
	return &Service{
		pool:   pool,
		store:  q,
		config: cfg,
	}
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

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		u, err := txStore.CreateUser(ctx, store.CreateUserParams{
			ID:                    userID,
			FullName:              in.FullName,
			Email:                 in.Email,
			PasswordHash:          pgtype.Text{String: passwordHash, Valid: true},
			Timezone:              timezone,
			PreferredCurrencyCode: "USD",
			ReferralCode:          pgtype.Text{String: referralCode, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to insert user record: %w", err)
		}
		createdUser = u

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

	tokenRecord, err := s.store.GetUserTokenByHash(ctx, store.GetUserTokenByHashParams{
		TokenHash: tokenHash,
		TokenType: store.TokenTypeEnumRefresh,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTokenRevoked
		}
		return nil, fmt.Errorf("failed to lookup token: %w", err)
	}

	if tokenRecord.RevokedAt.Valid {
		_ = s.store.RevokeAllUserTokensForUser(ctx, store.RevokeAllUserTokensForUserParams{
			UserID:    tokenRecord.UserID,
			TokenType: store.TokenTypeEnumRefresh,
		})
		return nil, ErrTokenRevoked
	}

	if time.Now().UTC().After(tokenRecord.ExpiresAt.Time) {
		return nil, ErrTokenExpired
	}

	rawNewRefreshToken, err := crypto.GenerateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate rotated refresh token: %w", err)
	}
	newTokenHash := crypto.HashToken(rawNewRefreshToken)
	newTokenID, err := uuid.NewV7()
	if err != nil {
		newTokenID = uuid.New()
	}
	newRefreshExpiry := time.Now().UTC().Add(s.config.Auth.RefreshExpiry)

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

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

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("token rotation transaction failed: %w", err)
	}

	user, err := s.store.GetUserByID(ctx, tokenRecord.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
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

	return &UserSummary{
		ID:                    user.ID,
		FullName:              user.FullName,
		Email:                 user.Email,
		Timezone:              user.Timezone,
		PreferredCurrencyCode: user.PreferredCurrencyCode,
		PointsBalance:         user.PointsBalance,
		CurrentStreak:         currentStreak,
	}, nil
}
