-- +goose Up

CREATE TABLE IF NOT EXISTS digital_products (
    id UUID PRIMARY KEY,
    sku_code VARCHAR(50) NOT NULL UNIQUE,
    apple_product_id VARCHAR(100) UNIQUE,
    google_product_id VARCHAR(100) UNIQUE,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,
    item_type digital_item_type NOT NULL,
    price_points BIGINT NOT NULL DEFAULT 0,
    is_iap_only BOOLEAN NOT NULL DEFAULT false,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    preview_asset_url TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS product_prices (
    id UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES digital_products(id) ON DELETE CASCADE,
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    amount NUMERIC(18,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_product_prices_currency UNIQUE (product_id, currency_code)
);

CREATE TABLE IF NOT EXISTS user_digital_inventories (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES digital_products(id),
    quantity INT NOT NULL DEFAULT 1 CHECK (quantity >= 0),
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_inventories_product UNIQUE (user_id, product_id)
);

CREATE TABLE IF NOT EXISTS subscription_plans (
    id UUID PRIMARY KEY,
    sku_code VARCHAR(50) NOT NULL UNIQUE,
    group_code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,
    plan_type subscription_plan_type NOT NULL,
    billing_interval subscription_frequency NOT NULL,
    interval_count INT NOT NULL DEFAULT 1,
    duration_months INT,
    benefits JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active BOOLEAN NOT NULL DEFAULT true,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subscription_plan_prices (
    id UUID PRIMARY KEY,
    plan_id UUID NOT NULL REFERENCES subscription_plans(id) ON DELETE CASCADE,
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    amount NUMERIC(18,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_plan_prices_currency UNIQUE (plan_id, currency_code)
);

CREATE TABLE IF NOT EXISTS iap_transactions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES digital_products(id),
    store_provider iap_store_type NOT NULL,
    store_transaction_id VARCHAR(255) NOT NULL,
    original_transaction_id VARCHAR(255),
    environment VARCHAR(20) NOT NULL,
    receipt_payload TEXT NOT NULL,
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    gross_amount NUMERIC(18,2) NOT NULL,
    store_fee_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    net_revenue_amount NUMERIC(18,2) NOT NULL,
    status payment_status NOT NULL DEFAULT 'pending',
    verified_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_iap_transactions_provider_tx UNIQUE (store_provider, store_transaction_id)
);

-- +goose Down
DROP TABLE IF EXISTS iap_transactions;
DROP TABLE IF EXISTS subscription_plan_prices;
DROP TABLE IF EXISTS subscription_plans;
DROP TABLE IF EXISTS user_digital_inventories;
DROP TABLE IF EXISTS product_prices;
DROP TABLE IF EXISTS digital_products;
