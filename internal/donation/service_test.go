package donation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/conservation"
	"github.com/Drewron01/bioverse-backend/internal/identity"
	"github.com/Drewron01/bioverse-backend/internal/platform/config"
	"github.com/Drewron01/bioverse-backend/internal/platform/database"
	"github.com/Drewron01/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
)

func TestDonationAndCoralAdoptionEndToEnd(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("skipping test: config unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		t.Skipf("skipping test: database unavailable: %v", err)
	}
	defer pool.Close()

	queries := store.New(pool)
	identitySvc := identity.NewService(pool, queries, cfg)
	consSvc := conservation.NewService(pool, queries)
	donSvc := NewService(pool, queries, consSvc, nil)

	// 1. Create a fresh test user
	uniqueSuffix := time.Now().UnixNano()
	regResult, err := identitySvc.Register(ctx, identity.RegisterInput{
		FullName: "Donation Tester",
		Email:    fmt.Sprintf("donor_%d@bioverse.id", uniqueSuffix),
		Password: "Password1234!",
		Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("failed to register test user: %v", err)
	}
	userID := regResult.User.ID

	// 2. Fetch the seeded Biorock project
	projects, err := consSvc.ListProjects(ctx)
	if err != nil || len(projects) == 0 {
		t.Fatalf("expected seeded conservation projects, got %v", err)
	}
	project := projects[0]

	// 3. Test Coral-First Gating: Bio-Shield must be rejected because user owns 0 active corals
	_, err = donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey:   uuid.New().String(),
		UserID:           userID,
		ProjectID:        &project.ID,
		UnitQuantity:     1,
		IncludeBioShield: true,
		PaymentMethod:    store.PaymentMethodTypeQris,
	})
	if err != ErrCoralFirstGatingFailed {
		t.Fatalf("expected ErrCoralFirstGatingFailed, got %v", err)
	}

	// 4. Checkout coral adoption (1 fragment at $3.20)
	idempotencyKey := uuid.New().String()
	checkoutRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey:   idempotencyKey,
		UserID:           userID,
		ProjectID:        &project.ID,
		UnitQuantity:     1,
		IncludeBioShield: false,
		CoralNickname:    "My First Baby Coral",
		PaymentMethod:    store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	if checkoutRes.GrossProjectValue.String() != "3.2" && checkoutRes.GrossProjectValue.String() != "3.20" {
		t.Errorf("expected gross project value 3.20, got %s", checkoutRes.GrossProjectValue.String())
	}
	if checkoutRes.QRISPayload == "" {
		t.Error("expected non-empty QRIS payload for QRIS checkout")
	}

	// 5. Process payment simulation webhook
	paidDonation, err := donSvc.ProcessPaidDonation(ctx, checkoutRes.DonationID, nil, "My First Baby Coral")
	if err != nil {
		t.Fatalf("failed to process paid donation: %v", err)
	}
	if paidDonation.PaymentStatus != store.PaymentStatusPaid {
		t.Errorf("expected payment status paid, got %s", paidDonation.PaymentStatus)
	}

	// 6. Verify user now has an active adopted coral fragment
	corals, err := consSvc.ListUserAdoptedCorals(ctx, userID)
	if err != nil || len(corals) == 0 {
		t.Fatalf("expected adopted coral fragment, got %v", err)
	}
	if corals[0].Nickname.String != "My First Baby Coral" {
		t.Errorf("expected nickname 'My First Baby Coral', got %s", corals[0].Nickname.String)
	}

	// 7. Verify Coral-First Gating now passes for Bio-Shield add-on!
	bioShieldRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey:   uuid.New().String(),
		UserID:           userID,
		ProjectID:        &project.ID,
		UnitQuantity:     1,
		IncludeBioShield: true,
		CoralNickname:    "Protected Coral",
		PaymentMethod:    store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("expected Bio-Shield checkout to succeed after adopting coral, got %v", err)
	}

	// Total should be $3.20 + $1.80 = $5.00
	if bioShieldRes.GrossProjectValue.String() != "5" && bioShieldRes.GrossProjectValue.String() != "5.00" {
		t.Errorf("expected gross value 5.00, got %s", bioShieldRes.GrossProjectValue.String())
	}
}
