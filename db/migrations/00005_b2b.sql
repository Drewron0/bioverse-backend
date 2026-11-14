-- +goose Up

CREATE TABLE IF NOT EXISTS companies (
    id UUID PRIMARY KEY,
    legal_name VARCHAR(255) NOT NULL,
    idx_ojk_ticker VARCHAR(20),
    vector_logo_url TEXT,
    pic_name VARCHAR(100) NOT NULL,
    work_email VARCHAR(255) NOT NULL,
    contact_phone VARCHAR(30) NOT NULL,
    mandatory_framework VARCHAR(50) NOT NULL DEFAULT 'POJK_51_2017',
    biodiversity_standard VARCHAR(50) NOT NULL DEFAULT 'GRI_101',
    reporting_frequency VARCHAR(20) NOT NULL DEFAULT 'quarterly',
    export_format VARCHAR(20) NOT NULL DEFAULT 'pdf_csv',
    external_auditor_direct_access BOOLEAN NOT NULL DEFAULT false,
    annual_fragment_target INT NOT NULL DEFAULT 0,
    baseline_survival_benchmark NUMERIC(5,2) NOT NULL DEFAULT 66.67,
    target_sroi_ratio NUMERIC(5,2) NOT NULL DEFAULT 5.00,
    default_plaque_inscription_text VARCHAR(255),
    public_transparency_feed_enabled BOOLEAN NOT NULL DEFAULT true,
    send_monthly_cleaning_cert BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS company_members (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    invited_email VARCHAR(255) NOT NULL,
    full_name VARCHAR(100) NOT NULL,
    role b2b_role NOT NULL,
    permission b2b_permission NOT NULL,
    invite_token_hash VARCHAR(64) UNIQUE,
    invite_expires_at TIMESTAMPTZ,
    is_accepted BOOLEAN NOT NULL DEFAULT false,
    invited_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    joined_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS b2b_packages (
    id UUID PRIMARY KEY,
    sku_code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    package_type b2b_package_type NOT NULL,
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    price_amount NUMERIC(18,2) NOT NULL,
    spider_frames_count INT NOT NULL,
    coral_fragments_count INT NOT NULL,
    tree_seedlings_count INT NOT NULL DEFAULT 0,
    includes_plaque BOOLEAN NOT NULL DEFAULT true,
    includes_dashboard BOOLEAN NOT NULL DEFAULT true,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS b2b_contracts (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    package_id UUID NOT NULL REFERENCES b2b_packages(id),
    renewed_from_contract_id UUID REFERENCES b2b_contracts(id),
    created_by_member_id UUID NOT NULL REFERENCES company_members(id),
    status contract_status NOT NULL DEFAULT 'draft',
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    contract_value NUMERIC(18,2) NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    auto_renew BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS b2b_sponsorship_invoices (
    id UUID PRIMARY KEY,
    invoice_number VARCHAR(50) NOT NULL UNIQUE,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    contract_id UUID NOT NULL REFERENCES b2b_contracts(id),
    created_by_member_id UUID NOT NULL REFERENCES company_members(id),
    sponsored_frames_count INT NOT NULL,
    sponsored_fragments_count INT NOT NULL,
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    total_amount NUMERIC(18,2) NOT NULL,
    settlement_currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code),
    settlement_amount NUMERIC(18,2) NOT NULL,
    fx_rate_snapshot NUMERIC(18,6) NOT NULL,
    payment_method payment_method_type NOT NULL,
    payment_provider VARCHAR(50) NOT NULL,
    status payment_status NOT NULL DEFAULT 'pending',
    external_gateway_ref VARCHAR(255) UNIQUE,
    invoice_pdf_url TEXT,
    payment_proof_url TEXT,
    due_date DATE NOT NULL,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS b2b_sponsorship_invoices;
DROP TABLE IF EXISTS b2b_contracts;
DROP TABLE IF EXISTS b2b_packages;
DROP TABLE IF EXISTS company_members;
DROP TABLE IF EXISTS companies;
