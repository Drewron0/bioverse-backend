-- name: CreateCompany :one
INSERT INTO companies (
    id, legal_name, idx_ojk_ticker, vector_logo_url, pic_name,
    work_email, contact_phone, mandatory_framework, biodiversity_standard,
    reporting_frequency, annual_fragment_target, default_plaque_inscription_text
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12
) RETURNING *;

-- name: GetCompanyByID :one
SELECT * FROM companies
WHERE id = $1
LIMIT 1;

-- name: ListCompanies :many
SELECT * FROM companies
ORDER BY legal_name ASC;

-- name: CreateCompanyMember :one
INSERT INTO company_members (
    id, company_id, user_id, invited_email, full_name, role, permission, is_accepted
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: ListCompanyMembers :many
SELECT * FROM company_members
WHERE company_id = $1
ORDER BY role ASC, full_name ASC;

-- name: ListActiveB2BPackages :many
SELECT * FROM b2b_packages
WHERE is_active = true
ORDER BY price_amount ASC;

-- name: CreateB2BContract :one
INSERT INTO b2b_contracts (
    id, company_id, package_id, created_by_member_id, status,
    currency_code, contract_value, start_date, end_date, auto_renew
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10
) RETURNING *;

-- name: ListContractsByCompanyID :many
SELECT bc.*, bp.name AS package_name
FROM b2b_contracts bc
JOIN b2b_packages bp ON bc.package_id = bp.id
WHERE bc.company_id = $1
ORDER BY bc.start_date DESC;

-- name: CreateSponsoredAdoptionQuota :one
INSERT INTO sponsored_adoption_quotas (
    id, company_id, contract_id, project_id, total_quota, used_quota
) VALUES (
    $1, $2, $3, $4, $5, 0
) RETURNING *;

-- name: GetSponsoredAdoptionQuota :one
SELECT * FROM sponsored_adoption_quotas
WHERE id = $1
LIMIT 1;

-- name: ClaimSponsoredQuota :one
UPDATE sponsored_adoption_quotas
SET used_quota = used_quota + 1
WHERE id = $1 AND (total_quota - used_quota) >= 1
RETURNING *;

-- name: CreateESGBiodiversityMetric :one
INSERT INTO esg_biodiversity_metrics (
    id, company_id, fiscal_year, fiscal_quarter, indicator_name,
    unit, baseline_value, current_value, target_value, variance_percentage, status
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: ListESGMetricsByCompany :many
SELECT * FROM esg_biodiversity_metrics
WHERE company_id = $1 AND fiscal_year = $2
ORDER BY fiscal_quarter DESC, indicator_name ASC;

-- name: CreateESGAuditReport :one
INSERT INTO esg_audit_reports (
    id, company_id, generated_by_member_id, title, fiscal_year, fiscal_quarter,
    reporting_period_label, pojk51_readiness_pct, gri_criteria_met, assurance_grade,
    chapter_pdf_url, raw_dataset_csv_url, auditor_status
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13
) RETURNING *;

-- name: ListESGAuditReportsByCompany :many
SELECT * FROM esg_audit_reports
WHERE company_id = $1
ORDER BY fiscal_year DESC, fiscal_quarter DESC;
