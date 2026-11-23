package donation

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/middleware"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Handler serves HTTP endpoints for donation checkouts and webhook processing.
type Handler struct {
	service           *Service
	webhookToken      string
	allowSimulatePaid bool
}

// NewHandler constructs a new donation HTTP handler.
func NewHandler(svc *Service, webhookToken string, allowSimulatePaid bool) *Handler {
	return &Handler{
		service:           svc,
		webhookToken:      webhookToken,
		allowSimulatePaid: allowSimulatePaid,
	}
}

// RegisterRoutes registers donation routes on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	checkoutLimiter := middleware.RateLimit(30, time.Minute)

	r.Route("/v1/app/donations", func(dr chi.Router) {
		dr.Use(authMiddleware)
		dr.With(checkoutLimiter).Post("/checkout", h.handleCheckout)
		dr.Get("/{id}", h.handleGetDonation)
		dr.Get("/", h.handleListHistory)
		dr.Get("/history", h.handleListHistory)
	})

	r.Route("/v1/webhooks/xendit", func(wr chi.Router) {
		wr.Post("/", h.handleXenditWebhook)
		if h.allowSimulatePaid {
			wr.Post("/simulate-paid", h.handleSimulatePaid)
		}
	})
}

type checkoutRequest struct {
	DonationType       string  `json:"donation_type"`
	ProjectID          *string `json:"project_id"`
	Quantity           int     `json:"quantity"`
	UnitQuantity       int     `json:"unit_quantity"`
	IncludeBioShield   bool    `json:"include_bio_shield"`
	TargetPetID        *string `json:"target_pet_id"`
	CoralNickname      string  `json:"coral_nickname"`
	PaymentMethodType  string  `json:"payment_method_type"`
	BankCode           string  `json:"bank_code"`
	SponsoredQuotaID   *string `json:"sponsored_quota_id"`
	PointsToBurn       int64   `json:"points_to_burn"`
	PointsToUse        int64   `json:"points_to_use"`
	PaymentMethod      string  `json:"payment_method"`
	IsAnonymous        bool    `json:"is_anonymous"`
}

type checkoutResponse struct {
	DonationID             string                 `json:"donation_id"`
	ProjectID              *string                `json:"project_id,omitempty"`
	ProjectTitle           *string                `json:"project_title,omitempty"`
	Quantity               int                    `json:"quantity,omitempty"`
	DonationType           string                 `json:"donation_type"`
	Status                 string                 `json:"status"`
	GrossAmount            string                 `json:"gross_amount"`
	DiscountAmount         string                 `json:"discount_amount"`
	NetPayableAmount       string                 `json:"net_payable_amount"`
	CurrencyCode           string                 `json:"currency_code"`
	SettlementCurrencyCode string                 `json:"settlement_currency_code"`
	SettlementAmount       string                 `json:"settlement_amount"`
	FxRateSnapshot         string                 `json:"fx_rate_snapshot"`
	PointsBurned           int64                  `json:"points_burned"`
	PaymentMethodType      string                 `json:"payment_method_type"`
	PaymentDetails         map[string]interface{} `json:"payment_details"`
	ExpiredAt              string                 `json:"expired_at"`
	CreatedAt              string                 `json:"created_at"`
}

func (h *Handler) handleCheckout(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		httperr.BadRequest(w, r, "Idempotency-Key header is required for checkout.", nil)
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	unitQty := req.UnitQuantity
	if unitQty <= 0 {
		unitQty = req.Quantity
	}
	if unitQty <= 0 {
		unitQty = 1
	}

	var projectID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		pID, err := uuid.Parse(*req.ProjectID)
		if err != nil {
			httperr.BadRequest(w, r, "Invalid project_id UUID format.", nil)
			return
		}
		projectID = &pID
	}

	var targetPetID *uuid.UUID
	if req.TargetPetID != nil && *req.TargetPetID != "" {
		petID, err := uuid.Parse(*req.TargetPetID)
		if err == nil {
			targetPetID = &petID
		}
	}

	var sponsoredQuotaID *uuid.UUID
	if req.SponsoredQuotaID != nil && *req.SponsoredQuotaID != "" {
		qID, err := uuid.Parse(*req.SponsoredQuotaID)
		if err == nil {
			sponsoredQuotaID = &qID
		}
	}

	payMethodStr := req.PaymentMethodType
	if payMethodStr == "" {
		payMethodStr = req.PaymentMethod
	}
	if payMethodStr == "" {
		payMethodStr = string(store.PaymentMethodTypeQris)
	}

	pointsToBurn := req.PointsToBurn
	if pointsToBurn <= 0 {
		pointsToBurn = req.PointsToUse
	}

	donationType := store.DonationType(req.DonationType)
	if string(donationType) == "" {
		donationType = store.DonationTypeCoralAdoption
	}

	result, err := h.service.Checkout(r.Context(), CheckoutInput{
		IdempotencyKey:   idempotencyKey,
		UserID:           userID,
		DonationType:     donationType,
		ProjectID:        projectID,
		UnitQuantity:     unitQty,
		IncludeBioShield: req.IncludeBioShield,
		TargetPetID:      targetPetID,
		CoralNickname:    req.CoralNickname,
		SponsoredQuotaID: sponsoredQuotaID,
		PointsToUse:      pointsToBurn,
		PaymentMethod:    store.PaymentMethodType(payMethodStr),
		BankCode:         req.BankCode,
		IsAnonymous:      req.IsAnonymous,
	})
	if err != nil {
		if errors.Is(err, ErrIdempotencyKeyReused) {
			httperr.ConflictWithCode(w, r, "IDEMPOTENCY_KEY_REUSED", "Idempotency key has already been used with different request parameters")
			return
		}
		if errors.Is(err, ErrCoralFirstGatingFailed) {
			httperr.Forbidden(w, r, err.Error())
			return
		}
		if errors.Is(err, ErrInsufficientPoints) {
			httperr.BadRequest(w, r, err.Error(), nil)
			return
		}
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	var qrCodeString *string
	if result.QRISPayload != "" {
		qrCodeString = &result.QRISPayload
	}
	var vaNumber *string
	if result.VANumber != "" {
		vaNumber = &result.VANumber
	}
	var bankCode *string
	if result.BankCode != "" {
		bankCode = &result.BankCode
	}
	var paymentURL *string
	if result.PaymentURL != "" {
		paymentURL = &result.PaymentURL
	}

	paymentDetails := map[string]interface{}{
		"qr_code_string": qrCodeString,
		"va_number":      vaNumber,
		"bank_code":      bankCode,
		"payment_url":    paymentURL,
	}

	var projIDStr *string
	if result.ProjectID != nil && *result.ProjectID != uuid.Nil {
		s := result.ProjectID.String()
		projIDStr = &s
	}
	var projTitleStr *string
	if result.ProjectTitle != "" {
		projTitleStr = &result.ProjectTitle
	}

	resp := checkoutResponse{
		DonationID:             result.DonationID.String(),
		ProjectID:              projIDStr,
		ProjectTitle:           projTitleStr,
		Quantity:               result.UnitQuantity,
		DonationType:           string(result.DonationType),
		Status:                 string(result.PaymentStatus),
		GrossAmount:            result.GrossProjectValue.StringFixed(2),
		DiscountAmount:         result.PointsDiscount.StringFixed(2),
		NetPayableAmount:       result.MoneyAmountPaid.StringFixed(2),
		CurrencyCode:           result.CurrencyCode,
		SettlementCurrencyCode: result.SettlementCurrency,
		SettlementAmount:       result.SettlementAmount.StringFixed(2),
		FxRateSnapshot:         result.FxRateSnapshot.StringFixed(6),
		PointsBurned:           result.PointsUsed,
		PaymentMethodType:      string(result.PaymentMethod),
		PaymentDetails:         paymentDetails,
		ExpiredAt:              result.ExpiredAt.Format(time.RFC3339),
		CreatedAt:              time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

type donationPaymentDetailsDTO struct {
	PaymentURL   *string `json:"payment_url"`
	QRCodeString *string `json:"qr_code_string"`
	VANumber     *string `json:"va_number"`
	BankCode     *string `json:"bank_code"`
}

type donationDetailDTO struct {
	ID                     string                     `json:"id"`
	DonationType           string                     `json:"donation_type"`
	Status                 string                     `json:"status"`
	ProjectID              *string                    `json:"project_id,omitempty"`
	ProjectTitle           *string                    `json:"project_title,omitempty"`
	ProjectCategory        *string                    `json:"project_category,omitempty"`
	ImageURL               *string                    `json:"image_url,omitempty"`
	LocationName           *string                    `json:"location_name,omitempty"`
	Quantity               int32                      `json:"quantity"`
	GrossAmount            string                     `json:"gross_amount"`
	NetPayableAmount       string                     `json:"net_payable_amount"`
	CurrencyCode           string                     `json:"currency_code"`
	SettlementCurrencyCode string                     `json:"settlement_currency_code"`
	SettlementAmount       string                     `json:"settlement_amount"`
	FxRateSnapshot         string                     `json:"fx_rate_snapshot"`
	PointsBurned           int64                      `json:"points_burned"`
	PaymentMethodType      string                     `json:"payment_method_type"`
	IsAnonymous            bool                       `json:"is_anonymous"`
	PaymentDetails         *donationPaymentDetailsDTO `json:"payment_details,omitempty"`
	PaidAt                 *string                    `json:"paid_at"`
	ExpiredAt              *string                    `json:"expired_at"`
	CreatedAt              string                     `json:"created_at"`
}

func mapDetailRowToDetail(d *store.GetDonationDetailByIDRow) donationDetailDTO {
	var paidAt *string
	if d.PaidAt.Valid {
		f := d.PaidAt.Time.UTC().Format(time.RFC3339)
		paidAt = &f
	}
	var expiredAt *string
	if d.ExpiredAt.Valid {
		f := d.ExpiredAt.Time.UTC().Format(time.RFC3339)
		expiredAt = &f
	}

	var paymentURL *string
	if d.PaymentUrl.Valid && d.PaymentUrl.String != "" {
		paymentURL = &d.PaymentUrl.String
	}
	var qrCodeString *string
	if d.QrisPayload.Valid && d.QrisPayload.String != "" {
		qrCodeString = &d.QrisPayload.String
	}
	var vaNumber *string
	if d.VaNumber.Valid && d.VaNumber.String != "" {
		vaNumber = &d.VaNumber.String
	}
	var bankCode *string
	if d.BankCode.Valid && d.BankCode.String != "" {
		bankCode = &d.BankCode.String
	}

	var projID *string
	if d.ProjectID != uuid.Nil {
		s := d.ProjectID.String()
		projID = &s
	}
	var projTitle *string
	if d.ProjectTitle.Valid {
		projTitle = &d.ProjectTitle.String
	}
	var projCat *string
	if d.ProjectCategory != "" {
		projCat = &d.ProjectCategory
	}
	var imgURL *string
	if d.ProjectImageUrl.Valid {
		imgURL = &d.ProjectImageUrl.String
	}
	var locName *string
	if d.ProjectLocationName.Valid {
		locName = &d.ProjectLocationName.String
	}

	return donationDetailDTO{
		ID:                     d.ID.String(),
		DonationType:           string(d.DonationType),
		Status:                 string(d.PaymentStatus),
		ProjectID:              projID,
		ProjectTitle:           projTitle,
		ProjectCategory:        projCat,
		ImageURL:               imgURL,
		LocationName:           locName,
		Quantity:               d.UnitQuantity,
		GrossAmount:            decimalutil.FromNumeric(d.GrossProjectValue).StringFixed(2),
		NetPayableAmount:       decimalutil.FromNumeric(d.MoneyAmountPaid).StringFixed(2),
		CurrencyCode:           d.CurrencyCode,
		SettlementCurrencyCode: d.SettlementCurrencyCode,
		SettlementAmount:       decimalutil.FromNumeric(d.SettlementAmount).StringFixed(2),
		FxRateSnapshot:         decimalutil.FromNumeric(d.FxRateSnapshot).StringFixed(6),
		PointsBurned:           d.PointsUsed,
		PaymentMethodType:      string(d.MoneyPaymentMethod),
		IsAnonymous:            d.IsAnonymous,
		PaymentDetails: &donationPaymentDetailsDTO{
			PaymentURL:   paymentURL,
			QRCodeString: qrCodeString,
			VANumber:     vaNumber,
			BankCode:     bankCode,
		},
		PaidAt:    paidAt,
		ExpiredAt: expiredAt,
		CreatedAt: d.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func mapDonationToDetail(d *store.Donations) donationDetailDTO {
	var paidAt *string
	if d.PaidAt.Valid {
		f := d.PaidAt.Time.UTC().Format(time.RFC3339)
		paidAt = &f
	}
	var expiredAt *string
	if d.ExpiredAt.Valid {
		f := d.ExpiredAt.Time.UTC().Format(time.RFC3339)
		expiredAt = &f
	}

	var paymentURL *string
	if d.PaymentUrl.Valid && d.PaymentUrl.String != "" {
		paymentURL = &d.PaymentUrl.String
	}
	var qrCodeString *string
	if d.QrisPayload.Valid && d.QrisPayload.String != "" {
		qrCodeString = &d.QrisPayload.String
	}
	var vaNumber *string
	if d.VaNumber.Valid && d.VaNumber.String != "" {
		vaNumber = &d.VaNumber.String
	}
	var bankCode *string
	if d.BankCode.Valid && d.BankCode.String != "" {
		bankCode = &d.BankCode.String
	}

	var projID *string
	if d.ProjectID != uuid.Nil {
		s := d.ProjectID.String()
		projID = &s
	}

	return donationDetailDTO{
		ID:                     d.ID.String(),
		DonationType:           string(d.DonationType),
		Status:                 string(d.PaymentStatus),
		ProjectID:              projID,
		Quantity:               d.UnitQuantity,
		GrossAmount:            decimalutil.FromNumeric(d.GrossProjectValue).StringFixed(2),
		NetPayableAmount:       decimalutil.FromNumeric(d.MoneyAmountPaid).StringFixed(2),
		CurrencyCode:           d.CurrencyCode,
		SettlementCurrencyCode: d.SettlementCurrencyCode,
		SettlementAmount:       decimalutil.FromNumeric(d.SettlementAmount).StringFixed(2),
		FxRateSnapshot:         decimalutil.FromNumeric(d.FxRateSnapshot).StringFixed(6),
		PointsBurned:           d.PointsUsed,
		PaymentMethodType:      string(d.MoneyPaymentMethod),
		IsAnonymous:            d.IsAnonymous,
		PaymentDetails: &donationPaymentDetailsDTO{
			PaymentURL:   paymentURL,
			QRCodeString: qrCodeString,
			VANumber:     vaNumber,
			BankCode:     bankCode,
		},
		PaidAt:    paidAt,
		ExpiredAt: expiredAt,
		CreatedAt: d.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func mapCursorRowToDetail(d *store.ListDonationsByUserIDWithCursorRow) donationDetailDTO {
	var paidAt *string
	if d.PaidAt.Valid {
		f := d.PaidAt.Time.UTC().Format(time.RFC3339)
		paidAt = &f
	}
	var expiredAt *string
	if d.ExpiredAt.Valid {
		f := d.ExpiredAt.Time.UTC().Format(time.RFC3339)
		expiredAt = &f
	}

	var paymentURL *string
	if d.PaymentUrl.Valid && d.PaymentUrl.String != "" {
		paymentURL = &d.PaymentUrl.String
	}
	var qrCodeString *string
	if d.QrisPayload.Valid && d.QrisPayload.String != "" {
		qrCodeString = &d.QrisPayload.String
	}
	var vaNumber *string
	if d.VaNumber.Valid && d.VaNumber.String != "" {
		vaNumber = &d.VaNumber.String
	}
	var bankCode *string
	if d.BankCode.Valid && d.BankCode.String != "" {
		bankCode = &d.BankCode.String
	}

	var projID *string
	if d.ProjectID != uuid.Nil {
		s := d.ProjectID.String()
		projID = &s
	}
	var projTitle *string
	if d.ProjectTitle.Valid {
		projTitle = &d.ProjectTitle.String
	}
	var projCat *string
	if d.ProjectCategory != "" {
		projCat = &d.ProjectCategory
	}
	var imgURL *string
	if d.ProjectImageUrl.Valid {
		imgURL = &d.ProjectImageUrl.String
	}
	var locName *string
	if d.ProjectLocationName.Valid {
		locName = &d.ProjectLocationName.String
	}

	return donationDetailDTO{
		ID:                     d.ID.String(),
		DonationType:           string(d.DonationType),
		Status:                 string(d.PaymentStatus),
		ProjectID:              projID,
		ProjectTitle:           projTitle,
		ProjectCategory:        projCat,
		ImageURL:               imgURL,
		LocationName:           locName,
		Quantity:               d.UnitQuantity,
		GrossAmount:            decimalutil.FromNumeric(d.GrossProjectValue).StringFixed(2),
		NetPayableAmount:       decimalutil.FromNumeric(d.MoneyAmountPaid).StringFixed(2),
		CurrencyCode:           d.CurrencyCode,
		SettlementCurrencyCode: d.SettlementCurrencyCode,
		SettlementAmount:       decimalutil.FromNumeric(d.SettlementAmount).StringFixed(2),
		FxRateSnapshot:         decimalutil.FromNumeric(d.FxRateSnapshot).StringFixed(6),
		PointsBurned:           d.PointsUsed,
		PaymentMethodType:      string(d.MoneyPaymentMethod),
		IsAnonymous:            d.IsAnonymous,
		PaymentDetails: &donationPaymentDetailsDTO{
			PaymentURL:   paymentURL,
			QRCodeString: qrCodeString,
			VANumber:     vaNumber,
			BankCode:     bankCode,
		},
		PaidAt:    paidAt,
		ExpiredAt: expiredAt,
		CreatedAt: d.CreatedAt.Time.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) handleGetDonation(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	idStr := chi.URLParam(r, "id")
	donationID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid donation ID format.", nil)
		return
	}

	donation, err := h.service.GetDonationDetail(r.Context(), donationID)
	if err != nil {
		if errors.Is(err, ErrDonationNotFound) {
			httperr.NotFound(w, r, "Donation record not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	if donation.UserID != userID {
		httperr.NotFound(w, r, "Donation record not found.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mapDetailRowToDetail(donation))
}

func (h *Handler) handleListHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		if l > 50 {
			limit = 50
		} else {
			limit = l
		}
	}

	var cursorTime *time.Time
	cursorStr := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursorStr != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, cursorStr); err == nil {
			cursorTime = &parsed
		} else if parsed, err := time.Parse(time.RFC3339, cursorStr); err == nil {
			cursorTime = &parsed
		}
	}

	items, nextCursor, err := h.service.ListUserDonationsWithCursor(r.Context(), userID, cursorTime, limit)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	respItems := make([]donationDetailDTO, 0, len(items))
	for i := range items {
		respItems = append(respItems, mapCursorRowToDetail(&items[i]))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       respItems,
		"next_cursor": nextCursor,
	})
}

type xenditCallbackPayload struct {
	ID         string          `json:"id"`
	ExternalID string          `json:"external_id"`
	Status     string          `json:"status"`
	Amount     decimal.Decimal `json:"amount"`
}

func (h *Handler) handleXenditWebhook(w http.ResponseWriter, r *http.Request) {
	callbackToken := r.Header.Get("x-callback-token")
	if callbackToken == "" {
		httperr.Unauthorized(w, r, "Missing x-callback-token webhook signature.")
		return
	}

	// Fail-closed: webhook verification token MUST be configured and match in constant time
	if strings.TrimSpace(h.webhookToken) == "" {
		httperr.InternalServerError(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(callbackToken), []byte(h.webhookToken)) != 1 {
		httperr.Unauthorized(w, r, "Invalid x-callback-token webhook signature.")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		httperr.BadRequest(w, r, "Failed to read webhook request body.", nil)
		return
	}

	var payload xenditCallbackPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		httperr.BadRequest(w, r, "Invalid webhook payload JSON.", nil)
		return
	}

	err = h.service.ProcessXenditInvoiceWebhook(r.Context(), payload.ID, payload.ExternalID, payload.Status, payload.Amount, bodyBytes)
	if err != nil {
		if errors.Is(err, ErrAmountMismatch) {
			httperr.BadRequest(w, r, err.Error(), nil)
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Webhook processed successfully.",
	})
}

type simulatePaidRequest struct {
	DonationID    string `json:"donation_id"`
	CoralNickname string `json:"coral_nickname,omitempty"`
}

func (h *Handler) handleSimulatePaid(w http.ResponseWriter, r *http.Request) {
	var req simulatePaidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	donationID, err := uuid.Parse(req.DonationID)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid donation_id UUID format.", nil)
		return
	}

	updatedDonation, err := h.service.ProcessPaidDonation(r.Context(), donationID, nil, req.CoralNickname)
	if err != nil {
		if errors.Is(err, ErrDonationNotFound) {
			httperr.NotFound(w, r, "Donation not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "paid_processed",
		"data":   updatedDonation,
	})
}
