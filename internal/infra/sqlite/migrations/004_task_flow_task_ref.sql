-- +goose Up
-- +goose StatementBegin
ALTER TABLE tasks ADD COLUMN flow_task_ref TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite cannot DROP COLUMN on older versions; no-op.
-- +goose StatementEnd
