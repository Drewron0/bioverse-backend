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

-- name: ListActiveSubscriptionPlans :many
SELECT sp.id, sp.sku_code, sp.group_code, sp.name, sp.description,
       sp.plan_type, sp.billing_interval, sp.interval_count, sp.duration_months,
       sp.benefits, spp.currency_code, spp.amount AS price_money
FROM subscription_plans sp
LEFT JOIN subscription_plan_prices spp ON sp.id = spp.plan_id AND spp.currency_code = $1
WHERE sp.is_active = true
ORDER BY sp.sort_order ASC;
