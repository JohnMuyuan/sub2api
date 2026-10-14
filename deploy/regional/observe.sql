-- Read-only, aggregate diagnostics; never publish request/event JSON or credentials.
-- Run against each region database. Missing schema is an error, not readiness.
BEGIN READ ONLY;
SET LOCAL statement_timeout = '5s';
SELECT state, count(*) AS requests FROM regional_local_requests GROUP BY state ORDER BY state;
SELECT count(*) AS unacknowledged_usage FROM regional_usage_outbox WHERE NOT acknowledged;
SELECT state, count(*) AS leases, sum(granted) AS granted_nano_usd,
       sum(spent) AS spent_nano_usd, sum(reserved) AS reserved_nano_usd
FROM regional_local_leases GROUP BY state ORDER BY state;
SELECT state, count(*) AS operations FROM regional_broker_operations GROUP BY state ORDER BY state;
SELECT kind, state, count(*) AS grants, sum(slots) AS slots, sum(used) AS occupied
FROM regional_local_slot_grants GROUP BY kind,state ORDER BY kind,state;
SELECT count(*) AS expired_occupied_grants FROM regional_local_slot_grants
WHERE used > 0 AND (grant_payload->>'expires_at')::timestamptz <= clock_timestamp();
SELECT state, count(*) AS queue_entries FROM regional_key_queue GROUP BY state ORDER BY state;
SELECT state, count(*) AS live_handoffs FROM regional_live_handoffs GROUP BY state ORDER BY state;
COMMIT;
