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

-- name: UpdateUserProfile :one
UPDATE users
SET full_name = COALESCE($2, full_name),
    phone_number = COALESCE($3, phone_number),
    gender = COALESCE($4, gender),
    date_of_birth = COALESCE($5, date_of_birth),
    timezone = COALESCE($6, timezone),
    preferred_currency_code = COALESCE($7, preferred_currency_code),
    avatar_url = COALESCE($8, avatar_url)
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, full_name, email, is_email_verified, password_hash, google_id, 
          phone_number, date_of_birth, gender, timezone, preferred_currency_code, 
          avatar_url, referral_code, points_balance, locked_points_balance, created_at;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2
WHERE id = $1 AND deleted_at IS NULL;

-- name: VerifyUserEmail :exec
UPDATE users
SET is_email_verified = true
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpsertFCMDeviceToken :exec
INSERT INTO user_tokens (
    id, user_id, token_type, token_hash, fcm_device_token, device_info, expires_at
) VALUES (
    $1, $2, 'fcm_device', $3, $4, $5, NOW() + INTERVAL '1 year'
) ON CONFLICT (token_hash) DO UPDATE
SET fcm_device_token = EXCLUDED.fcm_device_token,
    device_info = EXCLUDED.device_info,
    expires_at = EXCLUDED.expires_at;

-- name: GetPlatformAdminByEmail :one
SELECT id, full_name, email, password_hash, role, mfa_enabled, mfa_secret_encrypted, 
       mfa_verified_at, failed_login_count, locked_until, last_login_at, is_active, created_at
FROM platform_admins
WHERE email = $1 AND is_active = true
LIMIT 1;

-- name: CreateAdminToken :one
INSERT INTO admin_tokens (
    id, admin_id, token_type, token_hash, device_info, ip_address, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id, admin_id, token_type, token_hash, expires_at;

-- name: GetAdminTokenByHash :one
SELECT at.id, at.admin_id, at.token_type, at.token_hash, at.expires_at, at.used_at, at.revoked_at,
       pa.email, pa.full_name, pa.role, pa.is_active
FROM admin_tokens at
JOIN platform_admins pa ON at.admin_id = pa.id
WHERE at.token_hash = $1 AND at.token_type = $2 AND at.revoked_at IS NULL AND at.used_at IS NULL
LIMIT 1;

-- name: RevokeAdminToken :exec
UPDATE admin_tokens
SET revoked_at = NOW()
WHERE token_hash = $1;

-- name: MarkAdminTokenUsed :exec
UPDATE admin_tokens
SET used_at = NOW()
WHERE id = $1;

-- name: GetPlatformAdminByID :one
SELECT id, full_name, email, password_hash, role, mfa_enabled, mfa_secret_encrypted, 
       mfa_verified_at, failed_login_count, locked_until, last_login_at, is_active, created_at
FROM platform_admins
WHERE id = $1 AND is_active = true
LIMIT 1;

-- name: RecordAdminLoginSuccess :exec
UPDATE platform_admins
SET failed_login_count = 0,
    locked_until = NULL,
    last_login_at = NOW()
WHERE id = $1;

-- name: RecordAdminLoginFailure :one
UPDATE platform_admins
SET failed_login_count = failed_login_count + 1,
    locked_until = CASE 
        WHEN failed_login_count + 1 >= 5 THEN NOW() + INTERVAL '15 minutes' 
        ELSE locked_until 
    END
WHERE id = $1
RETURNING failed_login_count, locked_until;

-- name: UpdateAdminMFASecret :exec
UPDATE platform_admins
SET mfa_secret_encrypted = $2,
    mfa_enabled = false
WHERE id = $1;

-- name: ConfirmAdminMFA :exec
UPDATE platform_admins
SET mfa_enabled = true,
    mfa_verified_at = NOW()
WHERE id = $1;
