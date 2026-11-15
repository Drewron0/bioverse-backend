package payment

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

// Gateway defines the interface for creating payment invoices and checking status.
type Gateway interface {
	CreateInvoice(ctx context.Context, externalID string, amount decimal.Decimal, description, payerEmail string) (*InvoiceResult, error)
}

// InvoiceResult represents the created invoice from payment gateway.
type InvoiceResult struct {
	ID          string    `json:"id"`
	ExternalID  string    `json:"external_id"`
	InvoiceURL  string    `json:"invoice_url"`
	Status      string    `json:"status"`
	Amount      int64     `json:"amount"`
	ExpiredAt   time.Time `json:"expired_at"`
	QRISPayload string    `json:"qris_payload,omitempty"`
	VANumber    string    `json:"va_number,omitempty"`
}

// XenditClient handles API interactions with Xendit (both Sandbox and Production).
type XenditClient struct {
	secretKey  string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewXenditClient constructs a Xendit API client.
func NewXenditClient(secretKey string, logger *slog.Logger) *XenditClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &XenditClient{
		secretKey:  secretKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		logger:     logger,
	}
}

type xenditInvoicePayload struct {
	ExternalID      string   `json:"external_id"`
	Amount          int64    `json:"amount"`
	Description     string   `json:"description"`
	PayerEmail      string   `json:"payer_email,omitempty"`
	InvoiceDuration int      `json:"invoice_duration"` // in seconds
	Currency        string   `json:"currency"`
	PaymentMethods  []string `json:"payment_methods,omitempty"`
}

type xenditInvoiceResponse struct {
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	InvoiceURL string `json:"invoice_url"`
	Status     string `json:"status"`
	Amount     int64  `json:"amount"`
	ExpiryDate string `json:"expiry_date"`
}

// CreateInvoice issues a new payment invoice on Xendit.
func (c *XenditClient) CreateInvoice(ctx context.Context, externalID string, amount decimal.Decimal, description, payerEmail string) (*InvoiceResult, error) {
	if c.secretKey == "" {
		// Mock invoice fallback when running without key
		now := time.Now().UTC()
		return &InvoiceResult{
			ID:          "mock-inv-" + externalID,
			ExternalID:  externalID,
			InvoiceURL:  "https://checkout-staging.xendit.co/web/mock-" + externalID,
			Status:      "PENDING",
			Amount:      amount.IntPart(),
			ExpiredAt:   now.Add(24 * time.Hour),
			QRISPayload: "00020101021226610014ID.LINKAJA.WWW01189360091800000000005204581253033605802ID5914BioVerse Restorasi6007Jakarta6304C14A",
			VANumber:    "88081234567890",
		}, nil
	}

	payload := xenditInvoicePayload{
		ExternalID:      externalID,
		Amount:          amount.IntPart(),
		Description:     description,
		PayerEmail:      payerEmail,
		InvoiceDuration: 86400, // 24 hours
		Currency:        "IDR",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal invoice payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.xendit.co/v2/invoices", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to construct xendit request: %w", err)
	}

	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(c.secretKey+":"))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xendit invoice request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("xendit api error: HTTP %d", resp.StatusCode)
	}

	var res xenditInvoiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to parse xendit response: %w", err)
	}

	expiry, _ := time.Parse(time.RFC3339, res.ExpiryDate)
	c.logger.Info("xendit invoice created", slog.String("id", res.ID), slog.String("external_id", externalID))

	return &InvoiceResult{
		ID:         res.ID,
		ExternalID: res.ExternalID,
		InvoiceURL: res.InvoiceURL,
		Status:     res.Status,
		Amount:     res.Amount,
		ExpiredAt:  expiry,
	}, nil
}
