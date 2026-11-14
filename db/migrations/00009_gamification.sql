-- +goose Up

CREATE TABLE IF NOT EXISTS virtual_pets (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fragment_id UUID UNIQUE REFERENCES coral_fragments(id) ON DELETE SET NULL,
    equipped_product_id UUID REFERENCES digital_products(id) ON DELETE SET NULL,
    pet_name VARCHAR(100) NOT NULL,
    pet_species VARCHAR(50) NOT NULL,
    coral_type VARCHAR(50) NOT NULL,
    growth_stage pet_growth_stage NOT NULL DEFAULT 'hatchling',
    health_percent INT NOT NULL DEFAULT 100 CHECK (health_percent >= 0 AND health_percent <= 100),
    level INT NOT NULL DEFAULT 1,
    experience_points BIGINT NOT NULL DEFAULT 0,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    last_interacted_at TIMESTAMPTZ,
    adopted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_primary_pet_per_user 
ON virtual_pets (user_id) 
WHERE is_primary = true;

CREATE TABLE IF NOT EXISTS pet_boosts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pet_id UUID NOT NULL REFERENCES virtual_pets(id) ON DELETE CASCADE,
    donation_id UUID UNIQUE REFERENCES donations(id) ON DELETE CASCADE,
    product_id UUID REFERENCES digital_products(id) ON DELETE CASCADE,
    boost_type pet_boost_type NOT NULL,
    xp_multiplier NUMERIC(5,2) NOT NULL DEFAULT 1.25,
    is_active BOOLEAN NOT NULL DEFAULT true,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    CONSTRAINT chk_boost_source CHECK (
        (CASE WHEN donation_id IS NOT NULL THEN 1 ELSE 0 END +
         CASE WHEN product_id IS NOT NULL THEN 1 ELSE 0 END) = 1
    )
);

CREATE TABLE IF NOT EXISTS eco_missions (
    id UUID PRIMARY KEY,
    sponsor_company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    title VARCHAR(150) NOT NULL,
    description TEXT NOT NULL,
    instructions TEXT NOT NULL,
    category VARCHAR(50),
    difficulty VARCHAR(20),
    image_url TEXT,
    points_reward BIGINT NOT NULL,
    xp_reward BIGINT NOT NULL,
    requires_camera_proof BOOLEAN NOT NULL DEFAULT true,
    cv_target_label VARCHAR(100),
    cv_min_confidence NUMERIC(5,2) NOT NULL DEFAULT 0.80,
    is_daily BOOLEAN NOT NULL DEFAULT true,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_mission_submissions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mission_id UUID NOT NULL REFERENCES eco_missions(id) ON DELETE CASCADE,
    pet_id UUID REFERENCES virtual_pets(id) ON DELETE SET NULL,
    submission_date DATE NOT NULL,
    proof_image_url TEXT,
    image_hash VARCHAR(64),
    status submission_status NOT NULL DEFAULT 'pending',
    cv_job_id VARCHAR(100),
    cv_model_version VARCHAR(50),
    cv_confidence_score NUMERIC(5,2),
    cv_raw_response JSONB,
    awarded_points BIGINT NOT NULL DEFAULT 0,
    awarded_xp BIGINT NOT NULL DEFAULT 0,
    rejection_reason TEXT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_daily_mission_non_rejected 
ON user_mission_submissions (user_id, mission_id, submission_date) 
WHERE status != 'rejected';

CREATE TABLE IF NOT EXISTS point_ledgers (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    transaction_type point_tx_type NOT NULL,
    amount BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    reference_type VARCHAR(50) NOT NULL,
    reference_id UUID NOT NULL,
    description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_point_ledger_reference UNIQUE (reference_type, reference_id, transaction_type)
);

-- +goose Down
DROP TABLE IF EXISTS point_ledgers;
DROP TABLE IF EXISTS user_mission_submissions;
DROP TABLE IF EXISTS eco_missions;
DROP TABLE IF EXISTS pet_boosts;
DROP TABLE IF EXISTS virtual_pets;
