-- +goose Up

CREATE TABLE IF NOT EXISTS corporate_pr_assets (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    spider_frame_id UUID REFERENCES spider_frames(id) ON DELETE SET NULL,
    asset_category VARCHAR(50) NOT NULL,
    title VARCHAR(150) NOT NULL,
    file_format VARCHAR(20) NOT NULL,
    file_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS esg_biodiversity_metrics (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    fiscal_year INT NOT NULL,
    fiscal_quarter INT NOT NULL,
    indicator_name VARCHAR(100) NOT NULL,
    unit VARCHAR(30) NOT NULL,
    baseline_value NUMERIC(18,2) NOT NULL,
    current_value NUMERIC(18,2) NOT NULL,
    target_value NUMERIC(18,2) NOT NULL,
    variance_percentage NUMERIC(6,2) NOT NULL,
    status compliance_status NOT NULL DEFAULT 'compliant',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS esg_audit_reports (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    generated_by_member_id UUID REFERENCES company_members(id) ON DELETE SET NULL,
    title VARCHAR(150) NOT NULL,
    fiscal_year INT NOT NULL,
    fiscal_quarter INT NOT NULL,
    reporting_period_label VARCHAR(50) NOT NULL,
    pojk51_readiness_pct NUMERIC(5,2) NOT NULL DEFAULT 100.00,
    gri_criteria_met INT NOT NULL DEFAULT 12,
    assurance_grade VARCHAR(20) NOT NULL DEFAULT 'Grade_A',
    chapter_pdf_url TEXT,
    raw_dataset_csv_url TEXT,
    visual_annex_zip_url TEXT,
    auditor_status report_audit_status NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS esg_audit_reports;
DROP TABLE IF EXISTS esg_biodiversity_metrics;
DROP TABLE IF EXISTS corporate_pr_assets;
