# simple-disbursement-service-example

Disbursement API service built as the take-home backend test for the Backend Engineer role.

Layered Go application (`handler → service → repository → model`) exposing a JWT-protected disbursement API with idempotency, concurrency-safe status transitions, soft delete, and a non-blocking audit log.

## Tech Stack

| Concern     | Choice       |
| ----------- | ------------ |
| Language    | Go           |
| Web         | Gin          |
| Database    | PostgreSQL   |
| ORM/query   | GORM         |
| Migrations  | —            |
| Auth        | JWT          |

## Project Layout

```
.
├── cmd/api/            # entrypoint
├── internal/
│   ├── handler/        # HTTP layer: parse/validate, no business logic
│   ├── service/        # business rules
│   ├── repository/     # data access
│   ├── model/          # structs
│   ├── middleware/     # JWT, request-id, idempotency, rate limit
│   └── config/         # env config
├── migrations/         # schema migrations
├── ARCHITECTURE.md     # Part 1: design decisions
└── README.md
```

## Prerequisites

- Go 1.2x
- Docker + Docker Compose (PostgreSQL)
- `make` (optional)

## Setup

1. Copy env defaults: `cp .env.example .env`
2. Start database: `docker compose up -d db`
3. Run migrations: `make migrate`
4. Start API: `make run` (default `:8080`)
5. Run tests: `make test`

## Configuration

All config via environment variables. See `.env.example`.

| Variable          | Default        | Description        |
| ----------------- | -------------- | ------------------ |
| `APP_PORT`        | `8080`         | HTTP listen port   |
| `DATABASE_URL`    | —              | PostgreSQL DSN     |
| `JWT_SECRET`      | —              | HMAC signing key   |
| `ACCESS_TTL`      | `15m`          | Access token TTL   |
| `REFRESH_TTL`     | `168h` (7d)    | Refresh token TTL  |
| `IDEMPOTENCY_TTL` | `24h`          | Idempotency cache  |

## Endpoints

All endpoints except `/auth/*` require `Authorization: Bearer <access_token>`.

| Method | Path                            | Role        | Description                        |
| ------ | ------------------------------- | ----------- | ---------------------------------- |
| GET    | `/health`                       | public      | DB status; 503 if unreachable      |
| POST   | `/auth/login`                   | public      | Exchange credentials for tokens    |
| POST   | `/auth/refresh`                 | public      | Swap refresh token for new access  |
| POST   | `/auth/logout`                  | auth        | Invalidate refresh token           |
| GET    | `/disbursements`                | operator+   | Paginated, filterable, sortable    |
| GET    | `/disbursements/:id`            | operator+   | Single disbursement                |
| POST   | `/disbursements`                | operator+   | Create (supports `Idempotency-Key`)|
| PATCH  | `/disbursements/:id/status`     | admin+      | Concurrency-safe status transition |
| DELETE | `/disbursements/:id`            | superadmin  | Soft delete (PENDING only)         |
| GET    | `/audit-logs`                   | superadmin  | Filterable, paginated              |

### Idempotency

`POST /disbursements` accepts an `Idempotency-Key: <uuid-v4>` header.

- Same key within 24h → identical cached response, `X-Idempotent-Replayed: true`
- No key → normal processing, no idempotency guarantee
- See `ARCHITECTURE.md` §1.1 for storage design

### Status Transitions

`PENDING → APPROVED | REJECTED` (terminal). Approved/rejected rows cannot be re-transitioned. Concurrency-safe — see `ARCHITECTURE.md` §1.2.

## Response Format

Consistent envelope across all endpoints:

```json
{
  "success": true,
  "data": [],
  "meta": { "page": 1, "limit": 20, "total": 284, "total_pages": 15 }
}
```

Errors: `{ "success": false, "error": { "code": "...", "message": "..." } }` with appropriate HTTP status.

## Database Schema

| Table            | Notes                                                        |
| ---------------- | ------------------------------------------------------------ |
| `users`          | `id`, `username`, `password_hash`, `role`                    |
| `disbursements`  | status, `admin_fee`, `approved_by` (nullable), `deleted_at` (soft delete), indexes |
| `refresh_tokens` | token, user, expires_at, revoked                             |
| `idempotency`    | key, response snapshot, created_at (TTL)                     |
| `audit_logs`     | `LOG-###`, `entity_id` `DSB-###`, action, actor, before/after, created_at |

Full DDL in `migrations/`.

## Audit Logging

Every create / status change / delete writes a row to `audit_logs` in a separate table. Writes are non-blocking: if the audit write fails, the disbursement operation still succeeds and the failure is logged server-side.

## Structured Logging

Every request produces one JSON log line with a propagated `request_id`:

```json
{
  "level": "info",
  "timestamp": "2025-06-12T08:00:00Z",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "method": "PATCH",
  "path": "/disbursements/DSB-001/status",
  "status_code": 200,
  "latency_ms": 42,
  "user": "admin"
}
```

Response includes `X-Request-ID`.

## Testing

Unit tests cover the critical business logic:

- `admin_fee` calculation boundary (≥ 5,000,000 threshold)
- Status transition validation (terminal-state, role enforcement)
- Idempotency handler (replay vs. first request)

## Seed Users

| Username     | Password       | Role       |
| ------------ | -------------- | ---------- |
| `superadmin` | `superadmin123`| superadmin |
| `admin`      | `admin123`     | admin      |
| `operator`   | `operator123`  | operator   |

## Technical Decisions

Consolidated in [`ARCHITECTURE.md`](ARCHITECTURE.md):

1. Idempotency mechanism and where idempotency state lives
2. Concurrency/locking strategy for concurrent approval and its trade-offs
3. Refresh token storage (DB vs. in-memory)
4. Library choices and rationale

## License

— (none)
