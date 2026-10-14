-- Immutable content-addressed pricing. Application roles must not UPDATE or
-- DELETE published prices while requests/outbox/settlements refer to them.
CREATE TABLE IF NOT EXISTS regional_prices (
 version VARCHAR(64) PRIMARY KEY CHECK (version ~ '^[0-9a-f]{64}$'),
 price JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE FUNCTION reject_regional_price_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'regional pricing is immutable; publish a new version';
END;
$$;
CREATE TRIGGER regional_prices_immutable BEFORE UPDATE OR DELETE ON regional_prices
 FOR EACH ROW EXECUTE FUNCTION reject_regional_price_mutation();

-- Main-site in-flight money is deducted from the SAME spendable user row as
-- regional grants. Identity and unresolved holds survive process restart.
CREATE TABLE IF NOT EXISTS regional_primary_requests (
 request_id VARCHAR(63) PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id),
 request JSONB NOT NULL,
 maximum BIGINT NOT NULL CHECK (maximum > 0 AND maximum % 10 = 0),
 state TEXT NOT NULL CHECK (state IN ('reserved','started','completed','canceled')),
 event JSONB
);
