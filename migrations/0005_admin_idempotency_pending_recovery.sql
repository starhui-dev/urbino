UPDATE admin_idempotency
SET state = 'completed',
    response_status = 409,
    response_content_type = 'application/json',
    response_body = convert_to('{"code":"idempotency_outcome_unknown","message":"the previous management mutation outcome is unavailable; do not retry with this key","request_id":"00000000-0000-0000-0000-000000000000","retryable":false,"details":{}}', 'UTF8'),
    completed_at = now()
WHERE state = 'pending';
