package identity

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/crypto"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
)

func TestIdentityEndToEndFlow(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("skipping integration test: config could not be loaded")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		t.Skipf("skipping integration test: database not available: %v", err)
	}
	defer pool.Close()

	queries := store.New(pool)
	service := NewService(pool, queries, cfg, nil, nil)

	uniqueSuffix := time.Now().UnixNano()
	testEmail := fmt.Sprintf("testuser_%d@bioverse.id", uniqueSuffix)
	testPassword := "P@ssword1234!"

	// 1. Test Registration
	regResult, err := service.Register(ctx, RegisterInput{
		FullName: "Test BioVerse User",
		Email:    testEmail,
		Password: testPassword,
		Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	if regResult.AccessToken == "" || regResult.RefreshToken == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}
	if regResult.User.Email != testEmail {
		t.Errorf("expected email %s, got %s", testEmail, regResult.User.Email)
	}
	if regResult.User.PreferredCurrencyCode != "USD" {
		t.Errorf("expected default currency USD, got %s", regResult.User.PreferredCurrencyCode)
	}

	// 2. Test Duplicate Registration (should fail with conflict)
	_, err = service.Register(ctx, RegisterInput{
		FullName: "Duplicate User",
		Email:    testEmail,
		Password: testPassword,
		Timezone: "Asia/Jakarta",
	})
	if err == nil {
		t.Fatal("expected duplicate email registration to fail, but succeeded")
	}

	// 3. Test Login with Valid Credentials
	loginResult, err := service.Login(ctx, LoginInput{
		Email:      testEmail,
		Password:   testPassword,
		DeviceInfo: "Integration Test Device",
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loginResult.User.ID != regResult.User.ID {
		t.Errorf("expected login user ID %s, got %s", regResult.User.ID, loginResult.User.ID)
	}

	// 4. Test Login with Invalid Password
	_, err = service.Login(ctx, LoginInput{
		Email:    testEmail,
		Password: "WrongPassword!",
	})
	if err == nil {
		t.Fatal("expected login with invalid password to fail, but succeeded")
	}

	// 5. Test Token Refresh (Rotation)
	refreshResult, err := service.RefreshToken(ctx, RefreshInput{
		RefreshToken: loginResult.RefreshToken,
		DeviceInfo:   "Rotated Device",
	})
	if err != nil {
		t.Fatalf("token refresh failed: %v", err)
	}
	if refreshResult.RefreshToken == loginResult.RefreshToken {
		t.Fatal("expected rotated refresh token to be different from original")
	}

	// 6. Test Token Reuse (Old refresh token should now be rejected as revoked)
	_, err = service.RefreshToken(ctx, RefreshInput{
		RefreshToken: loginResult.RefreshToken,
	})
	if err == nil {
		t.Fatal("expected reuse of old refresh token to fail, but succeeded")
	}

	// 7. Test Get Profile
	profile, err := service.GetUserProfile(ctx, regResult.User.ID)
	if err != nil {
		t.Fatalf("get profile failed: %v", err)
	}
	if profile.Email != testEmail {
		t.Errorf("expected profile email %s, got %s", testEmail, profile.Email)
	}

	// 8. Test Notifications Flow
	notifID, _ := uuid.NewV7()
	_, err = queries.CreateNotification(ctx, store.CreateNotificationParams{
		ID:               notifID,
		UserID:           regResult.User.ID,
		Title:            "Welcome Guardian",
		Body:             "Thank you for joining BioVerse coral restoration initiative.",
		NotificationType: "announcement",
	})
	if err != nil {
		t.Fatalf("failed to create test notification: %v", err)
	}

	notifs, err := service.ListUserNotifications(ctx, regResult.User.ID, 10)
	if err != nil {
		t.Fatalf("failed to list notifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifs))
	}
	if notifs[0].IsRead {
		t.Errorf("expected notification to be unread initially")
	}

	markedNotif, err := service.MarkNotificationRead(ctx, regResult.User.ID, notifID)
	if err != nil {
		t.Fatalf("failed to mark notification read: %v", err)
	}
	if !markedNotif.IsRead {
		t.Errorf("expected notification to be marked as read")
	}

	err = service.MarkAllNotificationsRead(ctx, regResult.User.ID)
	if err != nil {
		t.Fatalf("failed to mark all notifications read: %v", err)
	}

	// 9. Verify partial profile update preserves existing fields
	newPhone := "+628123456789"
	updatedProfile, err := service.UpdateUserProfile(ctx, regResult.User.ID, UpdateProfileInput{
		PhoneNumber: &newPhone,
	})
	if err != nil {
		t.Fatalf("failed to update profile: %v", err)
	}
	if updatedProfile.FullName != regResult.User.FullName {
		t.Errorf("expected FullName to be preserved as %s, got %s", regResult.User.FullName, updatedProfile.FullName)
	}
	if updatedProfile.PreferredCurrencyCode != "USD" {
		t.Errorf("expected Currency to be preserved as USD, got %s", updatedProfile.PreferredCurrencyCode)
	}
	if updatedProfile.PhoneNumber == nil || *updatedProfile.PhoneNumber != newPhone {
		t.Errorf("expected PhoneNumber %s, got %v", newPhone, updatedProfile.PhoneNumber)
	}

	// 10. Verify Google Login rejects raw email injection and unverified/forged signatures
	_, err = service.LoginWithGoogle(ctx, "victim@example.com", "TestDevice")
	if err == nil {
		t.Fatal("expected LoginWithGoogle to reject raw email string")
	}

	forgedToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJhY2NvdW50cy5nb29nbGUuY29tIiwiZW1haWwiOiJ2aWN0aW1AZ21haWwuY29tIiwiZXhwIjoyMDAwMDAwMDAwfQ.invalid-signature"
	_, err = service.LoginWithGoogle(ctx, forgedToken, "TestDevice")
	if err == nil {
		t.Fatal("expected LoginWithGoogle to reject forged token with invalid signature")
	}

	// 11. Verify verified Google token payload succeeds with authentic verifier
	testGoogleEmail := fmt.Sprintf("google_%d@bioverse.id", time.Now().UnixNano())
	service.SetGoogleTokenVerifier(func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error) {
		if idToken != "valid-test-google-id-token" {
			return nil, errors.New("invalid token")
		}
		return &idtoken.Payload{
			Issuer:   "https://accounts.google.com",
			Audience: audience,
			Expires:  time.Now().Add(time.Hour).Unix(),
			Subject:  "1234567890",
			Claims: map[string]interface{}{
				"email":          testGoogleEmail,
				"email_verified": true,
				"name":           "Google Test User",
			},
		}, nil
	})

	gResult, err := service.LoginWithGoogle(ctx, "valid-test-google-id-token", "TestDevice")
	if err != nil {
		t.Fatalf("expected valid verified Google ID token to succeed: %v", err)
	}
	if gResult.AccessToken == "" {
		t.Fatal("expected access token from google login")
	}
	if gResult.User.Email != testGoogleEmail {
		t.Errorf("expected email %s, got %s", testGoogleEmail, gResult.User.Email)
	}

	// 12. Verify Admin MFA re-setup guard (cannot overwrite once enabled)
	adminID, _ := uuid.NewV7()
	adminHash, _ := crypto.HashPassword("AdminP@ss1234!")
	_, err = pool.Exec(ctx,
		"INSERT INTO platform_admins (id, full_name, email, password_hash, role) VALUES ($1, $2, $3, $4, $5)",
		adminID, "Test Admin MFA", fmt.Sprintf("admin_mfa_%d@bioverse.id", time.Now().UnixNano()), adminHash, store.AdminRoleSuperAdmin,
	)
	if err != nil {
		t.Fatalf("failed to insert test admin: %v", err)
	}

	mfaSetup1, err := service.AdminSetupMFA(ctx, adminID)
	if err != nil {
		t.Fatalf("first MFA setup failed: %v", err)
	}
	totpCode, err := crypto.GenerateTOTPCode(mfaSetup1.Secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("failed to generate TOTP code: %v", err)
	}
	if err := service.AdminVerifyMFA(ctx, adminID, totpCode); err != nil {
		t.Fatalf("MFA verify failed: %v", err)
	}

	// Now attempting setup again must fail
	_, err = service.AdminSetupMFA(ctx, adminID)
	if err == nil {
		t.Fatal("expected AdminSetupMFA to reject setup when MFA is already enabled")
	}
}
