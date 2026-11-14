package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Sender defines the contract for dispatching transactional emails.
type Sender interface {
	Send(ctx context.Context, to, subject, htmlContent string) error
	SendVerificationEmail(ctx context.Context, to, fullName, verifyURL string) error
	SendPasswordResetEmail(ctx context.Context, to, resetURL string) error
}

// ResendClient dispatches emails via Resend transactional email API.
type ResendClient struct {
	apiKey     string
	fromEmail  string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewResendClient constructs a new Resend email client.
func NewResendClient(apiKey, fromEmail string, logger *slog.Logger) *ResendClient {
	if fromEmail == "" {
		fromEmail = "BioVerse <onboarding@resend.dev>"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ResendClient{
		apiKey:     apiKey,
		fromEmail:  fromEmail,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// Send dispatches an email using Resend API.
func (c *ResendClient) Send(ctx context.Context, to, subject, htmlContent string) error {
	if c.apiKey == "" {
		c.logger.Info("Resend API key unset; skipping live email dispatch", slog.String("to", to), slog.String("subject", subject))
		return nil
	}

	payload := resendRequest{
		From:    c.fromEmail,
		To:      []string{to},
		Subject: subject,
		HTML:    htmlContent,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("resend request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend returned non-2xx status code: %d", resp.StatusCode)
	}

	c.logger.Info("transactional email sent successfully", slog.String("to", to), slog.String("subject", subject))
	return nil
}

// SendVerificationEmail dispatches a stylized welcome & verification link.
func (c *ResendClient) SendVerificationEmail(ctx context.Context, to, fullName, verifyURL string) error {
	subject := "Selamat Datang di BioVerse — Verifikasi Akun Anda"
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; border: 1px solid #e0e0e0; border-radius: 8px;">
			<h2 style="color: #0d9488;">Halo, %s!</h2>
			<p>Terima kasih telah bergabung dengan <strong>BioVerse</strong>, platform restorasi ekosistem Ridge-to-Reef Indonesia.</p>
			<p>Silakan klik tombol di bawah ini untuk memverifikasi alamat email Anda dan mulai mengadopsi karang serta merawat virtual pet Anda:</p>
			<div style="text-align: center; margin: 30px 0;">
				<a href="%s" style="background-color: #0d9488; color: #ffffff; padding: 12px 24px; text-decoration: none; border-radius: 6px; font-weight: bold; display: inline-block;">Verifikasi Akun Saya</a>
			</div>
			<p style="color: #666; font-size: 13px;">Tautan ini berlaku selama 24 jam. Jika Anda tidak merasa mendaftar di BioVerse, silakan abaikan email ini.</p>
		</div>
	`, fullName, verifyURL)

	return c.Send(ctx, to, subject, html)
}

// SendPasswordResetEmail dispatches a secure password reset link.
func (c *ResendClient) SendPasswordResetEmail(ctx context.Context, to, resetURL string) error {
	subject := "BioVerse — Permintaan Pengaturan Ulang Kata Sandi"
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px; border: 1px solid #e0e0e0; border-radius: 8px;">
			<h2 style="color: #0d9488;">Pengaturan Ulang Kata Sandi</h2>
			<p>Kami menerima permintaan untuk mengatur ulang kata sandi akun BioVerse Anda.</p>
			<div style="text-align: center; margin: 30px 0;">
				<a href="%s" style="background-color: #0d9488; color: #ffffff; padding: 12px 24px; text-decoration: none; border-radius: 6px; font-weight: bold; display: inline-block;">Atur Ulang Kata Sandi</a>
			</div>
			<p style="color: #666; font-size: 13px;">Tautan ini berlaku selama 15 menit. Jika Anda tidak meminta pengaturan ulang kata sandi, akun Anda tetap aman dan tidak ada tindakan yang perlu dilakukan.</p>
		</div>
	`, resetURL)

	return c.Send(ctx, to, subject, html)
}

// MockSender provides an in-memory email sender for unit and integration testing.
type MockSender struct {
	SentEmails []resendRequest
}

// Send records the email in memory.
func (m *MockSender) Send(_ context.Context, to, subject, htmlContent string) error {
	if to == "" {
		return errors.New("recipient email required")
	}
	m.SentEmails = append(m.SentEmails, resendRequest{
		To:      []string{to},
		Subject: subject,
		HTML:    htmlContent,
	})
	return nil
}

// SendVerificationEmail records verification email.
func (m *MockSender) SendVerificationEmail(ctx context.Context, to, fullName, verifyURL string) error {
	return m.Send(ctx, to, "Verify", verifyURL)
}

// SendPasswordResetEmail records password reset email.
func (m *MockSender) SendPasswordResetEmail(ctx context.Context, to, resetURL string) error {
	return m.Send(ctx, to, "Reset", resetURL)
}
