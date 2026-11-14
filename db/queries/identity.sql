-- name: GetUserByEmail :one
SELECT id, full_name, email, is_email_verified, password_hash, google_id, 
       phone_number, date_of_birth, gender, timezone, preferred_currency_code, 
       avatar_url, referral_code, referred_by_user_id, points_balance, 
       locked_points_balance, created_at, deleted_at
FROM users
WHERE email = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: GetUserByID :one
SELECT id, full_name, email, is_email_verified, password_hash, google_id, 
       phone_number, date_of_birth, gender, timezone, preferred_currency_code, 
       avatar_url, referral_code, referred_by_user_id, points_balance, 
       locked_points_balance, created_at, deleted_at
FROM users
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (
    id, full_name, email, password_hash, timezone, preferred_currency_code, referral_code
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id, full_name, email, is_email_verified, timezone, preferred_currency_code, 
            referral_code, points_balance, locked_points_balance, created_at;

-- name: CreateUserStreak :one
INSERT INTO user_streaks (
    user_id, current_streak, longest_streak
) VALUES (
    $1, 0, 0
) RETURNING user_id, current_streak, longest_streak, last_streak_date, last_freeze_used_date, updated_at;

-- name: GetUserStreakByUserID :one
SELECT user_id, current_streak, longest_streak, last_streak_date, last_freeze_used_date, updated_at
FROM user_streaks
WHERE user_id = $1
LIMIT 1;

-- name: CreateUserToken :one
INSERT INTO user_tokens (
    id, user_id, token_type, token_hash, device_info, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id, user_id, token_type, token_hash, device_info, expires_at, used_at, revoked_at, created_at;

-- name: GetUserTokenByHash :one
SELECT id, user_id, token_type, token_hash, fcm_device_token, device_info, 
       expires_at, used_at, revoked_at, created_at
FROM user_tokens
WHERE token_hash = $1 AND token_type = $2
LIMIT 1;

-- name: MarkUserTokenUsed :exec
UPDATE user_tokens
SET used_at = NOW()
WHERE id = $1;

-- name: RevokeUserToken :exec
UPDATE user_tokens
SET revoked_at = NOW()
WHERE id = $1;

-- name: RevokeAllUserTokensForUser :exec
UPDATE user_tokens
SET revoked_at = NOW()
WHERE user_id = $1 AND token_type = $2 AND revoked_at IS NULL;
