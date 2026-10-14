-- Per-minute request units, never refunded after send or unknown outcome.
CREATE TABLE regional_rpm_budgets (
 account_id BIGINT NOT NULL, minute BIGINT NOT NULL,
 primary_used INTEGER NOT NULL DEFAULT 0 CHECK(primary_used>=0),
 granted INTEGER NOT NULL DEFAULT 0 CHECK(granted>=0),
 PRIMARY KEY(account_id,minute)
);
CREATE TABLE regional_rpm_grants (
 grant_id VARCHAR(128) PRIMARY KEY, region_id VARCHAR(63) NOT NULL,
 account_id BIGINT NOT NULL, minute BIGINT NOT NULL,
 units INTEGER NOT NULL CHECK(units>0),
 FOREIGN KEY(account_id,minute) REFERENCES regional_rpm_budgets(account_id,minute)
);
CREATE TABLE regional_local_rpm_grants (
 grant_id VARCHAR(128) PRIMARY KEY, region_id VARCHAR(63) NOT NULL,
 account_id BIGINT NOT NULL, minute BIGINT NOT NULL,
 units INTEGER NOT NULL CHECK(units>0),
 used INTEGER NOT NULL DEFAULT 0 CHECK(used>=0 AND used<=units)
);
CREATE TABLE regional_account_state_events (
 event_id VARCHAR(128) PRIMARY KEY, region_id VARCHAR(63) NOT NULL,
 account_id BIGINT NOT NULL, payload JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 acknowledged BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE regional_account_states (
 account_id BIGINT PRIMARY KEY,
 revision BIGINT NOT NULL DEFAULT 0,
 cooldown_until TIMESTAMPTZ NOT NULL DEFAULT '-infinity',
 invalid BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE regional_local_account_states (
 account_id BIGINT PRIMARY KEY,
 revision BIGINT NOT NULL,
 cooldown_until TIMESTAMPTZ NOT NULL,
 invalid BOOLEAN NOT NULL,
 valid_until TIMESTAMPTZ NOT NULL, clock_valid_until TIMESTAMPTZ
);
CREATE TABLE regional_oauth_refresh_fences (
 account_id BIGINT PRIMARY KEY,
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 state TEXT NOT NULL CHECK(state IN ('started','completed'))
);
CREATE TABLE regional_rpm_operations (
 operation_id VARCHAR(128) PRIMARY KEY,
 payload JSONB NOT NULL,
 installed BOOLEAN NOT NULL DEFAULT false
);
