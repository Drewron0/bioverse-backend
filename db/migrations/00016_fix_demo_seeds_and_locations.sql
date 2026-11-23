-- +goose NO TRANSACTION
-- +goose Up

-- 1. Fix Tanjung Lesung Zone ID & Location Mapping
-- 00013 used 0193297a-96e0-7988-9271-e0e649ce4102 for Nusa Penida.
-- Insert dedicated zone for Tanjung Lesung with ID ending in 4104.
INSERT INTO conservation_zones (
    id, name, region, province, ecosystem_type, allocation_type, latitude, longitude, min_depth_meters, max_depth_meters
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4104',
    'Tanjung Lesung Coastal Mangrove',
    'Pandeglang',
    'Banten',
    'mangrove',
    'b2c_pool',
    -6.4833330,
    105.6666670,
    0.00,
    3.00
) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    region = EXCLUDED.region,
    province = EXCLUDED.province,
    ecosystem_type = EXCLUDED.ecosystem_type,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude;

-- Re-point Tanjung Lesung Mangrove Shield project to the correct Banten zone
UPDATE conservation_projects
SET zone_id = '0193297a-96e0-7988-9271-e0e649ce4104'
WHERE id = '0193297a-96e0-7988-9271-e0e649ce4202';

-- 2. Clean up test missions inserted by previous unit test runs
DELETE FROM eco_missions
WHERE title = 'Reusable Tumbler Snap';

-- 3. Seed canonical eco-missions with real images and estimated minutes
INSERT INTO eco_missions (
    id, title, description, instructions, category, difficulty, image_url,
    points_reward, xp_reward, requires_camera_proof, cv_target_label, cv_min_confidence,
    is_daily, is_active, estimated_minutes
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4801',
    'Bawa Tumbler Sendiri',
    'Gunakan botol minum isi ulang saat beraktivitas untuk mengurangi sampah plastik sekali pakai.',
    'Ambil foto botol tumbler Anda di samping dispenser air minum atau saat sedang digunakan.',
    'lifestyle',
    'easy',
    'https://images.unsplash.com/photo-1602143407151-7111542de6e8',
    50,
    25,
    true,
    'reusable_water_bottle',
    0.80,
    true,
    true,
    5
) ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    instructions = EXCLUDED.instructions,
    category = EXCLUDED.category,
    difficulty = EXCLUDED.difficulty,
    image_url = EXCLUDED.image_url,
    points_reward = EXCLUDED.points_reward,
    xp_reward = EXCLUDED.xp_reward,
    estimated_minutes = EXCLUDED.estimated_minutes,
    is_active = true;

INSERT INTO eco_missions (
    id, title, description, instructions, category, difficulty, image_url,
    points_reward, xp_reward, requires_camera_proof, cv_target_label, cv_min_confidence,
    is_daily, is_active, estimated_minutes
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4802',
    'Pilah Sampah Plastik',
    'Kumpulkan dan pisahkan botol atau kemasan plastik bekas konsumsi harian Anda ke tempat daur ulang.',
    'Foto wadah atau kantong sampah plastik yang telah dipilah sebelum diserahkan ke tempat daur ulang.',
    'waste_reduction',
    'medium',
    'https://images.unsplash.com/photo-1530587191325-3db32d826c18',
    75,
    40,
    true,
    'plastic_recycling',
    0.80,
    true,
    true,
    10
) ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    instructions = EXCLUDED.instructions,
    category = EXCLUDED.category,
    difficulty = EXCLUDED.difficulty,
    image_url = EXCLUDED.image_url,
    points_reward = EXCLUDED.points_reward,
    xp_reward = EXCLUDED.xp_reward,
    estimated_minutes = EXCLUDED.estimated_minutes,
    is_active = true;

INSERT INTO eco_missions (
    id, title, description, instructions, category, difficulty, image_url,
    points_reward, xp_reward, requires_camera_proof, cv_target_label, cv_min_confidence,
    is_daily, is_active, estimated_minutes
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4803',
    'Hemat Energi Listrik Siang Hari',
    'Matikan lampu ruangan dan maksimalkan pencahayaan sinar matahari alami dari jendela pada siang hari.',
    'Foto ruangan kerja atau kamar Anda yang terang benderang oleh cahaya matahari alami tanpa lampu.',
    'energy_saving',
    'easy',
    'https://images.unsplash.com/photo-1509395176047-4a66953fd231',
    40,
    20,
    true,
    'natural_daylight',
    0.80,
    true,
    true,
    5
) ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    instructions = EXCLUDED.instructions,
    category = EXCLUDED.category,
    difficulty = EXCLUDED.difficulty,
    image_url = EXCLUDED.image_url,
    points_reward = EXCLUDED.points_reward,
    xp_reward = EXCLUDED.xp_reward,
    estimated_minutes = EXCLUDED.estimated_minutes,
    is_active = true;

INSERT INTO eco_missions (
    id, title, description, instructions, category, difficulty, image_url,
    points_reward, xp_reward, requires_camera_proof, cv_target_label, cv_min_confidence,
    is_daily, is_active, estimated_minutes
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4804',
    'Gunakan Tas Belanja Kain',
    'Bawa dan gunakan tote bag kain saat berbelanja untuk menghindari kantong plastik sekali pakai.',
    'Foto tas kain belanjaan Anda yang sudah terisi barang kebutuhan harian.',
    'lifestyle',
    'easy',
    'https://images.unsplash.com/photo-1544816155-12df9643f363',
    60,
    30,
    true,
    'tote_bag',
    0.80,
    true,
    true,
    5
) ON CONFLICT (id) DO UPDATE SET
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    instructions = EXCLUDED.instructions,
    category = EXCLUDED.category,
    difficulty = EXCLUDED.difficulty,
    image_url = EXCLUDED.image_url,
    points_reward = EXCLUDED.points_reward,
    xp_reward = EXCLUDED.xp_reward,
    estimated_minutes = EXCLUDED.estimated_minutes,
    is_active = true;

-- +goose Down
-- Revert is safe
