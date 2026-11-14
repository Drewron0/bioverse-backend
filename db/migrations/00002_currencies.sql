-- +goose Up
CREATE TABLE IF NOT EXISTS currencies (
    code VARCHAR(3) PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    symbol VARCHAR(10) NOT NULL,
    minor_unit_digits INT NOT NULL DEFAULT 2,
    is_display_enabled BOOLEAN NOT NULL DEFAULT true,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS exchange_rates (
    id UUID PRIMARY KEY,
    base_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    quote_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    rate NUMERIC(18,6) NOT NULL,
    source VARCHAR(50) NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_exchange_rates_lookup 
ON exchange_rates (base_code, quote_code, effective_at DESC);

-- Seed base currencies USD and IDR
INSERT INTO currencies (code, name, symbol, minor_unit_digits, is_display_enabled, is_active)
VALUES 
    ('USD', 'US Dollar', '$', 2, true, true),
    ('IDR', 'Indonesian Rupiah', 'Rp', 0, true, true)
ON CONFLICT (code) DO NOTHING;

-- Initial exchange rate seed (1 USD = 16,000 IDR)
INSERT INTO exchange_rates (id, base_code, quote_code, rate, source, effective_at)
VALUES 
    ('0193297a-96e0-7988-9271-e0e649ce4301', 'USD', 'IDR', 16000.000000, 'manual_seed', NOW())
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS exchange_rates;
DROP TABLE IF EXISTS currencies;
