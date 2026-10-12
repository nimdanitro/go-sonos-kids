# Backlog

- **Search Tidal by name via SMAPI.** Replace manual album-ID lookup with
  `search`/`browse` against the linked Tidal service. Reference:
  [steipete/sonoscli](https://github.com/steipete/sonoscli) (MIT) —
  `internal/sonos/{smapi*,music_services,didl}.go`. Copy with the MIT notice
  (its `internal/` packages can't be imported). Unverified: whether Tidal needs
  a DeviceLink/AppLink auth handshake before SMAPI search works.
- Other sonoscli ideas: scenes, queue inspection, group management,
  stream proxy for non-Sonos-friendly URLs.
- Pi deployment: systemd unit for `sonoskids serve`, Chromium kiosk unit,
  cross-compile with `GOOS=linux GOARCH=arm64`.
- Audiobook sources: Spotify `audiobook/` links aren't parsed yet; local files
  on a NAS would need a Sonos music-library ref type.
- Drag-to-reorder items and per-profile cover caching (covers are hotlinked).
- Profile PIN or per-profile token if kids get browsers on their own devices.
- Profiles' `avatar` (emoji) is no longer shown or edited anywhere: the kids'
  UI uses the first letter of the name. Drop the field from the store and API
  (needs a small migration), or repurpose it for a custom colour.
- Run the Tidal artist import against the live API: it follows the documented
  TIDAL Open API shape but has only been tested against a fake server. The
  Spotify import is likewise tested against a fake (including its 10-item page
  limit) and not yet against real credentials.
- Record audiobook progress on the server even when no screen is open (a small
  poller per profile), and show a chapter list from the per-track record.
