-- +goose NO TRANSACTION
-- +goose Up

-- 1. Add mangrove_planting to donation_type if not already present
ALTER TYPE donation_type ADD VALUE IF NOT EXISTS 'mangrove_planting';

-- 2. Add estimated_minutes to eco_missions
ALTER TABLE eco_missions ADD COLUMN IF NOT EXISTS estimated_minutes INT NOT NULL DEFAULT 5;

-- 3. Seed additional conservation zones (Mangrove & Terrestrial Forest)
INSERT INTO conservation_zones (
    id, name, region, province, ecosystem_type, allocation_type, latitude, longitude, min_depth_meters, max_depth_meters
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4102',
    'Tanjung Lesung Coastal Mangrove',
    'Pandeglang',
    'Banten',
    'mangrove',
    'b2c_pool',
    -6.4833330,
    105.6666670,
    0.00,
    3.00
) ON CONFLICT (id) DO NOTHING;

INSERT INTO conservation_zones (
    id, name, region, province, ecosystem_type, allocation_type, latitude, longitude, min_depth_meters, max_depth_meters
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4103',
    'Rinjani Agroforestry Slope',
    'Lombok Timur',
    'Nusa Tenggara Barat',
    'terrestrial',
    'b2c_pool',
    -8.4166670,
    116.4500000,
    0.00,
    0.00
) ON CONFLICT (id) DO NOTHING;

-- 4. Seed Mangrove and Tree Projects
INSERT INTO conservation_projects (
    id, zone_id, title, description, objectives, category, species_or_type_name, 
    image_url, status, physical_health_status, unit_name, currency_code, 
    price_per_unit_money, price_per_unit_points, target_amount, target_units, 
    funded_units_count, planted_units_count, surviving_units_count, replant_reserve_pct, 
    donation_start_date, donation_end_date, estimated_planting_date
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4202',
    '0193297a-96e0-7988-9271-e0e649ce4102',
    'Tanjung Lesung Mangrove Shield',
    'Restorasi hutan bakau Rhizophora di pesisir Banten untuk mencegah abrasi pantai, menyerap karbon biru, dan melindungi perkampungan nelayan dari gelombang pasang.',
    'Menanam 2.000 bibit mangrove Rhizophora mucronata dan Avicennia marina di pesisir Selat Sunda.',
    'mangrove',
    'Rhizophora mucronata',
    'https://images.unsplash.com/photo-1544551763-46a013bb70d5',
    'open_for_donation',
    'healthy',
    'tree',
    'USD',
    2.50,
    250,
    5000.00,
    2000,
    640,
    500,
    490,
    20.00,
    '2026-01-01',
    '2026-12-31',
    '2026-07-01'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO conservation_projects (
    id, zone_id, title, description, objectives, category, species_or_type_name, 
    image_url, status, physical_health_status, unit_name, currency_code, 
    price_per_unit_money, price_per_unit_points, target_amount, target_units, 
    funded_units_count, planted_units_count, surviving_units_count, replant_reserve_pct, 
    donation_start_date, donation_end_date, estimated_planting_date
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4203',
    '0193297a-96e0-7988-9271-e0e649ce4103',
    'Rinjani Forest Canopy Restoration',
    'Penanaman pohon endemik dan tanaman keras agroforestry di lereng Gunung Rinjani untuk mencegah longsor dan memulihkan koridor hidrologis Pulau Lombok.',
    'Menanam 1.000 pohon kemiri dan beringin demi melindungi mata air alami masyarakat kaki Rinjani.',
    'tree',
    'Aleurites moluccanus & Ficus',
    'https://images.unsplash.com/photo-1448375240586-882707db888b',
    'open_for_donation',
    'healthy',
    'tree',
    'USD',
    4.00,
    400,
    4000.00,
    1000,
    310,
    200,
    195,
    15.00,
    '2026-01-01',
    '2026-12-31',
    '2026-08-15'
) ON CONFLICT (id) DO NOTHING;

-- 5. Seed News Articles
INSERT INTO news_articles (
    id, title, category, summary, content, thumbnail_url, is_featured, published_at
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4501',
    'Bibit Karang Pemuteran Tumbuh 3 Kali Lebih Cepat Berkat Teknologi Biorock',
    'coral',
    'Monitoring terbaru di Teluk Pemuteran menunjukkan fragmen Acropora berkembang pesat dengan tingkat kelangsungan hidup mencapai 95%.',
    'Tim konservasi lokal di Teluk Pemuteran, Bali Utara, merilis laporan pemantauan triwulan pertama tahun 2026. Melalui struktur Biorock bertegangan rendah, endapan mineral kalsium karbonat memperkuat kerangka karang yang baru diadopsi oleh para donatur BioVerse. Pertumbuhan cabang terbukti mencapai 3 kali kecepatan alami, menarik kembali anemon dan biota laut langka.',
    'https://images.unsplash.com/photo-1546026423-cc4642628d2b',
    true,
    NOW() - INTERVAL '2 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO news_articles (
    id, title, category, summary, content, thumbnail_url, is_featured, published_at
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4502',
    'Benteng Hijau Pesisir: 500 Bibit Mangrove Rhizophora Ditanam di Tanjung Lesung',
    'mangrove',
    'Kolaborasi relawan dan kelompok tani pesisir memperkuat garis pertahanan alami Banten dari ancaman abrasi Selat Sunda.',
    'Aksi penanaman mangrove di pesisir Tanjung Lesung berhasil menancapkan 500 bibit mangrove berakar tunjang kuat. Program ini didanai melalui donasi B2C dan program ESG korporat di platform BioVerse. Selain menahan erosi pantai, zona mangrove ini diproyeksikan menyerap hingga 12 ton CO2 per hektar setiap tahunnya.',
    'https://images.unsplash.com/photo-1544551763-46a013bb70d5',
    false,
    NOW() - INTERVAL '5 days'
) ON CONFLICT (id) DO NOTHING;

INSERT INTO news_articles (
    id, title, category, summary, content, thumbnail_url, is_featured, published_at
) VALUES (
    '0193297a-96e0-7988-9271-e0e649ce4503',
    'Restorasi Hutan Rinjani Mengembalikan Sumber Mata Air Warga Sembalun',
    'tree',
    'Pohon beringin dan kemiri yang ditanam tahun lalu kini mulai menahan limpasan air hujan di hulu lereng Rinjani.',
    'Laporan dari tim lapangan Rinjani menunjukkan kemajuan menggembirakan. Kanopi pepohonan yang mulai rimbun membantu meningkatkan resapan air tanah, sehingga debit mata air di perkampungan kaki bukit tetap stabil menjelang musim kemarau. Donatur kini dapat melihat foto berkala progres pohon adopsi mereka di aplikasi BioVerse.',
    'https://images.unsplash.com/photo-1448375240586-882707db888b',
    false,
    NOW() - INTERVAL '7 days'
) ON CONFLICT (id) DO NOTHING;

-- 6. Update seed eco_missions with real images and estimated minutes
UPDATE eco_missions
SET image_url = 'https://images.unsplash.com/photo-1602143407151-7111542de6e8',
    estimated_minutes = 5
WHERE title = 'Bawa Tumbler Sendiri';

UPDATE eco_missions
SET image_url = 'https://images.unsplash.com/photo-1530587191325-3db32d826c18',
    estimated_minutes = 10
WHERE title = 'Pilah Sampah Plastik';

UPDATE eco_missions
SET image_url = 'https://images.unsplash.com/photo-1509395176047-4a66953fd231',
    estimated_minutes = 15
WHERE title = 'Hemat Energi Listrik Siang Hari';

-- +goose Down
-- Revert is safe
