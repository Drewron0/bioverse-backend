-- name: CreateFieldOperator :one
INSERT INTO field_operators (
    id, zone_id, group_id, verified_by_admin_id, full_name, can_login,
    email, phone_number, password_hash, operator_role, nik_encrypted,
    nik_hmac, kyc_status, bank_code, bank_account_encrypted, bank_account_holder,
    rate_per_planting_idr, rate_per_maintenance_idr, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17, $18, $19
) RETURNING *;

-- name: GetFieldOperatorByID :one
SELECT * FROM field_operators
WHERE id = $1
LIMIT 1;

-- name: GetFieldOperatorByNIKHash :one
SELECT * FROM field_operators
WHERE nik_hmac = $1
LIMIT 1;

-- name: ListFieldOperatorsByZone :many
SELECT * FROM field_operators
WHERE zone_id = $1 AND is_active = true
ORDER BY full_name ASC;

-- name: CreateOperatorWorkLog :one
INSERT INTO operator_work_logs (
    id, operator_id, entered_by_admin_id, project_id, spider_frame_id,
    coral_fragment_id, task_type, units_completed, fee_earned_idr,
    proof_image_url, verification_status, work_date
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12
) RETURNING *;

-- name: ListOperatorWorkLogs :many
SELECT owl.*, fo.full_name AS operator_name, cp.title AS project_title
FROM operator_work_logs owl
JOIN field_operators fo ON owl.operator_id = fo.id
JOIN conservation_projects cp ON owl.project_id = cp.id
WHERE owl.operator_id = $1
ORDER BY owl.work_date DESC
LIMIT $2 OFFSET $3;

-- name: CreateMediaEvidence :one
INSERT INTO media_evidences (
    id, project_id, spider_frame_id, coral_fragment_id, company_id,
    captured_by_operator_id, uploaded_by_admin_id, media_type,
    milestone_month, milestone_title, subtitle_metrics, media_url,
    gps_latitude, gps_longitude, depth_meters, is_verified_healthy, captured_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $15, $16, $17
) RETURNING *;

-- name: ListMediaEvidencesByFragmentID :many
SELECT * FROM media_evidences
WHERE coral_fragment_id = $1
ORDER BY milestone_month ASC, captured_at ASC;

-- name: ListMediaEvidencesByProjectID :many
SELECT * FROM media_evidences
WHERE project_id = $1
ORDER BY captured_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateOperatorPayout :one
INSERT INTO operator_payouts (
    id, idempotency_key, operator_id, approved_by_admin_id, period_start,
    period_end, total_units_planted, total_maintenance_tasks, gross_amount_idr,
    transfer_fee_idr, net_disbursed_idr, payout_gateway_provider, status
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13
) RETURNING *;

-- name: GetOperatorPayoutByID :one
SELECT * FROM operator_payouts
WHERE id = $1
LIMIT 1;

-- name: ListOperatorPayouts :many
SELECT op.*, fo.full_name AS operator_name
FROM operator_payouts op
JOIN field_operators fo ON op.operator_id = fo.id
WHERE op.operator_id = $1
ORDER BY op.created_at DESC;

-- name: UpdateOperatorPayoutStatus :one
UPDATE operator_payouts
SET status = $2,
    approved_by_admin_id = COALESCE(sqlc.narg('approved_by_admin_id'), approved_by_admin_id),
    approved_at = COALESCE(sqlc.narg('approved_at'), approved_at),
    disbursed_at = COALESCE(sqlc.narg('disbursed_at'), disbursed_at),
    external_disbursement_id = COALESCE(sqlc.narg('external_disbursement_id'), external_disbursement_id),
    failure_reason = COALESCE(sqlc.narg('failure_reason'), failure_reason)
WHERE id = $1
RETURNING *;

-- name: GetOperatorPayoutByIdempotencyKey :one
SELECT * FROM operator_payouts
WHERE idempotency_key = $1
LIMIT 1;

-- name: GetOperatorVerifiedWorkLogsSummary :one
SELECT 
    COALESCE(SUM(CASE WHEN task_type = 'planting' THEN units_completed ELSE 0 END), 0)::int AS units_planted,
    COALESCE(SUM(CASE WHEN task_type != 'planting' THEN units_completed ELSE 0 END), 0)::int AS maintenance_tasks,
    COALESCE(SUM(fee_earned_idr), 0)::numeric AS total_fee_earned_idr
FROM operator_work_logs
WHERE operator_id = $1
  AND work_date >= $2
  AND work_date <= $3
  AND verification_status = 'verified'
  AND payout_id IS NULL;

-- name: LinkWorkLogsToPayout :exec
UPDATE operator_work_logs
SET payout_id = $1
WHERE operator_id = $2
  AND work_date >= $3
  AND work_date <= $4
  AND verification_status = 'verified'
  AND payout_id IS NULL;

