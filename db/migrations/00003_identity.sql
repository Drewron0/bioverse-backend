-- +goose Up

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    full_name VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    is_email_verified BOOLEAN NOT NULL DEFAULT false,
    password_hash TEXT,
    google_id VARCHAR(100) UNIQUE,
    phone_number VARCHAR(30),
    date_of_birth DATE,
    gender user_gender,
    timezone VARCHAR(50) NOT NULL DEFAULT 'Asia/Jakarta',
    preferred_currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code) DEFAULT 'USD',
    avatar_url TEXT,
    referral_code VARCHAR(20) UNIQUE,
    referred_by_user_id UUID REFERENCES users(id),
    points_balance BIGINT NOT NULL DEFAULT 0 CHECK (points_balance >= 0),
    locked_points_balance BIGINT NOT NULL DEFAULT 0 CHECK (locked_points_balance <= points_balance),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_referral_code ON users (referral_code);

CREATE TABLE IF NOT EXISTS user_streaks (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    current_streak INT NOT NULL DEFAULT 0,
    longest_streak INT NOT NULL DEFAULT 0,
    last_streak_date DATE,
    last_freeze_used_date DATE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_type token_type_enum NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    fcm_device_token TEXT,
    device_info VARCHAR(255),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_tokens_lookup ON user_tokens (token_hash, token_type);
CREATE INDEX IF NOT EXISTS idx_user_tokens_user ON user_tokens (user_id, token_type);

CREATE TABLE IF NOT EXISTS platform_admins (
    id UUID PRIMARY KEY,
    full_name VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role admin_role NOT NULL,
    mfa_enabled BOOLEAN NOT NULL DEFAULT false,
    mfa_secret_encrypted BYTEA,
    mfa_verified_at TIMESTAMPTZ,
    failed_login_count INT NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_platform_admins_email ON platform_admins (email);

CREATE TABLE IF NOT EXISTS admin_tokens (
    id UUID PRIMARY KEY,
    admin_id UUID NOT NULL REFERENCES platform_admins(id) ON DELETE CASCADE,
    token_type token_type_enum NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    device_info VARCHAR(255),
    ip_address VARCHAR(45),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_tokens_lookup ON admin_tokens (token_hash, token_type);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY,
    admin_id UUID REFERENCES platform_admins(id),
    user_id UUID REFERENCES users(id),
    operator_id UUID,
    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(100) NOT NULL,
    entity_id UUID,
    old_values JSONB,
    new_values JSONB,
    ip_address VARCHAR(45),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_audit_logs_actor CHECK (
        (CASE WHEN admin_id IS NOT NULL THEN 1 ELSE 0 END +
         CASE WHEN user_id IS NOT NULL THEN 1 ELSE 0 END +
         CASE WHEN operator_id IS NOT NULL THEN 1 ELSE 0 END) <= 1
    )
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs (entity_type, entity_id);

CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    notification_type VARCHAR(50) NOT NULL,
    reference_type VARCHAR(50),
    reference_id UUID,
    is_read BOOLEAN NOT NULL DEFAULT false,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications (user_id, is_read);

-- +goose Down
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS admin_tokens;
DROP TABLE IF EXISTS platform_admins;
DROP TABLE IF EXISTS user_tokens;
DROP TABLE IF EXISTS user_streaks;
DROP TABLE IF EXISTS users;
