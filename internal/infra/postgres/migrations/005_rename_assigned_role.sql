-- +goose Up
-- Rename current_role → assigned_role for consistency with the SQLite
-- backend. current_role is a PostgreSQL reserved keyword; this migration
-- is a no-op for fresh PG installs (003 already uses assigned_role) but
-- is required for any PG instance that ran the old 003 before the fix.
-- Use EXECUTE to prevent the parser from seeing the reserved word.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'agents' AND column_name = 'current_role'
    ) THEN
        EXECUTE 'ALTER TABLE agents RENAME COLUMN current_role TO assigned_role';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'agents' AND column_name = 'assigned_role'
    ) THEN
        EXECUTE 'ALTER TABLE agents RENAME COLUMN assigned_role TO current_role';
    END IF;
END $$;
-- +goose StatementEnd
