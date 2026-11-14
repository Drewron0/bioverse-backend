package email_test

import (
	"context"
	"testing"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/platform/email"
)

func TestMockSender(t *testing.T) {
	mock := &email.MockSender{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := mock.SendVerificationEmail(ctx, "budi@bioverse.test", "Budi", "https://bioverse.id/verify?token=123")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(mock.SentEmails) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(mock.SentEmails))
	}

	if mock.SentEmails[0].To[0] != "budi@bioverse.test" {
		t.Errorf("expected recipient budi@bioverse.test, got %s", mock.SentEmails[0].To[0])
	}
}
