-- BioVerse backend: migration 00001 — all enum types (36).
-- This file is BOTH the enum reference document and the first migration.
-- Rules:
--   * Enum = stable state machines / closed sets. Add a value later with
--     `ALTER TYPE x ADD VALUE 'y';` in its own migration marked `-- +goose NO TRANSACTION`.
--   * Values are never renamed or removed in production; deprecate in code instead.
--   * NOTE: `money_payment_method`, `incident_status`, `physical_health_status`, `verification_status`
--     etc. in the ERD are COLUMN names, not types. The type is the first token in each ERD line.

-- +goose Up

-- ─────────────────────────── Identity & access ───────────────────────────

-- Used by: platform_admins.role
-- super_admin: everything incl. admin management | operations: zones, projects, frames, operators, field data
-- finance: payouts, invoices, refunds | content: news, missions, catalog | support: read-mostly, user help
CREATE TYPE admin_role AS ENUM ('super_admin', 'operations', 'finance', 'content', 'support');

-- Used by: users.gender
CREATE TYPE user_gender AS ENUM ('male', 'female', 'other', 'prefer_not_to_say');

-- Used by: user_tokens.token_type, operator_tokens.token_type, admin_tokens.token_type
-- refresh: rotating refresh token | email_verification | password_reset | fcm_device: push token registration
CREATE TYPE token_type_enum AS ENUM ('refresh', 'email_verification', 'password_reset', 'fcm_device');

-- ─────────────────────────── Points & gamification ───────────────────────────

-- Used by: point_ledgers.transaction_type. Unique with (reference_type, reference_id).
-- lock_donation / unlock_donation change locked_points_balance only (points_balance unchanged);
-- spend_donation deducts points_balance (and releases the lock) when the donation is paid.
CREATE TYPE point_tx_type AS ENUM (
    'earn_mission', 'earn_pet_care', 'earn_referral', 'earn_bonus',
    'lock_donation', 'unlock_donation', 'spend_donation', 'spend_shop',
    'refund', 'admin_adjustment'
);

-- Used by: virtual_pets.growth_stage (thresholds live in app_settings)
CREATE TYPE pet_growth_stage AS ENUM ('hatchling', 'juvenile', 'adult', 'elder');

-- Used by: pet_boosts.boost_type
-- bio_shield_xp: x1.25 XP from the Bio-Shield add-on (source: donation) | event_xp: seasonal | sponsor_xp: brand campaign
-- item_xp: boost from an xp_boost_item inventory product (source: product)
CREATE TYPE pet_boost_type AS ENUM ('bio_shield_xp', 'event_xp', 'sponsor_xp', 'item_xp');

-- Used by: user_mission_submissions.status AND operator_work_logs.verification_status
-- pending -> processing (CV job running) -> verified | rejected | needs_review (low confidence, human check)
-- Operator work logs only use pending -> verified | rejected.
CREATE TYPE submission_status AS ENUM ('pending', 'processing', 'verified', 'rejected', 'needs_review');

-- ─────────────────────────── Catalog, subscriptions, payments ───────────────────────────

-- Used by: subscription_plans.plan_type
CREATE TYPE subscription_plan_type AS ENUM ('maintenance', 'premium_benefit', 'custom');

-- Used by: subscription_plans.billing_interval, donation_subscriptions.frequency
-- Unit only; multiples via interval_count (e.g. every 6 months = monthly x 6).
CREATE TYPE subscription_frequency AS ENUM ('weekly', 'monthly', 'yearly');

-- Used by: donation_subscriptions.status
-- pending_activation (first charge not paid) -> active <-> paused (failed charges) -> cancelled | completed
CREATE TYPE subscription_status AS ENUM ('pending_activation', 'active', 'paused', 'cancelled', 'completed');

-- Used by: donations.donation_type
CREATE TYPE donation_type AS ENUM ('coral_adoption', 'tree_planting', 'bio_shield_addon', 'maintenance', 'general_support');

-- Used by: donations.money_payment_method, donation_subscriptions.payment_method, b2b_sponsorship_invoices.payment_method
-- points_only: fully paid by points | sponsored_quota: paid by a brand quota | manual_bank_transfer: B2B invoices
CREATE TYPE payment_method_type AS ENUM (
    'qris', 'virtual_account', 'ewallet', 'card',
    'manual_bank_transfer', 'points_only', 'sponsored_quota'
);

-- Used by: donations.payment_status, b2b_sponsorship_invoices.status, iap_transactions.status
-- IAP mapping: paid = verified by the store, refunded = revoked.
-- pending -> paid | failed | expired | cancelled ; paid -> refunded
CREATE TYPE payment_status AS ENUM ('pending', 'paid', 'failed', 'expired', 'cancelled', 'refunded');

-- Used by: digital_products.item_type (new kinds = new value, config goes in digital_products.attributes)
CREATE TYPE digital_item_type AS ENUM (
    'streak_freeze', 'pet_skin', 'pet_accessory', 'habitat_background', 'xp_boost_item'
);

-- Used by: iap_transactions.store_provider
CREATE TYPE iap_store_type AS ENUM ('apple_app_store', 'google_play');

-- ─────────────────────────── Conservation domain ───────────────────────────

-- Used by: coral_fragments.lifecycle_status
-- pending_planting -> planted -> monitoring -> deceased | relocated
CREATE TYPE fragment_lifecycle_status AS ENUM ('pending_planting', 'planted', 'monitoring', 'deceased', 'relocated');

-- Used by: coral_fragments.health_status, conservation_projects.physical_health_status,
--          spider_frames.physical_health_status, project_incidents.incident_status
CREATE TYPE environmental_health_status AS ENUM (
    'healthy', 'stressed', 'bleaching', 'diseased', 'recovering', 'critical', 'dead'
);

-- Used by: conservation_zones.ecosystem_type
CREATE TYPE ecosystem_kind AS ENUM ('marine_coral', 'mangrove', 'terrestrial');

-- Used by: conservation_zones.allocation_type
-- b2c_pool: open to app users | b2b_dedicated: reserved for sponsors | mixed
CREATE TYPE zone_allocation_type AS ENUM ('b2c_pool', 'b2b_dedicated', 'mixed');

-- Used by: conservation_projects.category, news_articles.category. Trees are aggregate-only in the MVP.
CREATE TYPE project_category AS ENUM ('coral', 'mangrove', 'tree');

-- Used by: conservation_projects.status
-- draft -> open_for_donation -> fully_funded -> planting -> monitoring -> completed ; cancelled from any pre-completed state
CREATE TYPE project_status AS ENUM (
    'draft', 'open_for_donation', 'fully_funded', 'planting', 'monitoring', 'completed', 'cancelled'
);

-- ─────────────────────────── Field operations ───────────────────────────

-- Used by: community_groups.group_type
CREATE TYPE community_group_type AS ENUM (
    'fishermen', 'forest_farmers', 'indigenous_community', 'youth_group', 'other'
);

-- Used by: field_operators.operator_role
CREATE TYPE operator_role AS ENUM ('diver', 'planter', 'site_manager');

-- Used by: field_operators.kyc_status
-- pending -> submitted -> verified | rejected
CREATE TYPE kyc_verification_status AS ENUM ('pending', 'submitted', 'verified', 'rejected');

-- Used by: operator_work_logs.task_type
CREATE TYPE operator_task_type AS ENUM (
    'frame_installation', 'planting', 'replanting', 'maintenance_cleaning', 'monitoring_photo', 'survey'
);

-- Used by: operator_payouts.status
-- draft -> pending_approval -> approved -> processing -> disbursed | failed ; cancelled before processing
CREATE TYPE payout_status AS ENUM (
    'draft', 'pending_approval', 'approved', 'processing', 'disbursed', 'failed', 'cancelled'
);

-- Used by: field_service_requests.request_type (B2B asks the field team)
CREATE TYPE request_type AS ENUM ('diver_cleaning', 'inspection', 'photo_documentation', 'plaque_maintenance');

-- Used by: field_service_requests.status
-- requested -> scheduled -> in_progress -> completed ; cancelled
CREATE TYPE request_status AS ENUM ('requested', 'scheduled', 'in_progress', 'completed', 'cancelled');

-- Used by: media_evidences.media_type
CREATE TYPE evidence_media_type AS ENUM ('photo', 'video', 'orthomosaic', 'model_3d');

-- ─────────────────────────── B2B & ESG ───────────────────────────

-- Used by: company_members.role
CREATE TYPE b2b_role AS ENUM ('corporate_admin', 'esg_manager', 'finance', 'external_auditor', 'viewer');

-- Used by: company_members.permission
-- view_only < edit < sign_off (approve reports) < admin (manage members)
CREATE TYPE b2b_permission AS ENUM ('view_only', 'edit', 'sign_off', 'admin');

-- Used by: b2b_packages.package_type
CREATE TYPE b2b_package_type AS ENUM ('starter_bundle', 'enterprise_suite', 'brand_co_creation', 'custom');

-- Used by: b2b_contracts.status
-- draft -> pending_payment -> active -> expired | terminated (renewal = new contract via renewed_from_contract_id)
CREATE TYPE contract_status AS ENUM ('draft', 'pending_payment', 'active', 'expired', 'terminated');

-- Used by: esg_biodiversity_metrics.status
CREATE TYPE compliance_status AS ENUM ('compliant', 'at_risk', 'non_compliant', 'pending_data');

-- Used by: esg_audit_reports.auditor_status
-- draft -> submitted -> under_review -> certified | rejected
CREATE TYPE report_audit_status AS ENUM ('draft', 'submitted', 'under_review', 'certified', 'rejected');

-- ─────────────────────────── System ───────────────────────────

-- Used by: webhook_events.status
-- pending -> processing -> processed | failed (retry with backoff) ; ignored (unknown event type)
CREATE TYPE webhook_process_status AS ENUM ('pending', 'processing', 'processed', 'failed', 'ignored');

-- +goose Down
DROP TYPE IF EXISTS webhook_process_status;
DROP TYPE IF EXISTS report_audit_status;
DROP TYPE IF EXISTS compliance_status;
DROP TYPE IF EXISTS contract_status;
DROP TYPE IF EXISTS b2b_package_type;
DROP TYPE IF EXISTS b2b_permission;
DROP TYPE IF EXISTS b2b_role;
DROP TYPE IF EXISTS evidence_media_type;
DROP TYPE IF EXISTS request_status;
DROP TYPE IF EXISTS request_type;
DROP TYPE IF EXISTS payout_status;
DROP TYPE IF EXISTS operator_task_type;
DROP TYPE IF EXISTS kyc_verification_status;
DROP TYPE IF EXISTS operator_role;
DROP TYPE IF EXISTS community_group_type;
DROP TYPE IF EXISTS project_status;
DROP TYPE IF EXISTS project_category;
DROP TYPE IF EXISTS zone_allocation_type;
DROP TYPE IF EXISTS ecosystem_kind;
DROP TYPE IF EXISTS environmental_health_status;
DROP TYPE IF EXISTS fragment_lifecycle_status;
DROP TYPE IF EXISTS iap_store_type;
DROP TYPE IF EXISTS digital_item_type;
DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS payment_method_type;
DROP TYPE IF EXISTS donation_type;
DROP TYPE IF EXISTS subscription_status;
DROP TYPE IF EXISTS subscription_frequency;
DROP TYPE IF EXISTS subscription_plan_type;
DROP TYPE IF EXISTS submission_status;
DROP TYPE IF EXISTS pet_boost_type;
DROP TYPE IF EXISTS pet_growth_stage;
DROP TYPE IF EXISTS point_tx_type;
DROP TYPE IF EXISTS token_type_enum;
DROP TYPE IF EXISTS user_gender;
DROP TYPE IF EXISTS admin_role;
