# go-sonos-kids

A touch-friendly music player for kids on Sonos speakers. Each kid has a
profile mapped to their own speaker and a curated library of Tidal/Spotify
albums, playlists and audiobooks (no searching). Single Go binary with the
React UI embedded.

## Run

```sh
go generate ./ui                      # install deps + build the UI into ui/build
go build -o sonoskids .
SONOSKIDS_ADMIN_TOKEN=secret ./sonoskids serve --addr :8080
```

- Kids' UI: `http://<host>:8080/`
- Hidden admin (add albums/playlists/audiobooks, manage profiles and their
  speakers, rename/move/remove items): `/admin`, a route of the same React app.
  It is not linked anywhere, is served with `X-Robots-Tag: noindex`, and every
  API call needs the admin token. Without a token the admin API is disabled.
- Database: `~/.config/sonoskids/sonoskids.db` (SQLite, `--db` to change).

## Configuration

Every command is configured with flags, environment variables or a YAML config
file, merged by viper (precedence in that order, then defaults). The file is
`--config <file>`, else `./sonoskids.yaml`, else
`~/.config/sonoskids/config.yaml`. A complete sample (also in
`sonoskids.example.yaml`):

```yaml
# sonoskids.yaml
# Precedence: command-line flag > environment variable > this file > default.

# --- `sonoskids serve` ---------------------------------------------------
addr: 127.0.0.1:8080        # use 0.0.0.0:8080 to reach it from other devices
# db: /var/lib/sonoskids/sonoskids.db   # default: <user config dir>/sonoskids/sonoskids.db
# migrate_on_startup: true  # false = run `sonoskids migrate up` yourself; serve then refuses an outdated database
admin_token: change-me      # protects /admin; the admin API is off while empty
country: CH                 # artist import lists only releases available there

# Artist import ("add every album of an artist"). Both are optional; each
# service is enabled once it has a client ID and secret.
spotify:                    # https://developer.spotify.com/dashboard  (create an app)
  client_id: ""
  client_secret: ""
tidal:                      # https://developer.tidal.com  (create an app)
  client_id: ""
  client_secret: ""

# --- speaker commands (play, favorites, speakers) ------------------------
# speaker: Globi           # default room/group/IP, so you can leave out -s
# discover_timeout: 3s      # how long to wait for speakers to answer
# no_cache: false           # true = always rediscover via SSDP
```

Copy it, fill in what you need, and keep it private with `chmod 600`: the
server warns when a file holding secrets is readable by others, and secrets are
never logged.

Each key has a flag and an environment variable. A key is the flag name with
dashes as underscores, and the variable is `SONOSKIDS_` plus the key in upper
case:

| Config key | Flag | Environment variable |
|---|---|---|
| `addr` | `--addr` | `SONOSKIDS_ADDR` |
| `db` | `--db` | `SONOSKIDS_DB` |
| `admin_token` | `--admin-token` | `SONOSKIDS_ADMIN_TOKEN` |
| `country` | `--country` | `SONOSKIDS_COUNTRY` |
| `migrate_on_startup` | `--migrate-on-startup` | `SONOSKIDS_MIGRATE_ON_STARTUP` |
| `spotify.client_id` | `--spotify-client-id` | `SONOSKIDS_SPOTIFY_CLIENT_ID` or `SPOTIFY_CLIENT_ID` |
| `spotify.client_secret` | `--spotify-client-secret` | `SONOSKIDS_SPOTIFY_CLIENT_SECRET` or `SPOTIFY_CLIENT_SECRET` |
| `tidal.client_id` | `--tidal-client-id` | `SONOSKIDS_TIDAL_CLIENT_ID` or `TIDAL_CLIENT_ID` |
| `tidal.client_secret` | `--tidal-client-secret` | `SONOSKIDS_TIDAL_CLIENT_SECRET` or `TIDAL_CLIENT_SECRET` |
| `speaker` | `-s`, `--speaker` | `SONOSKIDS_SPEAKER` |
| `discover_timeout` | `--discover-timeout` | `SONOSKIDS_DISCOVER_TIMEOUT` |
| `no_cache` | `--no-cache` | `SONOSKIDS_NO_CACHE` |
| `config` | `--config` | `SONOSKIDS_CONFIG` |

### Importing an artist's whole discography

In `/admin`, paste an artist link (for example
`https://open.spotify.com/artist/<id>/discography/all`), choose the category and
the kids, and press "Diskografie laden". All releases are listed oldest first;
deselect what you don't want and add the rest. Releases are then shown to the
kids grouped by artist (music) or by release date, oldest first (audiobooks).

Listing a whole catalogue needs the services' official APIs, because their
public pages only show an artist's top tracks. Create a free app and put its
client ID and secret into the config:

- Spotify: <https://developer.spotify.com/dashboard> (no redirect URI is needed).
- Tidal: <https://developer.tidal.com>. The Tidal import follows the documented
  API but has not been run against the live service yet.

Single links (albums, playlists, tracks) need no credentials; their artist and
release date are read from the public page.

Spotify limits newer apps to 10 items per page. The import asks for 50, and when
Spotify answers with a 400 it switches to 10 for good and pages through the
whole catalogue.

### Resuming audiobooks

Sonos does not remember where you were in an album, so the server does. While
an audiobook plays, each status reading the player screen asks for (every
couple of seconds) is saved per kid as JSON in the item's `progress` column:
the track last heard, the position in it, and a record per track (title,
duration, position, done). Tapping an unfinished audiobook again starts the
speaker at that track, 5 seconds before where it stopped. A finished one starts
over. On the player screen the "Von vorn" button (top right) starts what is
playing over from the beginning, after a big yes/no question so a small finger
cannot erase a place by accident; the API equivalent is `POST
/api/profiles/{id}/play` with `"restart": true`. The cover on that screen shows
a progress bar through the whole album or audiobook for anything with tracks.
"Als ungehört markieren" in `/admin` forgets the progress. Only audiobooks are
tracked, never music. Progress is only recorded while the player screen is
open, and seeking inside a track is best effort: if the speaker refuses, the
track simply starts from its beginning.

### Filling in missing artists and dates

Music is grouped by artist, so every album and track needs one. Entries saved
without an artist or release date (for example before the library recorded
them) are looked up and filled in **every time the server starts** (see the
`metadata backfill` line in the log), and on demand with the "Fehlende Künstler
und Daten ergänzen" button in `/admin`. Anything already filled in, by hand or
otherwise, is never overwritten. What the services do not know stays empty,
shows up as "Künstler fehlt" in `/admin`, and can be edited by hand.

## Database migrations

The schema is managed with [goose](https://github.com/pressly/goose), the same
way as sitrep: numbered migrations in `migrations/`, embedded in the binary.
`serve` applies pending migrations by itself on start (`migrate_on_startup`,
on by default and logged one line per migration). To do it as a separate step,
set `--migrate-on-startup=false`; the server then refuses to start on an
outdated database and tells you to run:

```sh
sonoskids migrate status   # which migrations are applied
sonoskids migrate up       # apply pending ones
sonoskids migrate down     # roll back the last one (this can lose data)
```

Databases created before migrations were tracked are adopted in place: the
migrations check what already exists, so nothing is lost.

To add a migration, create the next numbered file in `migrations/`, for
example `00004_something.sql`:

```sql
-- +goose Up
ALTER TABLE items ADD COLUMN notes TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE items DROP COLUMN notes;
```

A change that needs logic is written in Go and registered in
`GoMigrations()` (see `00002_*.go`). `migrations/migrations_test.go` runs every
migration up, down and up again.

## Develop

```sh
go run . serve --admin-token secret   # API on 127.0.0.1:8080
cd ui && yarn dev                     # Vite dev server on :5173 (also /admin), proxies /api
```

UI checks (same oxlint/oxfmt setup as sitrep): `yarn lint`, `yarn fmt:check`,
`yarn typecheck`. Go: `go vet ./... && go test ./...`.

## CLI

`sonoskids speakers | play <url> | favorites [rename]` talk to the speakers
directly. See `--help`. Planned work is in `BACKLOG.md`.

## License

MIT, see [LICENSE](LICENSE).
