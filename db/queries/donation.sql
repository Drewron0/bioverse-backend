-- name: CreateDonation :one
INSERT INTO donations (
    id, idempotency_key, user_id, project_id, subscription_id, sponsored_quota_id,
    cycle_number, donation_type, is_anonymous, unit_quantity, currency_code,
    gross_project_value, money_amount_paid, settlement_currency_code, settlement_amount,
    fx_rate_snapshot, gateway_fee_settlement, money_payment_method, payment_provider,
    is_mixed_payment, bank_code, va_number, qris_payload, points_used,
    points_rate_snapshot, points_monetary_equivalent, is_points_locked, payment_status,
    external_gateway_ref, expired_at, target_pet_id, coral_nickname, include_bio_shield, payment_url, paid_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18, $19,
    $20, $21, $22, $23, $24,
    $25, $26, $27, $28,
    $29, $30, $31, $32, $33, $34, $35
) RETURNING *;

-- name: GetDonationByID :one
SELECT * FROM donations
WHERE id = $1
LIMIT 1;

-- name: GetDonationByIdempotencyKey :one
SELECT * FROM donations
WHERE idempotency_key = $1
LIMIT 1;

-- name: UpdateDonationPaymentStatus :one
UPDATE donations
SET payment_status = $2,
    paid_at = $3,
    external_gateway_ref = COALESCE($4, external_gateway_ref)
WHERE id = $1 AND (payment_status = 'pending' OR payment_status = $2)
RETURNING *;

-- name: ListDonationsByUserID :many
SELECT d.*, cp.title AS project_title
FROM donations d
JOIN conservation_projects cp ON d.project_id = cp.id
WHERE d.user_id = $1
ORDER BY d.created_at DESC;

-- name: ListDonationsByUserIDWithCursor :many
SELECT d.*, cp.title AS project_title
FROM donations d
JOIN conservation_projects cp ON d.project_id = cp.id
WHERE d.user_id = $1
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL OR d.created_at < sqlc.narg('cursor_created_at')::timestamptz)
ORDER BY d.created_at DESC
LIMIT $2;

-- name: ListExpiredPendingDonations :many
SELECT * FROM donations
WHERE payment_status = 'pending' AND expired_at < NOW()
LIMIT $1;

-- name: MarkDonationExpired :one
UPDATE donations
SET payment_status = 'expired',
    is_points_locked = false
WHERE id = $1 AND payment_status = 'pending'
RETURNING *;
