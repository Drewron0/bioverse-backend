-- name: ListActiveDigitalProducts :many
SELECT dp.id, dp.sku_code, dp.name, dp.description, dp.item_type,
       dp.price_points, dp.is_iap_only, dp.attributes, dp.preview_asset_url,
       pp.currency_code, pp.amount AS price_money
FROM digital_products dp
LEFT JOIN product_prices pp ON dp.id = pp.product_id AND pp.currency_code = $1
WHERE dp.is_active = true
ORDER BY dp.sku_code ASC;

-- name: GetDigitalProductBySKU :one
SELECT dp.id, dp.sku_code, dp.name, dp.description, dp.item_type,
       dp.price_points, dp.is_iap_only, dp.attributes, dp.preview_asset_url,
       pp.currency_code, pp.amount AS price_money
FROM digital_products dp
LEFT JOIN product_prices pp ON dp.id = pp.product_id AND pp.currency_code = $2
WHERE dp.sku_code = $1 AND dp.is_active = true
LIMIT 1;

-- name: GetDigitalProductByID :one
SELECT dp.id, dp.sku_code, dp.name, dp.description, dp.item_type,
       dp.price_points, dp.is_iap_only, dp.attributes, dp.preview_asset_url
FROM digital_products dp
WHERE dp.id = $1 AND dp.is_active = true
LIMIT 1;

-- name: ListActiveSubscriptionPlans :many
SELECT sp.id, sp.sku_code, sp.group_code, sp.name, sp.description,
       sp.plan_type, sp.billing_interval, sp.interval_count, sp.duration_months,
       sp.benefits, spp.currency_code, spp.amount AS price_money
FROM subscription_plans sp
LEFT JOIN subscription_plan_prices spp ON sp.id = spp.plan_id AND spp.currency_code = $1
WHERE sp.is_active = true
ORDER BY sp.sort_order ASC;

-- name: GetSubscriptionPlanByID :one
SELECT sp.id, sp.sku_code, sp.group_code, sp.name, sp.description,
       sp.plan_type, sp.billing_interval, sp.interval_count, sp.duration_months,
       sp.benefits, spp.currency_code, spp.amount AS price_money
FROM subscription_plans sp
LEFT JOIN subscription_plan_prices spp ON sp.id = spp.plan_id AND spp.currency_code = $2
WHERE sp.id = $1 AND sp.is_active = true
LIMIT 1;

-- name: ListUserInventory :many
SELECT udi.id, udi.user_id, udi.product_id, udi.quantity, udi.acquired_at,
       dp.sku_code, dp.name, dp.description, dp.item_type, dp.attributes, dp.preview_asset_url
FROM user_digital_inventories udi
JOIN digital_products dp ON udi.product_id = dp.id
WHERE udi.user_id = $1 AND udi.quantity > 0
ORDER BY udi.acquired_at DESC;

-- name: GetUserInventoryItem :one
SELECT udi.id, udi.user_id, udi.product_id, udi.quantity, udi.acquired_at,
       dp.sku_code, dp.name, dp.description, dp.item_type, dp.attributes, dp.preview_asset_url
FROM user_digital_inventories udi
JOIN digital_products dp ON udi.product_id = dp.id
WHERE udi.user_id = $1 AND dp.id = $2
LIMIT 1;

-- name: UpsertUserInventoryItem :one
INSERT INTO user_digital_inventories (id, user_id, product_id, quantity)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, product_id)
DO UPDATE SET quantity = user_digital_inventories.quantity + EXCLUDED.quantity
RETURNING *;

-- name: DecrementUserInventoryItem :one
UPDATE user_digital_inventories
SET quantity = quantity - $3
WHERE user_id = $1 AND product_id = $2 AND quantity >= $3
RETURNING *;

-- name: CreateDonationSubscription :one
INSERT INTO donation_subscriptions (
    id, user_id, plan_id, project_id, frequency, duration_months, unit_quantity,
    currency_code, amount_per_cycle, payment_method, payment_provider, status, next_billing_date
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13
) RETURNING *;

-- name: ListUserSubscriptions :many
SELECT ds.id, ds.user_id, ds.plan_id, ds.project_id, ds.frequency, ds.duration_months,
       ds.completed_cycles, ds.unit_quantity, ds.currency_code, ds.amount_per_cycle,
       ds.payment_method, ds.payment_provider, ds.failed_charge_count, ds.status,
       ds.next_billing_date, ds.created_at,
       sp.name AS plan_name, sp.sku_code AS plan_sku
FROM donation_subscriptions ds
LEFT JOIN subscription_plans sp ON ds.plan_id = sp.id
WHERE ds.user_id = $1
ORDER BY ds.created_at DESC;

-- name: UpdateSubscriptionStatus :one
UPDATE donation_subscriptions
SET status = $2, paused_at = $3, pause_reason = $4
WHERE id = $1 AND user_id = $5
RETURNING *;

-- name: CreateIAPTransaction :one
INSERT INTO iap_transactions (
    id, user_id, product_id, store_provider, store_transaction_id, environment,
    receipt_payload, currency_code, gross_amount, store_fee_amount, net_revenue_amount, status
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12
) RETURNING *;

-- name: GetIAPTransactionByStoreTxID :one
SELECT * FROM iap_transactions
WHERE store_provider = $1 AND store_transaction_id = $2
LIMIT 1;
