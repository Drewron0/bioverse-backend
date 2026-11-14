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
       cp.title AS project_title, cz.name AS zone_name
FROM coral_fragments cf
JOIN conservation_projects cp ON cf.project_id = cp.id
JOIN conservation_zones cz ON cp.zone_id = cz.id
WHERE cf.adopted_by_user_id = $1
ORDER BY cf.created_at DESC;
