-- +goose Up

CREATE TABLE IF NOT EXISTS conservation_zones (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    region VARCHAR(100) NOT NULL,
    province VARCHAR(100) NOT NULL,
    ecosystem_type ecosystem_kind NOT NULL,
    allocation_type zone_allocation_type NOT NULL DEFAULT 'b2c_pool',
    latitude NUMERIC(10,7) NOT NULL,
    longitude NUMERIC(10,7) NOT NULL,
    min_depth_meters NUMERIC(6,2),
    max_depth_meters NUMERIC(6,2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS conservation_projects (
    id UUID PRIMARY KEY,
    zone_id UUID NOT NULL REFERENCES conservation_zones(id) ON DELETE CASCADE,
    title VARCHAR(150) NOT NULL,
    description TEXT NOT NULL,
    objectives TEXT NOT NULL,
    category project_category NOT NULL,
    species_or_type_name VARCHAR(100) NOT NULL,
    image_url TEXT NOT NULL,
    status project_status NOT NULL DEFAULT 'open_for_donation',
    physical_health_status environmental_health_status NOT NULL DEFAULT 'healthy',
    is_underfunded_adjusted BOOLEAN NOT NULL DEFAULT false,
    original_target_amount NUMERIC(18,2),
    original_target_units INT,
    adjusted_at TIMESTAMPTZ,
    unit_name VARCHAR(50) NOT NULL DEFAULT 'fragment',
    currency_code VARCHAR(3) NOT NULL REFERENCES currencies(code) DEFAULT 'USD',
    price_per_unit_money NUMERIC(18,2) NOT NULL,
    price_per_unit_points BIGINT NOT NULL DEFAULT 0,
    target_amount NUMERIC(18,2) NOT NULL,
    target_units INT NOT NULL,
    funded_units_count INT NOT NULL DEFAULT 0,
    planted_units_count INT NOT NULL DEFAULT 0,
    surviving_units_count INT NOT NULL DEFAULT 0,
    replant_reserve_pct NUMERIC(5,2) NOT NULL DEFAULT 30.00,
    donation_start_date DATE NOT NULL,
    donation_end_date DATE NOT NULL,
    estimated_planting_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS spider_frames (
    id UUID PRIMARY KEY,
    frame_code VARCHAR(50) NOT NULL UNIQUE,
    company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    sponsorship_invoice_id UUID REFERENCES b2b_sponsorship_invoices(id) ON DELETE SET NULL,
    zone_id UUID NOT NULL REFERENCES conservation_zones(id) ON DELETE CASCADE,
    depth_meters NUMERIC(6,2) NOT NULL,
    latitude NUMERIC(10,7) NOT NULL,
    longitude NUMERIC(10,7) NOT NULL,
    max_capacity_fragments INT NOT NULL DEFAULT 15,
    fragments_planted INT NOT NULL DEFAULT 0,
    physical_health_status environmental_health_status NOT NULL DEFAULT 'healthy',
    plaque_inscription_text VARCHAR(255),
    plaque_material VARCHAR(50),
    plaque_dimensions VARCHAR(50),
    plaque_3d_preview_url TEXT,
    biofouling_status VARCHAR(50),
    last_cleaned_at DATE,
    next_maintenance_at DATE,
    deployed_at DATE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_frame_capacity CHECK (fragments_planted <= max_capacity_fragments)
);

-- +goose Down
DROP TABLE IF EXISTS spider_frames;
DROP TABLE IF EXISTS conservation_projects;
DROP TABLE IF EXISTS conservation_zones;
