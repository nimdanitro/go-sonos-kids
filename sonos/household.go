package sonos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DiscoverOptions controls DiscoverGroups.
type DiscoverOptions struct {
	Timeout time.Duration                    // SSDP wait time (default 3s)
	NoCache bool                             // ignore the cached speaker list
	Logf    func(format string, args ...any) // optional progress messages
}

func (o DiscoverOptions) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// DiscoverGroups resolves the household's groups. It first asks a cached
// speaker for the live topology (fast); if that fails or the cache is empty
// or disabled, it falls back to SSDP discovery and refreshes the cache.
func DiscoverGroups(ctx context.Context, opt DiscoverOptions) ([]Group, error) {
	if opt.Timeout == 0 {
		opt.Timeout = 3 * time.Second
	}

	if !opt.NoCache {
		for _, sp := range loadCachedSpeakers() {
			cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			groups, err := sp.Topology(cctx)

			cancel()

			if err == nil && len(groups) > 0 {
				saveCachedSpeakers(groups)
				return groups, nil
			}
		}
	}

	opt.logf("Searching for Sonos speakers...")

	speakers, err := Discover(ctx, opt.Timeout)
	if err != nil {
		return nil, err
	}

	if len(speakers) == 0 {
		return nil, fmt.Errorf("no speakers found (same network/VLAN? multicast allowed?)")
	}

	var topoErr error

	for _, sp := range speakers {
		groups, err := sp.Topology(ctx)
		if err == nil && len(groups) > 0 {
			saveCachedSpeakers(groups)
			return groups, nil
		}

		topoErr = err
	}

	return nil, fmt.Errorf("reading zone group topology: %w", topoErr)
}

// The cache only remembers which speakers exist (IPs). Group topology is
// always fetched live from a cached speaker, so group changes are never stale.

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "sonoskids", "speakers.json"), nil
}

func loadCachedSpeakers() []Speaker {
	path, err := cachePath()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is the app's own cache file
	if err != nil {
		return nil
	}

	var sp []Speaker
	if json.Unmarshal(data, &sp) != nil {
		return nil
	}

	return sp
}

// saveCachedSpeakers is best effort; failures are ignored.
func saveCachedSpeakers(groups []Group) {
	path, err := cachePath()
	if err != nil {
		return
	}

	seen := map[string]bool{}

	var sp []Speaker

	for _, g := range groups {
		for _, m := range g.Members {
			if !seen[m.IP] {
				seen[m.IP] = true
				sp = append(sp, m)
			}
		}
	}

	data, err := json.MarshalIndent(sp, "", "  ")
	if err != nil {
		return
	}

	if os.MkdirAll(filepath.Dir(path), 0o750) == nil {
		_ = os.WriteFile(path, data, 0o600)
	}
}
