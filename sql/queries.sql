-- name: GetRequest :one
SELECT id, tenant_id, project_id, request_id, model, protocol, status, started_at
FROM urbino.requests WHERE tenant_id = $1 AND id = $2;

-- name: CreateRequest :one
INSERT INTO urbino.requests (id, tenant_id, project_id, request_id, body_digest, model, protocol, status, dispatch_status, usage_status, started_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id, tenant_id, project_id, request_id, model, protocol, status, started_at;

-- name: CreateAttempt :one
INSERT INTO urbino.request_attempts (id, request_id, tenant_id, attempt_no, dispatch_phase, result_class, status)
VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, request_id, tenant_id, attempt_no, status;

-- name: InsertUsageEvent :one
INSERT INTO urbino.usage_events (id, tenant_id, request_id, attempt_id, source, event_key, completeness, input_total, output_total, is_estimate)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, event_key, completeness;
