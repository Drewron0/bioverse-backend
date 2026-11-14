-- +goose Up

CREATE TABLE IF NOT EXISTS field_service_requests (
    id UUID PRIMARY KEY,
    company_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    spider_frame_id UUID NOT NULL REFERENCES spider_frames(id) ON DELETE CASCADE,
    requested_by_member_id UUID NOT NULL REFERENCES company_members(id),
    assigned_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    request_type request_type NOT NULL,
    status request_status NOT NULL DEFAULT 'requested',
    agreed_fee_idr NUMERIC(18,2) NOT NULL DEFAULT 0,
    scheduled_date DATE,
    completed_at TIMESTAMPTZ,
    result_media_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS operator_payouts (
    id UUID PRIMARY KEY,
    idempotency_key VARCHAR(64) UNIQUE NOT NULL,
    operator_id UUID NOT NULL REFERENCES field_operators(id) ON DELETE CASCADE,
    approved_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    total_units_planted INT NOT NULL DEFAULT 0,
    total_maintenance_tasks INT NOT NULL DEFAULT 0,
    gross_amount_idr NUMERIC(18,2) NOT NULL,
    transfer_fee_idr NUMERIC(18,2) NOT NULL DEFAULT 0,
    net_disbursed_idr NUMERIC(18,2) NOT NULL,
    payout_gateway_provider VARCHAR(50) NOT NULL,
    external_disbursement_id VARCHAR(255) UNIQUE,
    status payout_status NOT NULL DEFAULT 'draft',
    failure_reason TEXT,
    approved_at TIMESTAMPTZ,
    disbursed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS operator_work_logs (
    id UUID PRIMARY KEY,
    operator_id UUID NOT NULL REFERENCES field_operators(id) ON DELETE CASCADE,
    entered_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    approved_by_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    project_id UUID REFERENCES conservation_projects(id) ON DELETE CASCADE,
    spider_frame_id UUID REFERENCES spider_frames(id) ON DELETE SET NULL,
    coral_fragment_id UUID REFERENCES coral_fragments(id) ON DELETE SET NULL,
    service_request_id UUID UNIQUE REFERENCES field_service_requests(id) ON DELETE SET NULL,
    payout_id UUID REFERENCES operator_payouts(id) ON DELETE SET NULL,
    task_type operator_task_type NOT NULL,
    units_completed INT NOT NULL DEFAULT 1,
    fee_earned_idr NUMERIC(18,2) NOT NULL DEFAULT 0,
    proof_image_url TEXT NOT NULL,
    verification_status submission_status NOT NULL DEFAULT 'pending',
    work_date TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subsea_telemetry_logs (
    id UUID PRIMARY KEY,
    zone_id UUID NOT NULL REFERENCES conservation_zones(id) ON DELETE CASCADE,
    spider_frame_id UUID REFERENCES spider_frames(id) ON DELETE SET NULL,
    recorded_by_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    entered_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    water_temperature_celsius NUMERIC(5,2) NOT NULL,
    temperature_status VARCHAR(50),
    salinity_psu NUMERIC(5,2) NOT NULL,
    visibility_meters NUMERIC(5,2) NOT NULL,
    camera_station_code VARCHAR(50),
    recorded_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS media_evidences (
    id UUID PRIMARY KEY,
    project_id UUID REFERENCES conservation_projects(id) ON DELETE SET NULL,
    spider_frame_id UUID REFERENCES spider_frames(id) ON DELETE SET NULL,
    coral_fragment_id UUID REFERENCES coral_fragments(id) ON DELETE SET NULL,
    company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    captured_by_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    verified_by_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    uploaded_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    media_type evidence_media_type NOT NULL DEFAULT 'photo',
    milestone_month INT CHECK (milestone_month IN (1, 3, 6)),
    milestone_title VARCHAR(100),
    subtitle_metrics VARCHAR(150),
    media_url TEXT NOT NULL,
    raw_archive_url TEXT,
    gps_latitude NUMERIC(10,7),
    gps_longitude NUMERIC(10,7),
    depth_meters NUMERIC(6,2),
    is_verified_healthy BOOLEAN NOT NULL DEFAULT true,
    cv_model_version VARCHAR(50),
    cv_confidence_score NUMERIC(5,2),
    cv_raw_response JSONB,
    exif_metadata JSONB,
    captured_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS survey_records (
    id UUID PRIMARY KEY,
    project_id UUID REFERENCES conservation_projects(id) ON DELETE CASCADE,
    spider_frame_id UUID REFERENCES spider_frames(id) ON DELETE SET NULL,
    surveyed_by_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    entered_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    survey_type VARCHAR(50) NOT NULL,
    indicator_name VARCHAR(100) NOT NULL,
    measured_value NUMERIC(18,2) NOT NULL,
    unit VARCHAR(30) NOT NULL,
    raw_data_url TEXT,
    survey_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS project_incidents (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES conservation_projects(id) ON DELETE CASCADE,
    spider_frame_id UUID REFERENCES spider_frames(id) ON DELETE SET NULL,
    reported_by_admin_id UUID REFERENCES platform_admins(id) ON DELETE SET NULL,
    reported_by_operator_id UUID REFERENCES field_operators(id) ON DELETE SET NULL,
    incident_status environmental_health_status NOT NULL,
    title VARCHAR(150) NOT NULL,
    description TEXT NOT NULL,
    affected_units_count INT NOT NULL DEFAULT 0,
    occurred_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS project_incidents;
DROP TABLE IF EXISTS survey_records;
DROP TABLE IF EXISTS media_evidences;
DROP TABLE IF EXISTS subsea_telemetry_logs;
DROP TABLE IF EXISTS operator_work_logs;
DROP TABLE IF EXISTS operator_payouts;
DROP TABLE IF EXISTS field_service_requests;
