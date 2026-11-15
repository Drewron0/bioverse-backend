package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/notification"
)

func TestMockDispatcher(t *testing.T) {
	mock := &notification.MockDispatcher{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := mock.SendPush(ctx, "fcm-sample-token-12345", "Coral Planted!", "Your adopted coral has been planted on Frame SF-BALI-001", map[string]string{
		"deep_link": "bioverse://coral/my",
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(mock.Sent) != 1 {
		t.Fatalf("expected 1 sent notification, got %d", len(mock.Sent))
	}

	if mock.Sent[0].Title != "Coral Planted!" {
		t.Errorf("expected title Coral Planted!, got %s", mock.Sent[0].Title)
	}
}
