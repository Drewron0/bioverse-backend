-- +goose Up

CREATE TABLE IF NOT EXISTS community_groups (
    id UUID PRIMARY KEY,
    zone_id UUID NOT NULL REFERENCES conservation_zones(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    group_type community_group_type NOT NULL,
    contact_phone VARCHAR(30) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS field_operators (
    id UUID PRIMARY KEY,
    zone_id UUID NOT NULL REFERENCES conservation_zones(id) ON DELETE CASCADE,
    group_id UUID REFERENCES community_groups(id) ON DELETE SET NULL,
    verified_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    full_name VARCHAR(100) NOT NULL,
    can_login BOOLEAN NOT NULL DEFAULT false,
    email VARCHAR(255) UNIQUE,
    phone_number VARCHAR(30) NOT NULL UNIQUE,
    password_hash TEXT,
    operator_role operator_role NOT NULL,
    nik_encrypted BYTEA,
    nik_hmac VARCHAR(64) UNIQUE,
    kyc_status kyc_verification_status NOT NULL DEFAULT 'pending',
    bank_code VARCHAR(20),
    bank_account_encrypted BYTEA,
    bank_account_holder VARCHAR(100),
    rate_per_planting_idr NUMERIC(18,2) NOT NULL DEFAULT 0,
    rate_per_maintenance_idr NUMERIC(18,2) NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS operator_tokens (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES field_operators(id) ON DELETE CASCADE,
    token_type token_type_enum NOT NULL,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    fcm_device_token TEXT,
    device_info VARCHAR(255),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS operator_tokens;
DROP TABLE IF EXISTS field_operators;
DROP TABLE IF EXISTS community_groups;
