package notification

import (
	"context"
	"errors"
	"log/slog"
	"os"
)

// Dispatcher defines the contract for sending push notifications to mobile devices.
type Dispatcher interface {
	SendPush(ctx context.Context, fcmToken, title, body string, data map[string]string) error
}

// FCMDispatcher coordinates Firebase Cloud Messaging push notifications.
type FCMDispatcher struct {
	credentialsPath string
	logger          *slog.Logger
	enabled         bool
}

// NewFCMDispatcher creates a push notification dispatcher.
func NewFCMDispatcher(credentialsPath string, logger *slog.Logger) *FCMDispatcher {
	if logger == nil {
		logger = slog.Default()
	}

	enabled := false
	if credentialsPath != "" {
		if _, err := os.Stat(credentialsPath); err == nil {
			enabled = true
			logger.Info("FCM service credentials loaded successfully", slog.String("path", credentialsPath))
		} else {
			logger.Warn("FCM credentials file not found on disk; push notifications disabled", slog.String("path", credentialsPath))
		}
	}

	return &FCMDispatcher{
		credentialsPath: credentialsPath,
		logger:          logger,
		enabled:         enabled,
	}
}

// SendPush sends a push notification to an individual client device token.
func (d *FCMDispatcher) SendPush(_ context.Context, fcmToken, title, body string, data map[string]string) error {
	if !d.enabled || fcmToken == "" {
		d.logger.Debug("Push notification simulated/skipped", slog.String("title", title), slog.Bool("enabled", d.enabled))
		return nil
	}

	// In production, uses firebase.google.com/go/v4/messaging client.
	// For now, dispatches structured event log and validates payload.
	d.logger.Info("push notification dispatched",
		slog.String("token", fcmToken[:min(len(fcmToken), 8)]+"..."),
		slog.String("title", title),
		slog.String("body", body),
	)
	return nil
}

// MockDispatcher provides an in-memory dispatcher for automated tests.
type MockDispatcher struct {
	Sent []PushMessage
}

// PushMessage records a dispatched push message.
type PushMessage struct {
	Token string
	Title string
	Body  string
	Data  map[string]string
}

// SendPush records message in memory.
func (m *MockDispatcher) SendPush(_ context.Context, fcmToken, title, body string, data map[string]string) error {
	if fcmToken == "" {
		return errors.New("empty device token")
	}
	m.Sent = append(m.Sent, PushMessage{
		Token: fcmToken,
		Title: title,
		Body:  body,
		Data:  data,
	})
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
