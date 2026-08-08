# Architecture & Design Decisions

Part 1 of the take-home test. Each answer is 150–250 words. These decisions are
implemented consistently across the codebase.

---

## 1.1 Idempotency

**Question:** How do you ensure `POST /disbursements` does not create duplicates if a client sends the same request twice (e.g. timeout then retry)? Explain the mechanism and where you store the idempotency state.

**Answer:**

`POST /disbursements` accepts an `Idempotency-Key: <uuid-v4>` header. The key is the client's retry token: the same logical request must reuse the same UUID, so a duplicate retry is provably the same operation rather than a legitimate new disbursement.

The mechanism is a unique constraint on the idempotency key in its own dedicated table (`idempotency_keys`). Flow:

1. On request, the handler reads `Idempotency-Key`. If absent, the request proceeds normally with no guarantee.
2. The service attempts an `INSERT` of `(key, status='in_progress')`. If a row already exists, we have a replay: load the stored response snapshot and return it verbatim with `X-Idempotent-Replayed: true`, without executing any side effect.
3. On success, the computed response is stored against the key and the row flips to `committed`; the next request with the same key returns the identical snapshot.

Why a DB table over an in-memory cache: the service may run multiple instances, and a process-local map would not dedupe across replicas or survive restarts. The unique constraint is the single source of truth and is inherently concurrency-safe — two racing requests with the same key collide on the `INSERT`, so exactly one proceeds.

Each row carries `created_at`, and keys expire after 24h to bound table growth. The response is stored as a JSON snapshot so replay returns byte-identical payload and status code. This satisfies the requirement that a replayed request has no side effects and returns the identical response.

**Trade-offs:** storing full response snapshots costs storage per unique request (bounded by the 24h TTL and a background cleanup job). The alternative — storing only a hash and recomputing — risks divergent responses on non-deterministic fields (e.g. `created_at`, timestamps), so the snapshot approach is preferred.

---

## 1.2 Concurrency & Locking

**Question:** Two admins click "Approve" on the same disbursement simultaneously. How does the system prevent a race condition? Explain the approach (optimistic, pessimistic, or other) and its trade-offs.

**Answer:**

The status transition uses **optimistic locking via a conditional `UPDATE`** on a version/status guard, executed atomically in the database.

The transition is a single statement of the form:

```sql
UPDATE disbursements
SET    status = 'APPROVED', approved_by = $actor, updated_at = now()
WHERE  id = $id
  AND  status IN ('PENDING')
```

PostgreSQL guarantees the row lock is taken on the matching row, and the `WHERE status IN ('PENDING')` guard means only one concurrent transaction can satisfy the predicate. If two admins fire simultaneously, the first `UPDATE` claims the row; the second matches zero rows (status is no longer `PENDING`) and the affected-row count is 0, which the service interprets as "already transitioned" and returns a clear 409/422 error. Only one approval ever succeeds.

Why optimistic over pessimistic: a full `SELECT ... FOR UPDATE` + explicit check would hold a row lock for the whole request (including any application-side work and audit-log writes), increasing lock contention and risk of deadlock under load. The conditional `UPDATE` is one round-trip, holds the lock only for the duration of the write, and pushes the invariant into the database where it cannot be violated by application bugs. A unique/terminal-state is further reinforced at the model layer with a check constraint that `APPROVED`/`REJECTED` rows cannot change.

**Trade-offs:** optimistic locking assumes contention is rare — if two clients genuinely race the same row often, one always "loses" and must retry or surface an error. For an approval workflow, that failure is the desired behavior (a disbursement is approved exactly once), so the cost is acceptable. The alternative pessimistic `FOR UPDATE` serializes all transitions but trades throughput and lock-hold time for a guaranteed-sequential outcome.

---

## Supplementary Decisions

### Refresh Token Storage

Refresh tokens are stored in a `refresh_tokens` table (token hash, `user_id`, `expires_at`, `revoked_at`, and a `revoked` flag) rather than an opaque in-memory store.

**Rationale:** a DB-backed store makes logout immediately enforceable (`POST /auth/logout` marks the token revoked), survives process restarts, supports multi-instance deployments, and allows the 7-day expiry to be enforced at query time. The stored value is a hash (not the raw token), so a leaked database does not expose usable credentials. Trade-off: one extra indexed lookup per refresh, which is negligible versus the security and revocation guarantees gained.

### Key Schema Decisions

- **Soft delete**: `disbursements` carries a nullable `deleted_at`; `DELETE` (superadmin, PENDING-only) sets the column rather than removing the row. All list/get queries filter `deleted_at IS NULL`.
- **Audit log isolation**: `audit_logs` is written to in a separate, best-effort path. If the audit write fails, the disbursement operation still succeeds and the failure is logged server-side — audit never blocks the primary operation.
- **Structured logging**: `zerolog` emits one JSON line per request with a `request_id` generated once per request and propagated through handler → service → repository via a context-scoped logger; responses carry `X-Request-ID`.

### Library Choices

| Choice   | Why                                                                 |
| -------- | ------------------------------------------------------------------- |
| Go       | Statically typed, first-class concurrency for the race-condition tests, single static binary. |
| Gin      | Mature router/middleware ecosystem, request-scoped context for propagating `request_id`. |
| GORM     | Migration support and struct mapping; kept thin so repository methods hold explicit SQL/`UPDATE` guards where correctness matters. |
| PostgreSQL | Required for the concurrency test — row-level locking and unique constraints are the correctness mechanism. |
| zerolog  | Zero-allocation structured JSON logging for the per-request log requirement. |
