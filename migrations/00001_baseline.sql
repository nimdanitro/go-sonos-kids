-- +goose Up

-- The original schema. IF NOT EXISTS, because databases created before
-- migrations were tracked already have these tables (see 00002 and 00003 for
-- how they are brought up to date).
CREATE TABLE IF NOT EXISTS profiles (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    speaker    TEXT NOT NULL,
    max_volume INTEGER NOT NULL DEFAULT 40,
    avatar     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_id INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    category   TEXT NOT NULL CHECK (category IN ('music', 'audiobook')),
    title      TEXT NOT NULL,
    cover_url  TEXT NOT NULL DEFAULT '',
    service    TEXT NOT NULL,
    kind       TEXT NOT NULL,
    ref_id     TEXT NOT NULL,
    UNIQUE (profile_id, category, service, kind, ref_id)
);

-- +goose Down
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS profiles;
