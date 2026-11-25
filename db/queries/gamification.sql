-- name: CountActiveCoralFragmentsByUserID :one
SELECT COUNT(*) FROM coral_fragments
WHERE adopted_by_user_id = $1 AND lifecycle_status != 'deceased';

-- name: ListVirtualPetsByUserID :many
SELECT vp.id, vp.user_id, vp.fragment_id, vp.equipped_product_id, vp.pet_name,
       vp.pet_species, vp.coral_type, vp.growth_stage, vp.health_percent,
       vp.level, vp.experience_points, vp.is_primary, vp.last_interacted_at,
       vp.adopted_at, vp.created_at,
       cf.fragment_code, cf.nickname AS coral_nickname
FROM virtual_pets vp
LEFT JOIN coral_fragments cf ON vp.fragment_id = cf.id
WHERE vp.user_id = $1
ORDER BY vp.is_primary DESC, vp.created_at ASC;

-- name: GetVirtualPetByID :one
SELECT vp.id, vp.user_id, vp.fragment_id, vp.equipped_product_id, vp.pet_name,
       vp.pet_species, vp.coral_type, vp.growth_stage, vp.health_percent,
       vp.level, vp.experience_points, vp.is_primary, vp.last_interacted_at,
       vp.adopted_at, vp.created_at,
       cf.fragment_code, cf.nickname AS coral_nickname
FROM virtual_pets vp
LEFT JOIN coral_fragments cf ON vp.fragment_id = cf.id
WHERE vp.id = $1 AND vp.user_id = $2
LIMIT 1;

-- name: CreateVirtualPet :one
INSERT INTO virtual_pets (
    id, user_id, fragment_id, pet_name, pet_species, coral_type, is_primary
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: ClearUserPrimaryPets :exec
UPDATE virtual_pets
SET is_primary = false
WHERE user_id = $1;

-- name: SetPrimaryPet :one
UPDATE virtual_pets
SET is_primary = true
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: UpdateVirtualPetHealthAndXP :one
UPDATE virtual_pets
SET health_percent = $3,
    experience_points = $4,
    level = $5,
    growth_stage = $6,
    last_interacted_at = $7
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: CreatePetBoost :one
INSERT INTO pet_boosts (
    id, user_id, pet_id, donation_id, product_id, boost_type, xp_multiplier, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: ListActivePetBoosts :many
SELECT id, user_id, pet_id, donation_id, product_id, boost_type, xp_multiplier, is_active, started_at, expires_at
FROM pet_boosts
WHERE pet_id = $1 AND is_active = true AND (expires_at IS NULL OR expires_at > NOW());

-- name: UpdateUserStreak :one
UPDATE user_streaks
SET current_streak = $2,
    longest_streak = $3,
    last_streak_date = $4,
    last_freeze_used_date = $5,
    updated_at = NOW()
WHERE user_id = $1
RETURNING user_id, current_streak, longest_streak, last_streak_date, last_freeze_used_date, updated_at;

-- name: ListActiveMissions :many
SELECT id, sponsor_company_id, title, description, instructions, category, difficulty, image_url,
       points_reward, xp_reward, requires_camera_proof, cv_target_label, cv_min_confidence, is_daily, is_active, 
       estimated_minutes, created_at
FROM eco_missions
WHERE is_active = true
ORDER BY is_daily DESC, points_reward ASC;

-- name: ListActiveMissionsForUser :many
SELECT m.id, m.sponsor_company_id, m.title, m.description, m.instructions, m.category, m.difficulty, m.image_url,
       m.points_reward, m.xp_reward, m.requires_camera_proof, m.cv_target_label, m.cv_min_confidence, m.is_daily, m.is_active, 
       m.estimated_minutes, m.created_at,
       COALESCE((
           SELECT s.status::text 
           FROM user_mission_submissions s 
           WHERE s.mission_id = m.id AND s.user_id = $1
           ORDER BY s.submitted_at DESC 
           LIMIT 1
       ), '')::text AS my_last_submission_status,
       EXISTS (
           SELECT 1 
           FROM user_mission_submissions s 
           WHERE s.mission_id = m.id AND s.user_id = $1 AND s.submission_date = $2 AND s.status != 'rejected'
       ) AS completed_today
FROM eco_missions m
WHERE m.is_active = true
ORDER BY m.is_daily DESC, m.points_reward ASC;

-- name: GetMissionByID :one
SELECT id, sponsor_company_id, title, description, instructions, category, difficulty, image_url,
       points_reward, xp_reward, requires_camera_proof, cv_target_label, cv_min_confidence, is_daily, is_active, 
       estimated_minutes, created_at
FROM eco_missions
WHERE id = $1 AND is_active = true
LIMIT 1;

-- name: GetTodaySubmissionForMission :one
SELECT * FROM user_mission_submissions
WHERE user_id = $1 AND mission_id = $2 AND submission_date = $3 AND status != 'rejected'
LIMIT 1;

-- name: CreateMissionSubmission :one
INSERT INTO user_mission_submissions (
    id, user_id, mission_id, pet_id, submission_date, proof_image_url, image_hash, status,
    cv_job_id, cv_model_version, cv_confidence_score, cv_raw_response, awarded_points, awarded_xp, submitted_at, verified_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12, $13, $14, NOW(), $15
) RETURNING *;

-- name: GetMissionSubmissionByID :one
SELECT * FROM user_mission_submissions
WHERE id = $1 AND user_id = $2
LIMIT 1;

-- name: ListSubmissionsByUserID :many
SELECT s.*, m.title AS mission_title, m.category AS mission_category
FROM user_mission_submissions s
JOIN eco_missions m ON s.mission_id = m.id
WHERE s.user_id = $1
ORDER BY s.submitted_at DESC
LIMIT $2 OFFSET $3;

-- name: ListPendingMissionSubmissions :many
SELECT s.*, m.cv_target_label, m.cv_min_confidence
FROM user_mission_submissions s
JOIN eco_missions m ON s.mission_id = m.id
WHERE s.status = 'pending'
ORDER BY s.submitted_at ASC
LIMIT $1;

-- name: UpdateMissionSubmissionStatus :one
UPDATE user_mission_submissions
SET status = $2,
    cv_confidence_score = $3,
    cv_model_version = $4,
    rejection_reason = $5,
    verified_at = $6
WHERE id = $1
RETURNING *;

-- name: CreditUserPoints :one
UPDATE users
SET points_balance = points_balance + $2
WHERE id = $1
RETURNING id, points_balance, locked_points_balance;

-- name: GetLeaderboard :many
SELECT 
    u.id, 
    u.full_name, 
    u.avatar_url, 
    u.points_balance,
    COALESCE(SUM(d.gross_project_value), 0)::numeric AS total_donations_usd,
    COALESCE(SUM(d.unit_quantity), 0)::bigint AS total_units_adopted,
    COALESCE(BOOL_OR(d.is_anonymous), false)::boolean AS is_anonymous,
    ROW_NUMBER() OVER (ORDER BY COALESCE(SUM(d.gross_project_value), 0) DESC, u.points_balance DESC, u.created_at ASC) AS rank_position
FROM users u
JOIN donations d ON u.id = d.user_id AND d.payment_status = 'paid'
    AND (sqlc.narg('period_start')::timestamptz IS NULL OR d.paid_at >= sqlc.narg('period_start')::timestamptz)
JOIN conservation_projects cp ON d.project_id = cp.id
    AND (sqlc.narg('category')::text IS NULL OR sqlc.narg('category')::text = 'all' OR cp.category::text = sqlc.narg('category')::text)
WHERE u.deleted_at IS NULL
GROUP BY u.id, u.full_name, u.avatar_url, u.points_balance, u.created_at
HAVING COALESCE(SUM(d.gross_project_value), 0) > 0
ORDER BY total_donations_usd DESC, u.points_balance DESC, u.created_at ASC
LIMIT $1 OFFSET $2;

-- name: CreatePointLedgerEntry :one
INSERT INTO point_ledgers (
    id, user_id, transaction_type, amount, balance_after, reference_type, reference_id, description
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: LockUserPoints :one
UPDATE users
SET locked_points_balance = locked_points_balance + $2
WHERE id = $1 AND (points_balance - locked_points_balance) >= $2
RETURNING id, points_balance, locked_points_balance;

-- name: UnlockUserPoints :one
UPDATE users
SET locked_points_balance = locked_points_balance - $2
WHERE id = $1 AND locked_points_balance >= $2
RETURNING id, points_balance, locked_points_balance;

-- name: SpendUserPoints :one
UPDATE users
SET points_balance = points_balance - $2,
    locked_points_balance = locked_points_balance - $2
WHERE id = $1 AND points_balance >= $2 AND locked_points_balance >= $2
RETURNING id, points_balance, locked_points_balance;

-- name: EquipVirtualPetProduct :one
UPDATE virtual_pets
SET equipped_product_id = $3
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: CreateMission :one
INSERT INTO eco_missions (
    id, sponsor_company_id, title, description, instructions, category, difficulty,
    image_url, points_reward, xp_reward, requires_camera_proof, cv_target_label,
    cv_min_confidence, is_daily, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12,
    $13, $14, $15
) RETURNING *;

-- name: UpdateMission :one
UPDATE eco_missions
SET title = $2, description = $3, instructions = $4,
    points_reward = $5, xp_reward = $6, is_active = $7
WHERE id = $1
RETURNING *;

-- name: ListAllMissions :many
SELECT * FROM eco_missions
ORDER BY created_at DESC;

-- name: ListSubmissionsForAdmin :many
SELECT s.id, s.user_id, s.mission_id, s.pet_id, s.submission_date, s.proof_image_url,
       s.status, s.cv_confidence_score, s.cv_model_version, s.cv_raw_response,
       s.awarded_points, s.awarded_xp, s.rejection_reason, s.submitted_at, s.verified_at,
       u.email AS user_email, u.full_name AS user_full_name,
       m.title AS mission_title, m.points_reward AS mission_points_reward, m.xp_reward AS mission_xp_reward
FROM user_mission_submissions s
JOIN users u ON s.user_id = u.id
JOIN eco_missions m ON s.mission_id = m.id
WHERE (sqlc.narg('status')::submission_status IS NULL OR s.status = sqlc.narg('status')::submission_status)
ORDER BY s.submitted_at DESC
LIMIT $1 OFFSET $2;

-- name: GetMissionSubmissionForAdmin :one
SELECT s.*, m.title AS mission_title, m.points_reward AS mission_points_reward, m.xp_reward AS mission_xp_reward
FROM user_mission_submissions s
JOIN eco_missions m ON s.mission_id = m.id
WHERE s.id = $1
LIMIT 1;

-- name: ReviewMissionSubmission :one
UPDATE user_mission_submissions
SET status = $2,
    awarded_points = $3,
    awarded_xp = $4,
    rejection_reason = $5,
    verified_at = NOW()
WHERE id = $1 AND status IN ('needs_review', 'pending', 'processing')
RETURNING *;
