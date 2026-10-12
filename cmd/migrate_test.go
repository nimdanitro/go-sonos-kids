package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the real command tree and returns what it printed.
func run(t *testing.T, args ...string) string {
	t.Helper()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	t.Cleanup(func() { rootCmd.SetArgs(nil); rootCmd.SetOut(nil); rootCmd.SetErr(nil) })

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("sonoskids %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}

	return out.String()
}

func TestMigrateCommands(t *testing.T) {
	isolate(t)
	db := filepath.Join(t.TempDir(), "sonoskids.db")

	status := run(t, "migrate", "status", "--db", db)
	if strings.Count(status, "pending") != 4 || strings.Contains(status, "applied") {
		t.Errorf("a new database has everything pending:\n%s", status)
	}

	up := run(t, "migrate", "up", "--db", db)
	for _, want := range []string{"00001_baseline.sql", "00002_artist_release_date_playlist.go", "00003_played_at.go", "00004_progress.sql"} {
		if !strings.Contains(up, want) {
			t.Errorf("up should report %s:\n%s", want, up)
		}
	}

	if status = run(
		t,
		"migrate",
		"status",
		"--db",
		db,
	); strings.Count(status, "applied") != 4 ||
		strings.Contains(status, "pending") {
		t.Errorf("after up everything is applied:\n%s", status)
	}

	if again := run(t, "migrate", "up", "--db", db); !strings.Contains(again, "up to date") {
		t.Errorf("a second up has nothing to do:\n%s", again)
	}

	if down := run(t, "migrate", "down", "--db", db); !strings.Contains(down, "rolled back  00004") {
		t.Errorf("down undoes the newest migration:\n%s", down)
	}

	status = run(t, "migrate", "status", "--db", db)
	pending4 := false

	for line := range strings.SplitSeq(status, "\n") {
		if strings.Contains(line, "pending") && strings.Contains(line, "00004_progress.sql") {
			pending4 = true
		}
	}

	if strings.Count(status, "applied") != 3 || !pending4 {
		t.Errorf("after down 00004 is pending again:\n%s", status)
	}

	if up = run(t, "migrate", "up", "--db", db); strings.Count(up, "applied") != 1 || !strings.Contains(up, "00004") {
		t.Errorf("up re-applies just that one:\n%s", up)
	}
}

func TestMigrateDownWithNothingAppliedIsNotAnError(t *testing.T) {
	isolate(t)

	db := filepath.Join(t.TempDir(), "empty.db")
	if out := run(t, "migrate", "down", "--db", db); !strings.Contains(out, "nothing to roll back") {
		t.Errorf("down on a new database:\n%s", out)
	}
}

func TestMigrateUsesTheConfiguredDatabase(t *testing.T) {
	isolate(t)
	db := filepath.Join(t.TempDir(), "from-env.db")
	t.Setenv("SONOSKIDS_DB", db) // no --db flag: the setting comes through viper like any other
	run(t, "migrate", "up")

	if status := run(t, "migrate", "status"); strings.Count(status, "applied") != 4 {
		t.Errorf("status of the database named by SONOSKIDS_DB:\n%s", status)
	}
}

func TestVersionFlagReportsTheBuild(t *testing.T) {
	isolate(t)
	t.Cleanup(func() { SetBuildInfo("", "") })

	SetBuildInfo("v1.2.3", "0123456789abcdef")

	if out := run(t, "--version"); !strings.Contains(out, "v1.2.3 (0123456)") {
		t.Errorf("--version: %q", out)
	}

	SetBuildInfo("", "") // a plain `go build`

	if out := run(t, "--version"); !strings.Contains(out, "dev") || strings.Contains(out, "()") {
		t.Errorf("--version without build info: %q", out)
	}
}

// A container has no home directory. Without a database path the server must
// say what to do, not just fail with a bare "$HOME is not defined".
func TestNoDatabasePathAndNoHomeGivesAnActionableError(t *testing.T) {
	isolate(t)
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	rootCmd.SetArgs(
		[]string{"migrate", "status", "--db="},
	) // cobra keeps flag values between runs in a test process; clear the earlier --db

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() { rootCmd.SetArgs(nil); rootCmd.SetOut(nil); rootCmd.SetErr(nil) })

	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "SONOSKIDS_DB") {
		t.Errorf("error should name the setting to use, got: %v", err)
	}
}
