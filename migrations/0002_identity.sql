-- Urbino phase 03 identity and authorization boundary.
-- Secrets are never stored; only keyed digests and non-sensitive metadata live here.
CREATE TABLE project_members (
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('owner', 'member', 'viewer')),
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, project_id, user_id),
    CONSTRAINT project_members_project_fk FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id),
    CONSTRAINT project_members_user_fk FOREIGN KEY (tenant_id, user_id) REFERENCES users(tenant_id, id)
);

CREATE TABLE api_keys (
    tenant_id UUID NOT NULL,
    id UUID NOT NULL,
    project_id UUID NOT NULL,
    user_id UUID NOT NULL,
    public_id TEXT NOT NULL CHECK (length(public_id) BETWEEN 16 AND 200),
    secret_digest BYTEA NOT NULL CHECK (octet_length(secret_digest) = 32),
    digest_key_id TEXT NOT NULL CHECK (length(digest_key_id) BETWEEN 1 AND 100),
    scopes TEXT[] NOT NULL CHECK (cardinality(scopes) > 0),
    allowed_models TEXT[] NOT NULL CHECK (cardinality(allowed_models) > 0),
    status TEXT NOT NULL CHECK (status IN ('active', 'revoked', 'expired')),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    auth_version BIGINT NOT NULL CHECK (auth_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT api_keys_public_id_unique UNIQUE (public_id),
    CONSTRAINT api_keys_project_fk FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id),
    CONSTRAINT api_keys_user_fk FOREIGN KEY (tenant_id, user_id) REFERENCES users(tenant_id, id),
    CONSTRAINT api_keys_member_fk FOREIGN KEY (tenant_id, project_id, user_id) REFERENCES project_members(tenant_id, project_id, user_id),
    CONSTRAINT api_keys_revocation_consistency CHECK ((status = 'revoked') = (revoked_at IS NOT NULL))
);

CREATE OR REPLACE FUNCTION urbino_require_active_project_member() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM project_members
        WHERE tenant_id = NEW.tenant_id AND project_id = NEW.project_id
          AND user_id = NEW.user_id AND status = 'active'
    ) THEN
        RAISE EXCEPTION 'project membership required';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER api_keys_require_active_member
BEFORE INSERT OR UPDATE OF tenant_id, project_id, user_id ON api_keys
FOR EACH ROW EXECUTE FUNCTION urbino_require_active_project_member();

CREATE INDEX api_keys_lookup_idx ON api_keys (public_id) WHERE status = 'active';
CREATE INDEX api_keys_scope_idx ON api_keys (tenant_id, project_id, user_id);

CREATE TABLE admin_principals (
    id UUID PRIMARY KEY,
    display_name TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
    scopes TEXT[] NOT NULL CHECK (cardinality(scopes) > 0),
    version BIGINT NOT NULL CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_tokens (
    id UUID PRIMARY KEY,
    principal_id UUID NOT NULL REFERENCES admin_principals(id),
    public_id TEXT NOT NULL CHECK (length(public_id) BETWEEN 16 AND 200),
    secret_digest BYTEA NOT NULL CHECK (octet_length(secret_digest) = 32),
    digest_key_id TEXT NOT NULL CHECK (length(digest_key_id) BETWEEN 1 AND 100),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    auth_version BIGINT NOT NULL CHECK (auth_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT admin_tokens_public_id_unique UNIQUE (public_id),
    CONSTRAINT admin_tokens_revocation_consistency CHECK ((revoked_at IS NOT NULL) OR expires_at > created_at)
);

CREATE INDEX admin_tokens_lookup_idx ON admin_tokens (public_id) WHERE revoked_at IS NULL;

CREATE TABLE audit_events (
    id UUID PRIMARY KEY,
    tenant_id UUID,
    actor_kind TEXT NOT NULL CHECK (actor_kind IN ('admin', 'api_key', 'system')),
    actor_id UUID,
    action TEXT NOT NULL CHECK (length(action) BETWEEN 1 AND 120),
    target_type TEXT NOT NULL CHECK (length(target_type) BETWEEN 1 AND 120),
    target_id UUID,
    reason TEXT,
    result TEXT NOT NULL CHECK (result IN ('success', 'denied', 'failure')),
    safe_metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(safe_metadata) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audit_events_tenant_fk FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);

CREATE INDEX audit_events_scope_idx ON audit_events (tenant_id, created_at DESC);
