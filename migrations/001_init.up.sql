-- 001_init.up.sql
-- Disbursement API initial schema.
-- Production DDL; local dev also uses AutoMigrate in main.go.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Users ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL CHECK (role IN ('superadmin', 'admin', 'operator')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Disbursements -------------------------------------------------------------
CREATE TABLE IF NOT EXISTS disbursements (
    id             BIGSERIAL PRIMARY KEY,
    recipient_name TEXT        NOT NULL,
    account_number TEXT        NOT NULL,
    bank_code      TEXT        NOT NULL,
    amount         BIGINT      NOT NULL CHECK (amount >= 10000),
    admin_fee      BIGINT      NOT NULL CHECK (admin_fee >= 0),
    note           TEXT,
    status         TEXT        NOT NULL DEFAULT 'PENDING'
                   CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    created_by     BIGINT      REFERENCES users (id),
    approved_by    BIGINT      REFERENCES users (id),
    deleted_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_disbursements_status    ON disbursements (status);
CREATE INDEX IF NOT EXISTS idx_disbursements_bank_code ON disbursements (bank_code);
CREATE INDEX IF NOT EXISTS idx_disbursements_deleted   ON disbursements (deleted_at);
CREATE INDEX IF NOT EXISTS idx_disbursements_created   ON disbursements (created_at);

-- Refresh tokens (raw token never stored; only a hash) ----------------------
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id),
    token_hash TEXT        NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user  ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_valid ON refresh_tokens (expires_at);

-- Idempotency keys ----------------------------------------------------------
CREATE TABLE IF NOT EXISTS idempotency_keys (
    id              BIGSERIAL PRIMARY KEY,
    idempotency_key UUID        NOT NULL UNIQUE,
    disbursement_id BIGINT      REFERENCES disbursements (id),
    status          TEXT        NOT NULL DEFAULT 'in_progress',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Audit logs -----------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    log_ref    TEXT        NOT NULL UNIQUE,      -- LOG-001
    entity_ref TEXT        NOT NULL,             -- DSB-001
    action     TEXT        NOT NULL CHECK (action IN ('created', 'status_changed', 'deleted')),
    actor      TEXT        NOT NULL,
    before     JSONB       NOT NULL DEFAULT '{}',
    after      JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_entity ON audit_logs (entity_ref);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs (action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs (created_at);

-- Seed users (bcrypt hashes generated at runtime by the seed routine) --------
-- Username / password:
--   superadmin / superadmin123
--   admin      / admin123
--   operator   / operator123
