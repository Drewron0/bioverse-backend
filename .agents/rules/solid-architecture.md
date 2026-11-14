# BioVerse Architecture & SOLID Engineering Rules for AI Agents

All AI assistants working on this backend repository MUST adhere to these architectural, security, and SOLID design rules:

## 1. Single Responsibility Principle (SRP)
- **Handlers** (`internal/<domain>/handler.go`): ONLY parse HTTP requests, decode JSON, validate syntax, invoke services, and write RFC 7807 responses. No database queries, no business calculations.
- **Services** (`internal/<domain>/service.go`): Own domain rules, state invariants, permissions, and atomic transactions via `database.WithTx`.
- **Stores** (`internal/platform/store`): Own SQL execution generated strictly by `sqlc`. No business logic.

## 2. Open/Closed & Dependency Inversion Principles (OCP & DIP)
- External dependencies (payment gateways, object storage, push notification, email services) MUST sit behind explicit Go interfaces.
- Services must accept interfaces in constructors (`NewService(pool, querier, config)`), never instantiate global connections or concrete third-party SDKs directly.
- Adding a new payment provider (e.g., Midtrans next to Xendit) MUST only require adding a new adapter package implementing `PaymentGateway`, without modifying existing donation transaction logic.

## 3. Interface Segregation Principle (ISP)
- Do not create giant God-interfaces.
- Define narrow, consumer-driven interfaces where callers only declare what they actually use (e.g., `UserQuerier`, `TransactionExecutor`).

## 4. Defense-in-Depth Security & Edge Case Handling
- **Money Math**: ALWAYS use `shopspring/decimal.Decimal` in Go and `NUMERIC(18,2)` or `NUMERIC(18,6)` in PostgreSQL. NEVER use `float32` or `float64`.
- **Concurrency & Races**: Counters and limited quotas (e.g. 15 fragments per spider frame, sponsored brand quotas) MUST be updated under row locks (`FOR UPDATE SKIP LOCKED`) or atomic single-statement SQL updates. NEVER do read-modify-write in Go memory.
- **Cryptographic Security**:
  - Passwords: `argon2id` with constant-time verification.
  - Refresh Tokens: Stored HASHED with SHA-256 (`token_hash` unique). Raw tokens are never stored in plaintext.
  - Token Theft Detection: Replay of a revoked refresh token MUST immediately revoke the entire user token family.
  - Sensitive Columns: NIK and bank account numbers encrypted with app-level key (`BYTEA`); searchable values use HMAC (`nik_hmac`).
- **Error Sanitization**:
  - External callers receive RFC 7807 `application/problem+json` with clean, stable error codes (`BAD_REQUEST`, `UNAUTHORIZED`, `CONFLICT`, etc.).
  - Internal database errors, SQL syntax, or driver errors MUST NEVER leak to API clients; log them internally via `slog.Error`.

## 5. Duplication Philosophy (DRY vs Premature Abstraction)
- Do not merge coincidental duplication across distinct domain boundaries.
- Only merge **True Duplication** where three or more instances represent the exact same business invariant and must evolve in lockstep.
- Prefer clean, straightforward, boring Go functions over reflection, complex generics, or framework hooks.
