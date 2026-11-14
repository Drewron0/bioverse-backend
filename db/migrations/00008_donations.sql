-- +goose Up

CREATE TABLE IF NOT EXISTS donation_subscriptions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id UUID REFERENCES subscription_plans(id) ON DELETE SET NULL,
    project_id UUID REFERENCES conservation_projects(id) ON DELETE SET NULL,
    frequency subscription_frequency NOT NULL,
    duration_months INT,
    completed_cycles INT NOT NULL DEFAULT 0,
    unit_quantity INT NOT NULL DEFAULT 1,
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    amount_per_cycle NUMERIC(18,2) NOT NULL,
    payment_method payment_method_type NOT NULL,
    payment_provider VARCHAR(50) NOT NULL,
    gateway_customer_id_encrypted BYTEA,
    payment_token_encrypted BYTEA,
    failed_charge_count INT NOT NULL DEFAULT 0,
    last_failed_at TIMESTAMPTZ,
    status subscription_status NOT NULL DEFAULT 'pending_activation',
    pause_reason TEXT,
    paused_at TIMESTAMPTZ,
    last_reminder_sent_at TIMESTAMPTZ,
    resume_payment_url TEXT,
    next_billing_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sponsored_adoption_quotas (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    contract_id UUID NOT NULL REFERENCES b2b_contracts(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES conservation_projects(id) ON DELETE CASCADE,
    mission_id UUID,
    total_quota INT NOT NULL CHECK (total_quota > 0),
    used_quota INT NOT NULL DEFAULT 0 CHECK (used_quota <= total_quota),
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS donations (
    id UUID PRIMARY KEY,
    idempotency_key VARCHAR(64) UNIQUE NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES conservation_projects(id),
    subscription_id UUID REFERENCES donation_subscriptions(id) ON DELETE SET NULL,
    sponsored_quota_id UUID REFERENCES sponsored_adoption_quotas(id) ON DELETE SET NULL,
    cycle_number INT,
    donation_type donation_type NOT NULL,
    is_anonymous BOOLEAN NOT NULL DEFAULT false,
    unit_quantity INT NOT NULL DEFAULT 1,
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    gross_project_value NUMERIC(18,2) NOT NULL,
    money_amount_paid NUMERIC(18,2) NOT NULL,
    settlement_currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    settlement_amount NUMERIC(18,2) NOT NULL,
    fx_rate_snapshot NUMERIC(18,6) NOT NULL,
    gateway_fee_settlement NUMERIC(18,2) NOT NULL DEFAULT 0,
    money_payment_method payment_method_type NOT NULL,
    payment_provider VARCHAR(50) NOT NULL,
    is_mixed_payment BOOLEAN NOT NULL DEFAULT false,
    bank_code VARCHAR(20),
    va_number VARCHAR(50),
    qris_payload TEXT,
    points_used BIGINT NOT NULL DEFAULT 0,
    points_rate_snapshot NUMERIC(18,6) NOT NULL DEFAULT 0,
    points_monetary_equivalent NUMERIC(18,2) NOT NULL DEFAULT 0,
    is_points_locked BOOLEAN NOT NULL DEFAULT false,
    payment_status payment_status NOT NULL DEFAULT 'pending',
    external_gateway_ref VARCHAR(255) UNIQUE,
    paid_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_subscription_cycle UNIQUE (subscription_id, cycle_number)
);

CREATE TABLE IF NOT EXISTS coral_fragments (
    id UUID PRIMARY KEY,
    fragment_code VARCHAR(50) NOT NULL UNIQUE,
    nickname VARCHAR(100),
    project_id UUID NOT NULL REFERENCES conservation_projects(id) ON DELETE CASCADE,
    donation_id UUID REFERENCES donations(id) ON DELETE SET NULL,
    spider_frame_id UUID REFERENCES spider_frames(id) ON DELETE SET NULL,
    adopted_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    planted_by_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    species_name VARCHAR(100) NOT NULL,
    lifecycle_status fragment_lifecycle_status NOT NULL DEFAULT 'pending_planting',
    health_status environmental_health_status,
    latest_photo_url TEXT,
    planted_at TIMESTAMPTZ,
    last_monitored_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS coral_fragments;
DROP TABLE IF EXISTS donations;
DROP TABLE IF EXISTS sponsored_adoption_quotas;
DROP TABLE IF EXISTS donation_subscriptions;
