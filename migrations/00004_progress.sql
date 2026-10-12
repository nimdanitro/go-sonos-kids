-- +goose Up

-- How far a kid got in an item, as JSON (see store.Progress): the track last
-- heard, the position in it, and a record per track. Empty if never played.
ALTER TABLE items ADD COLUMN progress TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE items DROP COLUMN progress;
