-- +goose Up
-- +goose StatementBegin
ALTER TABLE agents ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN runtime TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite prior to 3.35 cannot DROP COLUMN; we no-op here. If you need to
-- revert, recreate the agents table from a prior migration snapshot.
-- +goose StatementEnd
