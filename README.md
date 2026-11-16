# BioVerse Backend

Backend API and background worker engine for **BioVerse** - a gamified "Ridge-to-Reef" ecosystem restoration platform ("Tamagotchi for real-world ecosystem restoration").

BioVerse powers:
- **B2C Mobile App (Flutter)**: Coral fragment adoption, Bio-Shield add-on, virtual pets, streaks, computer vision eco-missions, restoration progress timelines.
- **B2B Web Dashboard**: ESG compliance reporting (POJK 51/2017, GRI 101 Biodiversity, TNFD), plaque previews, field service requests.
- **Admin Back Office**: Multi-zone and project management, spider frames, fragment allocation, operator payouts, audit logs.

---

## Tech Stack

| Component | Technology | Description |
|---|---|---|
| Language | Go 1.23+ | Native performance, strict typing, no ORMs |
| HTTP Router | chi router (`go-chi/chi/v5`) | Lightweight, idiomatic standard net/http compatible |
| Database | PostgreSQL 16 | NUMERIC lat/long coordinates (No PostGIS) |
| DB Access | `pgx/v5` + `sqlc` | High performance connection pool and type-safe SQL queries |
| Migrations | `goose` | Clean SQL migrations with Up and Down steps |
| Job Queue | `River` | PostgreSQL-backed transactional background queue |
| Object Storage | S3-Compatible / MinIO | Presigned upload/download URLs |
| Money Precision | `shopspring/decimal` | Exact decimal arithmetic (never float) |
| Logging | `log/slog` | Structured JSON and text logging |
| Payment Gateway | Xendit (MVP) | QRIS, Virtual Account, Card, Payouts behind `PaymentGateway` interface |

---

## Repository Layout

```
bioverse-backend/
  api/
    openapi.yaml        # OpenAPI 3.1 Contract-First specification
  cmd/
    api/                # HTTP API server entrypoint (cmd/api/main.go)
    worker/             # Background job worker entrypoint (cmd/worker/main.go)
  db/
    migrations/         # goose SQL migration files
    queries/            # sqlc SQL query definitions per domain
  deploy/
    docker-compose.yml  # Local PostgreSQL 16 & MinIO S3 cluster
  docs/
    SETUP_GUIDE.md      # Detailed developer guide & Phase 0 checklist
  internal/
    platform/           # Core platform utilities (config, database, logger, middleware, httperr)
    identity/           # User, admin, operator authentication & tokens
    gamification/       # Virtual pets, missions, boosts, streaks, points ledger
    catalog/            # Digital products, subscription plans, pricing, IAP
    conservation/       # Zones, projects, spider frames, coral fragments, incidents
    donation/           # Checkout, donations, subscriptions, webhook processing
    payment/            # PaymentGateway interface + Xendit & Midtrans adapters
    fieldops/           # Work logs, telemetry, operator payouts, media evidence
    b2b/                # Corporate accounts, contracts, invoices, quotas, ESG reports
    content/            # News, notifications, leaderboards
    jobs/               # River background job workers
  sqlc.yaml             # sqlc configuration file
  Makefile              # Developer operational commands
```

---

## Getting Started

### 1. Prerequisites
- [Go 1.23+](https://go.dev/dl/)
- [Docker Desktop](https://www.docker.com/products/docker-desktop/)
- [Goose](https://github.com/pressly/goose) (`go install github.com/pressly/goose/v3/cmd/goose@latest`)
- [SQLC](https://sqlc.dev/) (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`)

### 2. Environment Setup
Copy the environment template and customize as needed:
```bash
cp .env.example .env
```

### 3. Start Database and Object Storage
Start PostgreSQL 16 and MinIO via Docker Compose:
```bash
make up
# or: docker compose -f deploy/docker-compose.yml up -d
```

### 4. Run Database Migrations
Apply database migrations using goose:
```bash
make migrate-up
```

### 5. Start HTTP Server
```bash
make run-api
# or: go run cmd/api/main.go
```
The server will listen at `http://localhost:8080`.

Verify server health:
```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/v1/health
```

---

## API Documentation & Contract-First Development

BioVerse enforces an **OpenAPI-First** development workflow. The API contract is maintained in:
- [`api/openapi.yaml`](api/openapi.yaml)

Frontend clients (Flutter mobile app and B2B web dashboard) generate their API clients and mock servers directly from this file.

---

## Testing

Run unit and integration test suites:
```bash
make test
# or: go test -v -race ./...
```

---

## Developer Guide & Non-Code Setup Checklist

For step-by-step instructions on setting up GitHub, Xendit Sandbox, Google Cloud OAuth, Firebase FCM, and Apple/Google IAP, see:
- [Developer Setup Guide & Checklist](docs/SETUP_GUIDE.md)
