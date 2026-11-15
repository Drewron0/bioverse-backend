-- name: ListConservationProjects :many
SELECT cp.id, cp.zone_id, cp.title, cp.description, cp.objectives, cp.category,
       cp.species_or_type_name, cp.image_url, cp.status, cp.physical_health_status,
       cp.unit_name, cp.currency_code, cp.price_per_unit_money, cp.price_per_unit_points,
       cp.target_amount, cp.target_units, cp.funded_units_count, cp.planted_units_count,
       cp.surviving_units_count, cp.replant_reserve_pct, cp.donation_start_date,
       cp.donation_end_date, cp.estimated_planting_date,
       cz.name AS zone_name, cz.region AS zone_region, cz.province AS zone_province,
       cz.ecosystem_type AS zone_ecosystem_type, cz.latitude AS zone_latitude, cz.longitude AS zone_longitude
FROM conservation_projects cp
JOIN conservation_zones cz ON cp.zone_id = cz.id
WHERE cp.status = 'open_for_donation'
ORDER BY cp.created_at DESC;

-- name: GetConservationProjectByID :one
SELECT cp.id, cp.zone_id, cp.title, cp.description, cp.objectives, cp.category,
       cp.species_or_type_name, cp.image_url, cp.status, cp.physical_health_status,
       cp.unit_name, cp.currency_code, cp.price_per_unit_money, cp.price_per_unit_points,
       cp.target_amount, cp.target_units, cp.funded_units_count, cp.planted_units_count,
       cp.surviving_units_count, cp.replant_reserve_pct, cp.donation_start_date,
       cp.donation_end_date, cp.estimated_planting_date,
       cz.name AS zone_name, cz.region AS zone_region, cz.province AS zone_province,
       cz.ecosystem_type AS zone_ecosystem_type, cz.latitude AS zone_latitude, cz.longitude AS zone_longitude
FROM conservation_projects cp
JOIN conservation_zones cz ON cp.zone_id = cz.id
WHERE cp.id = $1
LIMIT 1;

-- name: GetOpenSpiderFrameForZone :one
SELECT id, frame_code, zone_id, depth_meters, latitude, longitude,
       max_capacity_fragments, fragments_planted, physical_health_status
FROM spider_frames
WHERE zone_id = $1 AND company_id IS NULL AND is_active = true
  AND fragments_planted < max_capacity_fragments
ORDER BY fragments_planted DESC
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: IncrementSpiderFrameFragmentsPlanted :one
UPDATE spider_frames
SET fragments_planted = fragments_planted + $2
WHERE id = $1
RETURNING id, fragments_planted, max_capacity_fragments;

-- name: IncrementProjectFundedUnits :one
UPDATE conservation_projects
SET funded_units_count = funded_units_count + $2
WHERE id = $1
RETURNING id, funded_units_count, target_units;

-- name: CreateCoralFragment :one
INSERT INTO coral_fragments (
    id, fragment_code, nickname, project_id, donation_id, spider_frame_id, 
    adopted_by_user_id, species_name, lifecycle_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, 'pending_planting'
) RETURNING id, fragment_code, nickname, project_id, donation_id, spider_frame_id, 
            adopted_by_user_id, species_name, lifecycle_status, created_at;

-- name: ListCoralFragmentsByUserID :many
SELECT cf.id, cf.fragment_code, cf.nickname, cf.project_id, cf.donation_id,
       cf.spider_frame_id, cf.species_name, cf.lifecycle_status, cf.health_status,
       cf.latest_photo_url, cf.planted_at, cf.last_monitored_at, cf.created_at,
       cp.title AS project_title, cz.name AS zone_name, sf.frame_code AS spider_frame_tag
FROM coral_fragments cf
JOIN conservation_projects cp ON cf.project_id = cp.id
JOIN conservation_zones cz ON cp.zone_id = cz.id
LEFT JOIN spider_frames sf ON cf.spider_frame_id = sf.id
WHERE cf.adopted_by_user_id = $1
ORDER BY cf.created_at DESC;

-- name: GetCoralFragmentByID :one
SELECT cf.id, cf.fragment_code, cf.nickname, cf.project_id, cf.donation_id,
       cf.spider_frame_id, cf.adopted_by_user_id, cf.species_name, cf.lifecycle_status, cf.health_status,
       cf.latest_photo_url, cf.planted_at, cf.last_monitored_at, cf.created_at,
       cp.title AS project_title, cz.name AS zone_name, sf.frame_code AS spider_frame_tag
FROM coral_fragments cf
JOIN conservation_projects cp ON cf.project_id = cp.id
JOIN conservation_zones cz ON cp.zone_id = cz.id
LEFT JOIN spider_frames sf ON cf.spider_frame_id = sf.id
WHERE cf.id = $1
LIMIT 1;

-- name: ListCoralMilestonesByFragmentID :many
SELECT me.id, me.milestone_month, me.milestone_title, me.media_url, 
       me.is_verified_healthy, me.subtitle_metrics, me.captured_at, me.created_at,
       cf.health_status
FROM media_evidences me
JOIN coral_fragments cf ON me.coral_fragment_id = cf.id
WHERE me.coral_fragment_id = $1
ORDER BY me.milestone_month ASC;

-- name: ListNewsArticles :many
SELECT id, title, category, summary, content, thumbnail_url, is_featured, published_at, created_at
FROM news_articles
ORDER BY published_at DESC
LIMIT $1;

-- name: GetPlatformImpactSummary :one
SELECT 
    COALESCE((SELECT COUNT(*) FROM coral_fragments WHERE lifecycle_status != 'deceased'), 0)::bigint AS total_corals_adopted,
    COALESCE((SELECT SUM(planted_units_count) FROM conservation_projects WHERE category = 'tree'), 0)::bigint AS total_trees_planted,
    COALESCE((SELECT COUNT(DISTINCT adopted_by_user_id) FROM coral_fragments WHERE adopted_by_user_id IS NOT NULL), 0)::bigint AS active_guardians_count,
    COALESCE((SELECT SUM(planted_units_count) * 2.5 FROM conservation_projects), 0)::numeric AS total_biomass_restored_kg;

-- name: CreateConservationZone :one
INSERT INTO conservation_zones (
    id, name, region, province, ecosystem_type, allocation_type,
    latitude, longitude, min_depth_meters, max_depth_meters
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: ListConservationZones :many
SELECT * FROM conservation_zones
ORDER BY name ASC;

-- name: GetConservationZoneByID :one
SELECT * FROM conservation_zones
WHERE id = $1
LIMIT 1;

-- name: UpdateConservationZone :one
UPDATE conservation_zones
SET name = $2, region = $3, province = $4, ecosystem_type = $5,
    allocation_type = $6, latitude = $7, longitude = $8,
    min_depth_meters = $9, max_depth_meters = $10
WHERE id = $1
RETURNING *;

-- name: CreateConservationProject :one
INSERT INTO conservation_projects (
    id, zone_id, title, description, objectives, category,
    species_or_type_name, image_url, status, physical_health_status,
    unit_name, currency_code, price_per_unit_money, price_per_unit_points,
    target_amount, target_units, replant_reserve_pct,
    donation_start_date, donation_end_date, estimated_planting_date
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
) RETURNING *;

-- name: UpdateConservationProject :one
UPDATE conservation_projects
SET title = $2, description = $3, objectives = $4,
    status = $5, physical_health_status = $6,
    price_per_unit_money = $7, price_per_unit_points = $8
WHERE id = $1
RETURNING *;

-- name: CreateSpiderFrame :one
INSERT INTO spider_frames (
    id, frame_code, zone_id, company_id, depth_meters, latitude, longitude,
    max_capacity_fragments, physical_health_status, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING *;

-- name: ListSpiderFramesByZoneID :many
SELECT * FROM spider_frames
WHERE zone_id = $1
ORDER BY frame_code ASC;

-- name: CreateNewsArticle :one
INSERT INTO news_articles (
    id, title, category, summary, content, thumbnail_url, is_featured, published_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: UpdateNewsArticle :one
UPDATE news_articles
SET title = $2, category = $3, summary = $4, content = $5,
    thumbnail_url = $6, is_featured = $7
WHERE id = $1
RETURNING *;

-- name: DeleteNewsArticle :exec
DELETE FROM news_articles
WHERE id = $1;

-- name: GetNewsArticleByID :one
SELECT * FROM news_articles
WHERE id = $1
LIMIT 1;
