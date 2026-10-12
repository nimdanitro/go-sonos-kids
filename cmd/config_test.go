package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// allEnv lists every environment variable the configuration reads, so a test
// can neutralise real credentials and settings on the developer's machine.
var allEnv = []string{
	"SONOSKIDS_CONFIG", "SONOSKIDS_SPEAKER", "SONOSKIDS_SN", "SONOSKIDS_NO_CACHE", "SONOSKIDS_DISCOVER_TIMEOUT", "SONOSKIDS_DRY_RUN",
	"SONOSKIDS_ADDR", "SONOSKIDS_DB", "SONOSKIDS_MIGRATE_ON_STARTUP", "SONOSKIDS_ADMIN_TOKEN", "SONOSKIDS_COUNTRY",
	"SONOSKIDS_SPOTIFY_CLIENT_ID", "SONOSKIDS_SPOTIFY_CLIENT_SECRET", "SPOTIFY_CLIENT_ID", "SPOTIFY_CLIENT_SECRET",
	"SONOSKIDS_TIDAL_CLIENT_ID", "SONOSKIDS_TIDAL_CLIENT_SECRET", "TIDAL_CLIENT_ID", "TIDAL_CLIENT_SECRET",
}

func isolate(t *testing.T) {
	t.Helper()
	for _, n := range allEnv {
		t.Setenv(n, "") // empty means unset to viper
	}
	t.Chdir(t.TempDir())                     // no ./sonoskids.yaml
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no user config either
}

func writeConfig(t *testing.T, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sonoskids.yaml")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// load builds the configuration for the full flag set, as a command would.
func load(t *testing.T, args ...string) (v *viper.Viper, file, warning string) {
	t.Helper()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	registerRootFlags(fs)
	registerSpeakerFlag(fs)
	registerServeFlags(fs)
	fs.String("sn", "", "")
	fs.Bool("dry-run", false, "")
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	v, file, warning, err := newConfig(fs)
	if err != nil {
		t.Fatalf("newConfig: %v", err)
	}
	return v, file, warning
}

const sample = `
speaker: Globi
discover_timeout: 7s
no_cache: true
addr: 0.0.0.0:9000
admin_token: from-file
country: DE
spotify:
  client_id: file-spotify-id
  client_secret: file-spotify-secret
tidal:
  client_id: file-tidal-id
  client_secret: file-tidal-secret
`

func TestDefaultsWithoutAnyConfig(t *testing.T) {
	isolate(t)
	v, file, _ := load(t)
	cfg := serveConfigFrom(v)
	if file != "" || cfg.Addr != "127.0.0.1:8080" || cfg.Country != "CH" || cfg.AdminToken != "" || cfg.SpotifyClientID != "" {
		t.Errorf("serve defaults: file=%q cfg=%+v", file, cfg)
	}
	if v.GetDuration("discover_timeout") != 3*time.Second || v.GetBool("no_cache") || v.GetString("speaker") != "" {
		t.Errorf("cli defaults: %v %v %q", v.GetDuration("discover_timeout"), v.GetBool("no_cache"), v.GetString("speaker"))
	}
}

func TestConfigFileValuesForEveryCommand(t *testing.T) {
	isolate(t)
	path := writeConfig(t, sample, 0o600)
	v, file, warning := load(t, "--config", path)
	want := serveConfig{
		Addr: "0.0.0.0:9000", AdminToken: "from-file", Country: "DE", MigrateOnStartup: true, // on unless switched off
		SpotifyClientID: "file-spotify-id", SpotifyClientSecret: "file-spotify-secret",
		TidalClientID: "file-tidal-id", TidalClientSecret: "file-tidal-secret",
	}
	if file != path || warning != "" || serveConfigFrom(v) != want {
		t.Errorf("file=%q warning=%q\n got %+v\nwant %+v", file, warning, serveConfigFrom(v), want)
	}
	// The CLI commands' settings come from the same file.
	if v.GetString("speaker") != "Globi" || v.GetDuration("discover_timeout") != 7*time.Second || !v.GetBool("no_cache") {
		t.Errorf("cli settings from file: %q %v %v", v.GetString("speaker"), v.GetDuration("discover_timeout"), v.GetBool("no_cache"))
	}
}

func TestPrecedenceFlagOverEnvOverFile(t *testing.T) {
	isolate(t)
	path := writeConfig(t, sample, 0o600)

	// Environment beats the file; the prefixed name beats the legacy one.
	t.Setenv("SPOTIFY_CLIENT_ID", "legacy-env-id")
	t.Setenv("SONOSKIDS_SPOTIFY_CLIENT_ID", "prefixed-env-id")
	t.Setenv("TIDAL_CLIENT_SECRET", "legacy-env-secret")
	t.Setenv("SONOSKIDS_ADMIN_TOKEN", "env-token")
	t.Setenv("SONOSKIDS_SPEAKER", "Globine")
	t.Setenv("SONOSKIDS_DISCOVER_TIMEOUT", "9s")
	t.Setenv("SONOSKIDS_NO_CACHE", "false")
	v, _, _ := load(t, "--config", path)
	cfg := serveConfigFrom(v)
	if cfg.SpotifyClientID != "prefixed-env-id" || cfg.TidalClientSecret != "legacy-env-secret" || cfg.AdminToken != "env-token" {
		t.Errorf("env over file (serve): %+v", cfg)
	}
	if v.GetString("speaker") != "Globine" || v.GetDuration("discover_timeout") != 9*time.Second || v.GetBool("no_cache") {
		t.Errorf("env over file (cli): %q %v %v", v.GetString("speaker"), v.GetDuration("discover_timeout"), v.GetBool("no_cache"))
	}
	if cfg.SpotifyClientSecret != "file-spotify-secret" || cfg.Country != "DE" {
		t.Errorf("untouched keys must still come from the file: %+v", cfg)
	}

	// A flag beats both.
	v, _, _ = load(t, "--config", path, "--admin-token", "flag-token", "--country", "AT", "--spotify-client-id", "flag-id",
		"--speaker", "Living Room", "--discover-timeout", "1s", "--no-cache")
	cfg = serveConfigFrom(v)
	if cfg.AdminToken != "flag-token" || cfg.Country != "AT" || cfg.SpotifyClientID != "flag-id" {
		t.Errorf("flag over env (serve): %+v", cfg)
	}
	if v.GetString("speaker") != "Living Room" || v.GetDuration("discover_timeout") != time.Second || !v.GetBool("no_cache") {
		t.Errorf("flag over env (cli): %q %v %v", v.GetString("speaker"), v.GetDuration("discover_timeout"), v.GetBool("no_cache"))
	}
}

func TestConfigFileSelection(t *testing.T) {
	isolate(t)

	// The path itself can come from the environment, like any other setting.
	t.Setenv("SONOSKIDS_CONFIG", writeConfig(t, "country: SE\n", 0o600))
	if v, _, _ := load(t); v.GetString("country") != "SE" {
		t.Errorf("SONOSKIDS_CONFIG ignored: %q", v.GetString("country"))
	}
	t.Setenv("SONOSKIDS_CONFIG", "")

	// Otherwise ./sonoskids.yaml is found.
	if err := os.WriteFile("sonoskids.yaml", []byte("country: FR\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, file, _ := load(t)
	if file != "sonoskids.yaml" || v.GetString("country") != "FR" {
		t.Errorf("local config: file=%q country=%q", file, v.GetString("country"))
	}
}

func TestBadConfigFilesAreErrors(t *testing.T) {
	isolate(t)
	newErr := func(args ...string) error {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		registerRootFlags(fs)
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		_, _, _, err := newConfig(fs)
		return err
	}
	if err := newErr("--config", filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("expected an error for a missing --config file")
	}
	if err := newErr("--config", writeConfig(t, "addr: [unclosed", 0o600)); err == nil || !strings.Contains(err.Error(), "reading config") {
		t.Errorf("malformed config: %v", err)
	}
}

func TestPermissionWarning(t *testing.T) {
	isolate(t)
	if _, _, w := load(t, "--config", writeConfig(t, sample, 0o644)); !strings.Contains(w, "chmod 600") {
		t.Errorf("a world-readable file with secrets should warn, got %q", w)
	}
	if _, _, w := load(t, "--config", writeConfig(t, sample, 0o600)); w != "" {
		t.Errorf("a private file must not warn, got %q", w)
	}
	if _, _, w := load(t, "--config", writeConfig(t, "country: DE\n", 0o644)); w != "" {
		t.Errorf("a file without secrets must not warn, got %q", w)
	}
}

// Every setting must flow through viper. Binding a flag straight to a Go
// variable (StringVar, BoolVar, ...) would bypass the config file and the
// environment, so this fails if anyone adds one.
func TestNoFlagIsBoundToAVariable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	direct := regexp.MustCompile(`\.(String|Bool|Int|Int64|Duration|Float64|StringSlice)(P)?Var\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := direct.Find(src); m != nil {
			t.Errorf("%s binds a flag to a variable (%s...); define it with fs.String/Bool/... and read it via conf", f, m)
		}
	}
}

// The documented sample must load, and the README copy must match the example file.
func TestSampleConfigLoadsAndMatchesReadme(t *testing.T) {
	// Resolve the repo files before isolate() changes the working directory.
	examplePath, err := filepath.Abs("../sonoskids.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	readmePath, err := filepath.Abs("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	isolate(t)

	example, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	v, _, _ := load(t, "--config", examplePath)
	cfg := serveConfigFrom(v)
	if cfg.Addr != "127.0.0.1:8080" || cfg.AdminToken != "change-me" || cfg.Country != "CH" {
		t.Errorf("example values: %+v", cfg)
	}
	if v.GetDuration("discover_timeout") != 3*time.Second { // commented out in the sample, so the default applies
		t.Errorf("discover_timeout: %v", v.GetDuration("discover_timeout"))
	}

	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindSubmatch(readme)
	if block == nil {
		t.Fatal("no yaml block in README")
	}
	const marker = "# --- `sonoskids serve`"
	body := func(b []byte) string {
		s := string(b)
		i := strings.Index(s, marker)
		if i < 0 {
			t.Fatalf("marker not found in %.40q", s)
		}
		return strings.TrimSpace(s[i:])
	}
	if body(block[1]) != body(example) {
		t.Error("the sample in README.md differs from sonoskids.example.yaml; keep them identical")
	}
}

// Migrations run on startup unless switched off, by flag, environment variable or config file.
func TestMigrateOnStartupDefaultsToTrue(t *testing.T) {
	isolate(t)
	if v, _, _ := load(t); !serveConfigFrom(v).MigrateOnStartup {
		t.Error("migrate on startup must be on by default")
	}
	if v, _, _ := load(t, "--migrate-on-startup=false"); serveConfigFrom(v).MigrateOnStartup {
		t.Error("the flag should switch it off")
	}
	t.Setenv("SONOSKIDS_MIGRATE_ON_STARTUP", "false")
	if v, _, _ := load(t); serveConfigFrom(v).MigrateOnStartup {
		t.Error("the environment variable should switch it off")
	}
	if v, _, _ := load(t, "--migrate-on-startup"); !serveConfigFrom(v).MigrateOnStartup {
		t.Error("a flag beats the environment")
	}
	t.Setenv("SONOSKIDS_MIGRATE_ON_STARTUP", "")
	if v, _, _ := load(t, "--config", writeConfig(t, "migrate_on_startup: false\n", 0o600)); serveConfigFrom(v).MigrateOnStartup {
		t.Error("the config file should switch it off")
	}
}
