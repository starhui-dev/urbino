ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_inference_scopes_check
    CHECK (scopes <@ ARRAY['models:read', 'models:invoke', 'usage:read']::TEXT[]);
