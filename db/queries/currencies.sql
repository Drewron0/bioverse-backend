-- name: GetCurrencyByCode :one
SELECT code, name, symbol, minor_unit_digits, is_display_enabled, is_active, created_at
FROM currencies
WHERE code = $1 AND is_active = true
LIMIT 1;

-- name: ListActiveCurrencies :many
SELECT code, name, symbol, minor_unit_digits, is_display_enabled, is_active, created_at
FROM currencies
WHERE is_active = true
ORDER BY code ASC;

-- name: GetLatestExchangeRate :one
SELECT id, base_code, quote_code, rate, source, effective_at, created_at
FROM exchange_rates
WHERE base_code = $1 AND quote_code = $2
ORDER BY effective_at DESC
LIMIT 1;
