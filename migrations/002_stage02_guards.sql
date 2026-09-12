-- +goose Up
ALTER TABLE urbino.tenants ADD CONSTRAINT tenants_id_currency_key UNIQUE (id,currency);
ALTER TABLE urbino.billing_accounts ADD CONSTRAINT billing_accounts_tenant_currency_fkey FOREIGN KEY (tenant_id,currency) REFERENCES urbino.tenants(id,currency);
ALTER TABLE urbino.journal_transactions ADD CONSTRAINT journal_transactions_tenant_currency_fkey FOREIGN KEY (tenant_id,currency) REFERENCES urbino.tenants(id,currency);
ALTER TABLE urbino.price_versions ALTER COLUMN published_at DROP NOT NULL;
ALTER TABLE urbino.price_versions ALTER COLUMN immutable SET DEFAULT false;
UPDATE urbino.schema_version SET version = 2 WHERE singleton = TRUE;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION urbino.reject_price_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    version_ids uuid[];
    version_row record;
BEGIN
    IF TG_TABLE_NAME = 'price_versions' THEN
        IF OLD.published_at IS NOT NULL OR OLD.immutable THEN
            RAISE EXCEPTION 'published price rows are immutable';
        END IF;
    ELSE
        IF TG_OP = 'INSERT' THEN
            version_ids := ARRAY[NEW.price_version_id];
        ELSIF TG_OP = 'DELETE' THEN
            version_ids := ARRAY[OLD.price_version_id];
        ELSE
            version_ids := ARRAY[OLD.price_version_id, NEW.price_version_id];
        END IF;
        -- 与发布 UPDATE 互斥；跨版本移动按固定顺序锁定父版本。
        FOR version_row IN
            SELECT id, published_at, immutable FROM urbino.price_versions
            WHERE id = ANY(version_ids) ORDER BY id FOR UPDATE
        LOOP
            IF version_row.published_at IS NOT NULL OR version_row.immutable THEN
                RAISE EXCEPTION 'published price rows are immutable';
            END IF;
        END LOOP;
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS price_items_immutable ON urbino.price_items;
CREATE TRIGGER price_items_immutable BEFORE INSERT OR UPDATE OR DELETE ON urbino.price_items FOR EACH ROW EXECUTE FUNCTION urbino.reject_price_mutation();
-- +goose Down
-- Intentionally unsupported: use an application-first compatible rollback.
