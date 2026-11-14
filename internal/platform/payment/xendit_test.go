package payment_test

import (
	"context"
	"testing"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/platform/payment"
	"github.com/shopspring/decimal"
)

func TestMockXenditClient(t *testing.T) {
	client := payment.NewXenditClient("", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	inv, err := client.CreateInvoice(ctx, "test-ext-123", decimal.NewFromInt(51200), "Adoption donation", "budi@bioverse.test")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if inv.Status != "PENDING" {
		t.Errorf("expected PENDING status, got %s", inv.Status)
	}
	if inv.Amount != 51200 {
		t.Errorf("expected 51200 amount, got %d", inv.Amount)
	}
	if inv.QRISPayload == "" {
		t.Errorf("expected non-empty QRIS payload")
	}
}
