-- +goose Up

-- 1. App settings
INSERT INTO app_settings (setting_key, setting_value, description)
VALUES 
    ('points_per_usd', '100'::jsonb, 'Points granted per 1 USD donated or value of 100 points = 1 USD discount'),
    ('coral_fragment_price_usd', '3.20'::jsonb, 'Baseline price per coral fragment adoption'),
    ('bio_shield_addon_price_usd', '1.80'::jsonb, 'Price for Bio-Shield add-on offering 1.25x XP multiplier'),
    ('max_fragments_per_spider_frame', '15'::jsonb, 'Maximum capacity of fragments on a single spider frame')
ON CONFLICT (setting_key) DO NOTHING;

-- 2. Conservation zones
INSERT INTO conservation_zones (id, name, region, province, ecosystem_type, allocation_type, latitude, longitude, min_depth_meters, max_depth_meters)
VALUES 
    ('0193297a-96e0-7988-9271-e0e649ce4101', 'Pemuteran Bay Coral Sanctuary', 'Buleleng', 'Bali', 'marine_coral', 'b2c_pool', -8.1436000, 114.6534000, 3.00, 10.00),
    ('0193297a-96e0-7988-9271-e0e649ce4102', 'Nusa Penida Marine Protected Area', 'Klungkung', 'Bali', 'marine_coral', 'b2b_dedicated', -8.7278000, 115.5444000, 5.00, 15.00)
ON CONFLICT (id) DO NOTHING;

-- 3. Conservation projects
INSERT INTO conservation_projects (
    id, zone_id, title, description, objectives, category, species_or_type_name, 
    image_url, status, physical_health_status, unit_name, currency_code, 
    price_per_unit_money, price_per_unit_points, target_amount, target_units, 
    funded_units_count, planted_units_count, surviving_units_count, replant_reserve_pct, 
    donation_start_date, donation_end_date, estimated_planting_date
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4201',
    '0193297a-96e0-7988-9271-e0e649ce4101',
    'Biorock Coral Restoration Pemuteran',
    'Restorasi terumbu karang berbasis teknologi struktur Biorock di Teluk Pemuteran Bali untuk mempercepat pertumbuhan karang hingga 3x lipat.',
    'Menanam 1.500 fragmen karang Acropora dan Pocillopora demi memulihkan habitat penyu dan keanekaragaman hayati laut Bali Utara.',
    'coral',
    'Acropora & Pocillopora',
    'https://images.unsplash.com/photo-1546026423-cc4642628d2b',
    'open_for_donation',
    'healthy',
    'fragment',
    'USD',
    3.20,
    320,
    4800.00,
    1500,
    420,
    300,
    285,
    30.00,
    '2026-01-01',
    '2026-12-31',
    '2026-06-01'
) ON CONFLICT (id) DO NOTHING;

-- 4. Spider frames (capacity 15 each)
INSERT INTO spider_frames (
    id, frame_code, zone_id, depth_meters, latitude, longitude, max_capacity_fragments, fragments_planted, physical_health_status
) VALUES 
    ('0193297a-96e0-7988-9271-e0e649ce4301', 'SF-BALI-001', '0193297a-96e0-7988-9271-e0e649ce4101', 5.50, -8.1436100, 114.6534200, 15, 10, 'healthy'),
    ('0193297a-96e0-7988-9271-e0e649ce4302', 'SF-BALI-002', '0193297a-96e0-7988-9271-e0e649ce4101', 6.00, -8.1436300, 114.6534500, 15, 0, 'healthy')
ON CONFLICT (id) DO NOTHING;

-- 5. Digital products & prices
INSERT INTO digital_products (id, sku_code, name, description, item_type, price_points, is_iap_only, attributes, preview_asset_url, is_active)
VALUES 
    ('0193297a-96e0-7988-9271-e0e649ce4401', 'STREAK_FREEZE_1X', 'Streak Freeze', 'Melindungi streak harian Anda agar tidak reset jika terlewat satu hari.', 'streak_freeze', 150, false, '{"freeze_days": 1}'::jsonb, 'https://assets.bioverse.id/items/streak_freeze.png', true),
    ('0193297a-96e0-7988-9271-e0e649ce4402', 'BIO_SHIELD_XP_BOOST', 'Bio-Shield XP Elixir', 'Memberikan boost XP x1.25 untuk pet Anda selama 7 hari.', 'xp_boost_item', 300, false, '{"xp_multiplier": 1.25, "duration_hours": 168}'::jsonb, 'https://assets.bioverse.id/items/bio_shield_boost.png', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO product_prices (id, product_id, currency_code, amount)
VALUES 
    ('0193297a-96e0-7988-9271-e0e649ce4501', '0193297a-96e0-7988-9271-e0e649ce4401', 'USD', 0.99),
    ('0193297a-96e0-7988-9271-e0e649ce4401', '0193297a-96e0-7988-9271-e0e649ce4401', 'IDR', 15000.00),
    ('0193297a-96e0-7988-9271-e0e649ce4502', '0193297a-96e0-7988-9271-e0e649ce4402', 'USD', 1.80),
    ('0193297a-96e0-7988-9271-e0e649ce4402', '0193297a-96e0-7988-9271-e0e649ce4402', 'IDR', 29000.00)
ON CONFLICT (id) DO NOTHING;

-- 6. Subscription plans
INSERT INTO subscription_plans (id, sku_code, group_code, name, description, plan_type, billing_interval, interval_count, benefits, is_active, sort_order)
VALUES 
    ('0193297a-96e0-7988-9271-e0e649ce4601', 'BIOVERSE_PLUS_MONTHLY', 'bioverse_plus', 'BioVerse Plus Monthly', 'Dukungan pembersihan rutin bulanan untuk karang adopsi Anda beserta sertifikat diver.', 'maintenance', 'monthly', 1, '{"monthly_report": true, "exclusive_skin": true}'::jsonb, true, 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO subscription_plan_prices (id, plan_id, currency_code, amount)
VALUES 
    ('0193297a-96e0-7988-9271-e0e649ce4701', '0193297a-96e0-7988-9271-e0e649ce4601', 'USD', 1.99),
    ('0193297a-96e0-7988-9271-e0e649ce4702', '0193297a-96e0-7988-9271-e0e649ce4601', 'IDR', 32000.00)
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM subscription_plan_prices WHERE id IN ('0193297a-96e0-7988-9271-e0e649ce4701', '0193297a-96e0-7988-9271-e0e649ce4702');
DELETE FROM subscription_plans WHERE id = '0193297a-96e0-7988-9271-e0e649ce4601';
DELETE FROM product_prices WHERE id IN ('0193297a-96e0-7988-9271-e0e649ce4501', '0193297a-96e0-7988-9271-e0e649ce4502');
DELETE FROM digital_products WHERE id IN ('0193297a-96e0-7988-9271-e0e649ce4401', '0193297a-96e0-7988-9271-e0e649ce4402');
DELETE FROM spider_frames WHERE id IN ('0193297a-96e0-7988-9271-e0e649ce4301', '0193297a-96e0-7988-9271-e0e649ce4302');
DELETE FROM conservation_projects WHERE id = '0193297a-96e0-7988-9271-e0e649ce4201';
DELETE FROM conservation_zones WHERE id IN ('0193297a-96e0-7988-9271-e0e649ce4101', '0193297a-96e0-7988-9271-e0e649ce4102');
DELETE FROM app_settings WHERE setting_key IN ('points_per_usd', 'coral_fragment_price_usd', 'bio_shield_addon_price_usd', 'max_fragments_per_spider_frame');
