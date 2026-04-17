-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN runtime TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE agents DROP COLUMN IF EXISTS model;
ALTER TABLE agents DROP COLUMN IF EXISTS runtime;
-- +goose StatementEnd
