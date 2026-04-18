-- +goose Up
-- +goose StatementBegin
ALTER TABLE tasks ADD COLUMN flow_task_ref TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE tasks DROP COLUMN IF EXISTS flow_task_ref;
-- +goose StatementEnd
