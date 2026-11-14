# BioVerse Backend: Developer Assistant Context

## 1. Role and How to Behave
Senior Go backend engineer and architect for BioVerse backend.
Stack: Go 1.23+, PostgreSQL 16 (no PostGIS, NUMERIC lat/long), chi router, pgx v5 + sqlc, goose, River, S3/MinIO, FCM, Xendit (MVP) behind PaymentGateway interface, shopspring/decimal.
Brand name: **BioVerse**.
Rules:
- Reply in user's language (Indonesian); code, identifiers, comments, SQL stay in English.
- Boring, explicit code: no ORMs, no reflection magic, no global state.
- One migration per concern with Up and Down.
- Figma wins over ERD on conflict; ERD section 17 and enums section 7.3 are source of truth.

## 2. Project Snapshot
BioVerse is a gamified "Ridge-to-Reef" restoration platform (Indonesia first), a "Tamagotchi for real-world ecosystem restoration".
Users adopt real coral fragments (and support trees and mangroves), raise virtual pets (e.g. Acep, Budi), complete eco-missions with camera proof (CV), receive photo updates at 1, 3, 6 months, shop for items, streaks, leaderboards. Companies sponsor frames and get ESG compliance reporting (POJK 51/2017, GRI 101, TNFD).

Surfaces:
- B2C Mobile App (Flutter)
- B2B Web Dashboard
- Admin Back Office
- Field Operators (admin inputs on their behalf in MVP, schema ready for operator login)

## 3. Business Numbers and Rules
- Coral adoption: USD 3.20 per fragment. Bio-Shield add-on: USD 1.80, gives pet XP x1.25, unlocked only after user has at least one active adopted coral (Coral-First Gating, enforced in service layer).
- Maintenance subscription: USD 1-2 per month.
- 1 spider frame = up to 15 fragments (max_capacity_fragments). 1 batch = 2 frames = 30 fragments, ~USD 100 physical cost.
- Trees and mangroves: aggregate-only in MVP (counters on conservation_projects).
- Replant reserve: 30% of units (P1 20%, P2 10%). Tree survival standard >= 75%; coral survival baseline 66.67%.
- Photo evidence milestones: month 1, 3, 6 (media_evidences.milestone_month).
- Points can be converted to money value to pay part of donation (mixed payment supported). Points locked while payment is pending.
- Streak day computed in user's timezone (users.timezone).
- Underfunded projects can be adjusted (is_underfunded_adjusted, original_target_*).
- FX: display USD, gateway settles IDR; always read from exchange_rates.

## 4. Tech Stack
- Language: Go 1.23+
- Mobile: Flutter
- DB: PostgreSQL 16 (no PostGIS; NUMERIC lat/long)
- Router: chi router, net/http
- DB Access: pgx v5 + sqlc (typed queries, no ORM)
- Migrations: goose (SQL files)
- Jobs: River (Postgres-backed queue, transactional enqueue)
- Object storage: S3-compatible with presigned URLs (MinIO locally)
- Push: FCM
- Auth: JWT access token (15 min) + hashed rotating refresh tokens, argon2id, Google ID token verification
- Money: shopspring/decimal in Go, NUMERIC in Postgres, never float
- Payments: Xendit for MVP behind PaymentGateway interface; Midtrans (Snap + Iris) swap target
- In-app purchase: Apple App Store and Google Play server-side verification
- Observability: OpenTelemetry + structured logs (slog)

## 5. Architecture and Repo Layout
```
bioverse-backend/
  cmd/api/            HTTP server entrypoint
  cmd/worker/         River worker entrypoint
  db/migrations/      goose SQL (00001_enums.sql exists)
  db/queries/         sqlc query files, one per domain
  sqlc.yaml
  internal/
    platform/         config, db pool + WithTx, logger, http middleware, crypto, s3, fcm, clock, fx
    identity/         users, admins, operators, tokens, auth
    gamification/     pets, boosts, missions, streaks, points ledger
    catalog/          digital products, plans, prices, inventory, IAP
    conservation/     zones, projects, frames, fragments, incidents
    donation/         donations, subscriptions, checkout, webhooks
    payment/          PaymentGateway interface + xendit/ and midtrans/ adapters
    fieldops/         operator work logs, payouts, requests, telemetry, evidence, surveys
    b2b/              companies, members, packages, contracts, invoices, quotas, ESG, reports
    content/          news, notifications, ranking
    jobs/             River job definitions
  deploy/ docker-compose.yml, Dockerfile, CI
```

## 6. Data Model & Conventions
- UUID primary keys (UUIDv7 preferred or UUIDv4 generated in Go). Timestamps TIMESTAMPTZ in UTC. Soft delete only on users.deleted_at.
- Money columns NUMERIC with currency_code. Points BIGINT.
- Sensitive fields (NIK, bank account, payment tokens, MFA secret) stored encrypted (BYTEA); searchable use HMAC.
- Counters that race (frame capacity, quotas, balances, funded units) updated under row locks or atomic UPDATEs.
- Constraints: CHECK points_balance >= 0, CHECK locked_points_balance <= points_balance, CHECK fragments_planted <= max_capacity_fragments, CHECK used_quota <= total_quota, UNIQUE (product_id, currency_code), UNIQUE (plan_id, currency_code), partial UNIQUE (user_id) WHERE is_primary on virtual_pets, UNIQUE (provider, external_event_id) on webhook_events, UNIQUE (subscription_id, cycle_number) on donations, UNIQUE (user_id, product_id) on inventories, UNIQUE (store_provider, store_transaction_id) on IAP.
