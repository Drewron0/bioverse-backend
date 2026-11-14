# Test Integrity and Anti-Flawed Testing Standard

## 1. Core Principle: The Test is the Specification Contract
Unit, integration, and end-to-end tests exist to guarantee that the business rules, security constraints, and financial invariants of **BioVerse** are strictly honored. 
A test is **NEVER** a rubber stamp designed to pass unconditionally.

> [!CRITICAL]
> **RULE OF TRUTH**: When a test fails against a known specification requirement, **the implementation code is wrong**. 
> You MUST fix the production code. You are strictly forbidden from weakening assertions, commenting out test expectations, or bending test conditions merely to manufacture a green `PASS`.

---

## 2. Prohibition of Flawed & Tautological Testing

AI assistants and developers are strictly forbidden from writing the following anti-patterns:

### 2.1 Tautological Assertions
- **Forbidden**: Assertions that cannot fail (e.g., asserting `err == nil` without inspecting the returned entity, or asserting `len(items) >= 0`).
- **Forbidden**: Mock tautologies where a mock is stubbed to return `X` and the test only verifies that the function returns `X` without exercising any business validation, branching, or calculations.
- **Mandatory**: Assert exact state mutations, database side effects, point ledger balance adjustments, and lifecycle state transitions.

### 2.2 Tolerance Weakening
- **Forbidden**: Changing expected values from an exact number (e.g., changing `balance == 50` to `balance >= 0`, or changing exact $3.20 price check to `price > 0`) when the real code produces wrong values.
- **Mandatory**: Use exact decimal arithmetic (`shopspring/decimal`). In BioVerse, money and points have zero tolerance for imprecise bounds.

### 2.3 Skipping Error Scenarios
- **Forbidden**: Testing only the "happy path" and ignoring failure modes.
- **Mandatory**: Every domain feature MUST test negative bounds:
  - Attempting checkout without active coral for Bio-Shield (**Coral-First Gating** must fail with HTTP 403 / `ErrCoralFirstGatingFailed`).
  - Attempting to spend more points than available balance (must fail with `ErrInsufficientPoints`).
  - Attempting duplicate submission on daily missions (must fail with HTTP 409 / conflict).
  - Attempting to allocate fragments beyond spider frame capacity (15 units).

---

## 3. Standard Protocol When a Test Fails

When `go test ./...` fails:
1. **Analyze Root Cause in Domain Code**: Read the failure message and locate the discrepancy between the expected business invariant and the actual behavior in `internal/<domain>/service.go`, query, or handler.
2. **Fix the Implementation**: Modify the production Go code to satisfy the invariant cleanly.
3. **Re-execute the Unaltered Test**: Run `go test -v ./internal/<domain>` with the original test expectations intact.
4. **Never Rewrite the Goalpost**: Only update a test file if the upstream specification ([`docs/BACKEND_SPEC.md`](file:///c:/Users/ASUS/Documents/College/Sem_5/DEPD/Project/bioverse-backend/docs/BACKEND_SPEC.md)) has officially evolved, and document the change explicitly.
