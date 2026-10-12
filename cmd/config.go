package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// conf is the single source of configuration for every command. viper merges
// the command-line flags, environment variables and the config file, in that
// order of precedence, then the flag defaults. Commands never read flag
// variables directly; they read values from conf, so a setting behaves the
// same wherever it comes from.
//
// A flag's key is its name with dashes turned into underscores (--no-cache is
// no_cache). Environment variables are SONOSKIDS_ plus the upper-case key
// (SONOSKIDS_NO_CACHE). Nested keys (spotify.client_id) use "_" in the
// environment variable name (SONOSKIDS_SPOTIFY_CLIENT_ID).
var conf = viper.New()

// configFileUsed is the file conf was read from, or "" if there was none.
var configFileUsed string

// nestedKeys are flags that live under a section in the config file.
var nestedKeys = map[string]string{
	"spotify-client-id":     "spotify.client_id",
	"spotify-client-secret": "spotify.client_secret",
	"tidal-client-id":       "tidal.client_id",
	"tidal-client-secret":   "tidal.client_secret",
}

// legacyEnv are extra environment variables accepted for a key, because those
// are the names the services' own tooling uses.
var legacyEnv = map[string][]string{
	"spotify.client_id":     {"SONOSKIDS_SPOTIFY_CLIENT_ID", "SPOTIFY_CLIENT_ID"},
	"spotify.client_secret": {"SONOSKIDS_SPOTIFY_CLIENT_SECRET", "SPOTIFY_CLIENT_SECRET"},
	"tidal.client_id":       {"SONOSKIDS_TIDAL_CLIENT_ID", "TIDAL_CLIENT_ID"},
	"tidal.client_secret":   {"SONOSKIDS_TIDAL_CLIENT_SECRET", "TIDAL_CLIENT_SECRET"},
}

func flagKey(name string) string {
	if key, ok := nestedKeys[name]; ok {
		return key
	}
	return strings.ReplaceAll(name, "-", "_")
}

// Flag definitions are shared between the commands and the tests.

func registerRootFlags(fs *pflag.FlagSet) {
	fs.String("config", "", "config file (default: ./sonoskids.yaml or <user config dir>/sonoskids/config.yaml)")
	fs.Bool("no-cache", false, "ignore the cached speaker list and rediscover via SSDP")
	fs.Duration("discover-timeout", 3*time.Second, "how long to wait for speakers to answer")
}

func registerSpeakerFlag(fs *pflag.FlagSet) {
	fs.StringP("speaker", "s", "", "group/room name or IP (prompts if empty)")
}

// registerDBFlag defines --db, shared by serve and the migrate commands.
func registerDBFlag(fs *pflag.FlagSet) {
	fs.String("db", "", "SQLite database path (default: <user config dir>/sonoskids/sonoskids.db)")
}

func registerServeFlags(fs *pflag.FlagSet) {
	fs.String("addr", "127.0.0.1:8080", "listen address (use :8080 to allow other devices)")
	registerDBFlag(fs)
	fs.Bool("migrate-on-startup", true, "apply pending database migrations before starting (use --migrate-on-startup=false to run `migrate up` yourself)")
	fs.String("admin-token", "", "admin API token")
	fs.String("country", "CH", "country code for artist import (only releases available there are listed)")
	fs.String("spotify-client-id", "", "Spotify API client ID for artist import")
	fs.String("spotify-client-secret", "", "Spotify API client secret")
	fs.String("tidal-client-id", "", "Tidal API client ID for artist import")
	fs.String("tidal-client-secret", "", "Tidal API client secret")
}

// serveConfig is what `serve` is configured with.
type serveConfig struct {
	Addr       string
	DB         string
	AdminToken string
	Country    string

	MigrateOnStartup bool

	SpotifyClientID, SpotifyClientSecret string
	TidalClientID, TidalClientSecret     string
}

func serveConfigFrom(v *viper.Viper) serveConfig {
	return serveConfig{
		Addr:                v.GetString("addr"),
		DB:                  v.GetString("db"),
		AdminToken:          v.GetString("admin_token"),
		Country:             v.GetString("country"),
		MigrateOnStartup:    v.GetBool("migrate_on_startup"),
		SpotifyClientID:     v.GetString("spotify.client_id"),
		SpotifyClientSecret: v.GetString("spotify.client_secret"),
		TidalClientID:       v.GetString("tidal.client_id"),
		TidalClientSecret:   v.GetString("tidal.client_secret"),
	}
}

// dbPath is the configured database path, or the default one in the user
// config directory. Its directory is created if it does not exist.
func dbPath(v *viper.Viper) (string, error) {
	path := v.GetString("db")
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(dir, "sonoskids", "sonoskids.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// configSearchPaths are the places looked at when no config file is named.
func configSearchPaths() []string {
	paths := []string{"sonoskids.yaml", "sonoskids.yml"}
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, "sonoskids", "config.yaml"), filepath.Join(dir, "sonoskids", "config.yml"))
	}
	return paths
}

// newConfig builds a viper instance for a command's flag set: every flag is
// bound to its key, the environment is consulted, and the config file (the
// "config" flag or SONOSKIDS_CONFIG, else the first file in configSearchPaths)
// is read. It returns the file that was read ("" if none) and a warning, if
// any, about the file's permissions.
func newConfig(flags *pflag.FlagSet) (v *viper.Viper, file, warning string, err error) {
	v = viper.New()
	v.SetEnvPrefix("SONOSKIDS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	flags.VisitAll(func(f *pflag.Flag) {
		if err == nil {
			err = v.BindPFlag(flagKey(f.Name), f)
		}
	})
	for key, names := range legacyEnv {
		if err == nil {
			err = v.BindEnv(append([]string{key}, names...)...)
		}
	}
	if err != nil {
		return nil, "", "", err
	}

	file = v.GetString("config")
	if file == "" {
		for _, p := range configSearchPaths() {
			if _, statErr := os.Stat(p); statErr == nil {
				file = p
				break
			}
		}
	}
	if file != "" {
		v.SetConfigFile(file)
		if err := v.ReadInConfig(); err != nil {
			return nil, "", "", fmt.Errorf("reading config %s: %w", file, err)
		}
		warning = permissionWarning(v, file)
	}
	return v, file, warning, nil
}

// initConfig runs before every command and makes conf reflect its flags.
func initConfig(cmd *cobra.Command) error {
	v, file, warning, err := newConfig(cmd.Flags())
	if err != nil {
		return err
	}
	conf, configFileUsed = v, file
	if warning != "" {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	return nil
}

// permissionWarning flags a config file that holds secrets but can be read by
// other users on the machine.
func permissionWarning(v *viper.Viper, file string) string {
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm()&0o077 == 0 {
		return ""
	}
	for _, key := range []string{"admin_token", "spotify.client_secret", "tidal.client_secret"} {
		if v.InConfig(key) {
			return fmt.Sprintf("%s contains secrets but is readable by other users; run: chmod 600 %s", file, file)
		}
	}
	return ""
}
