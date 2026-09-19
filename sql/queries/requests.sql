-- name: GetRequest :one
SELECT tenant_id, id, project_id, user_id, idempotency_digest, body_digest,
       model, protocol, status, dispatch_status, usage_status,
       price_version_id, started_at, finished_at, created_at
FROM requests
WHERE tenant_id = $1 AND id = $2;

-- name: GetAttempt :one
SELECT tenant_id, id, request_id, attempt_no, account_id, credential_version,
       egress_version, upstream_request_id, dispatch_phase, result_class, created_at
FROM request_attempts
WHERE tenant_id = $1 AND request_id = $2 AND attempt_no = $3;

-- name: FindSettlementByBusinessKey :one
SELECT tenant_id, id, request_id, business_key, price_version_id, amount_micros,
       status, created_at
FROM settlements
WHERE tenant_id = $1 AND business_key = $2;
