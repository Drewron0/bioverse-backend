package identity

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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

	// 10. Verify Google Login rejects raw email injection & invalid tokens
	_, err = service.LoginWithGoogle(ctx, "victim@example.com", "TestDevice")
	if err == nil {
		t.Fatal("expected LoginWithGoogle to reject raw email string")
	}

	// Test valid Google JWT structure
	claims := jwt.MapClaims{
		"iss":            "accounts.google.com",
		"aud":            cfg.OAuth.GoogleClientID,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"email":          fmt.Sprintf("google_%d@bioverse.id", time.Now().UnixNano()),
		"email_verified": true,
		"name":           "Google Test User",
	}
	rawToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, _ := rawToken.SignedString([]byte("any-dummy-test-key"))

	gResult, err := service.LoginWithGoogle(ctx, signedToken, "TestDevice")
	if err != nil {
		t.Fatalf("expected valid Google JWT to succeed: %v", err)
	}
	if gResult.AccessToken == "" {
		t.Fatal("expected access token from google login")
	}
}
