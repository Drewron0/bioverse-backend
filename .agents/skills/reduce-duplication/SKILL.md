---
name: reduce-duplication
description: Finds duplicated logic, separates true duplication from code that only looks alike, and merges only true duplicates behind one well-named function. Use when one fix keeps landing in many places.
license: CC0-1.0
metadata:
  version: 1.0.1
  kind: prompt
  category: refactoring
---

# Reduce Code Duplication & Enforce SOLID Principles

## Inputs
- [SCOPE] (required): The directory, module, or set of files to examine.

<context>
Duplication hurts when the copies represent the exact same business rule and must change together.
However, code that only looks alike syntactically but belongs to different domain concepts is coincidental duplication.
Merging coincidental code creates premature abstractions full of boolean flags that couple unrelated domains.
The wrong abstraction costs far more than harmless syntactic similarity.
</context>

<task>
1. Identify candidate duplicates across services, handlers, and stores.
2. Classify each candidate:
   - **True Duplication**: Copies represent the identical business invariant and must evolve together.
   - **Coincidental**: Copies happen to look similar today (e.g., similar struct fields or validation shape) but belong to separate domains that will diverge.
3. Merge only true duplication where at least 3 instances exist, or where drift has already introduced bugs.
4. Enforce Go SOLID Principles during any refactoring:
   - **S (Single Responsibility)**: Transport logic in handlers, business invariants in services, queries in sqlc stores.
   - **O (Open/Closed)**: Extend functionality through interfaces (e.g., `PaymentGateway`) rather than switch-cases on concrete providers.
   - **L (Liskov Substitution)**: Adapters and test mocks must satisfy interfaces without altering calling contract invariants.
   - **I (Interface Segregation)**: Keep Go interfaces small and consumer-driven (prefer 1-3 methods per interface).
   - **D (Dependency Inversion)**: Services accept interfaces in constructor functions (`NewService(pool, store, cfg)`).
5. Always run test suites after any merge or refactoring.
</task>

<constraints>
- Never merge coincidental duplicates across different domain boundaries (e.g. do not merge user token revocation with operator token revocation if their lifecycles diverge).
- Zero behaviour changes during deduplication.
- Prefer explicit plain functions over reflection, type assertions, or generic wrappers.
- Do not add boolean parameters to serve differing callers; use distinct functions instead.
</constraints>
