ALTER TABLE admin_idempotency
    ADD COLUMN state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'completed')),
    ADD COLUMN response_status INTEGER CHECK (response_status IS NULL OR response_status BETWEEN 200 AND 599),
    ADD COLUMN response_content_type TEXT CHECK (response_content_type IS NULL OR length(response_content_type) BETWEEN 1 AND 120),
    ADD COLUMN response_body BYTEA CHECK (response_body IS NULL OR octet_length(response_body) <= 65536),
    ADD COLUMN completed_at TIMESTAMPTZ;

ALTER TABLE admin_idempotency
    ADD CONSTRAINT admin_idempotency_response_consistency CHECK (
        (state = 'pending' AND response_status IS NULL AND response_content_type IS NULL AND response_body IS NULL AND completed_at IS NULL)
        OR (state = 'completed' AND response_status IS NOT NULL AND response_content_type IS NOT NULL AND response_body IS NOT NULL AND completed_at IS NOT NULL)
    );
