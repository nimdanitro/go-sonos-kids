package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/spf13/cobra"

	"github.com/nimdanitro/go-sonos-kids/catalog"
	"github.com/nimdanitro/go-sonos-kids/meta"
	"github.com/nimdanitro/go-sonos-kids/server"
	"github.com/nimdanitro/go-sonos-kids/store"
)

// artistCatalog enables artist import for each service that has credentials.
// Secrets are never logged.
func artistCatalog(cfg serveConfig) catalog.Service {
	providers := map[string]catalog.Provider{}
	if cfg.SpotifyClientID != "" && cfg.SpotifyClientSecret != "" {
		providers["spotify"] = catalog.NewSpotify(cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.Country)
	}

	if cfg.TidalClientID != "" && cfg.TidalClientSecret != "" {
		providers["tidal"] = catalog.NewTidal(cfg.TidalClientID, cfg.TidalClientSecret, cfg.Country)
	}

	for _, name := range []string{"spotify", "tidal"} {
		if providers[name] != nil {
			slog.Info("artist import enabled", slog.String("provider", name), slog.String("country", cfg.Country))
		} else {
			slog.Info("artist import is off, no api credentials", slog.String("provider", name))
		}
	}

	return catalog.Service{Providers: providers}
}

// logSonos adapts printf-style progress messages from the sonos package to slog.
func logSonos(format string, args ...any) {
	slog.Info("sonos", slog.String("detail", fmt.Sprintf(format, args...)))
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the kids' music API server (and the hidden admin at /admin)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg := serveConfigFrom(conf)

		if configFileUsed != "" {
			slog.Info("using config file", slog.String("path", configFileUsed))
		}

		if cfg.AdminToken == "" {
			slog.Warn(
				"no admin token set (--admin-token, SONOSKIDS_ADMIN_TOKEN or admin_token in the config file): admin api is disabled",
			)
		}

		path, err := dbPath(conf)
		if err != nil {
			return err
		}

		if cfg.MigrateOnStartup {
			err := migrateUp(cmd.Context(), path, func(r *goose.MigrationResult) {
				slog.InfoContext(
					cmd.Context(),
					"migration applied",
					slog.Int64("version", r.Source.Version),
					slog.String("file", filepath.Base(r.Source.Path)),
					slog.Duration("duration", r.Duration.Round(time.Millisecond)),
				)
			})
			if err != nil {
				return fmt.Errorf("migrating the database: %w", err)
			}
		}
		// Refuses to start on an outdated schema, which can only happen with
		// --migrate-on-startup=false.
		st, err := store.OpenNoMigrate(path)
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()

		app := server.New(server.Options{
			Store:      st,
			Player:     &server.SonosPlayer{Logf: logSonos},
			Meta:       meta.NewFetcher(),
			Catalog:    artistCatalog(cfg),
			AdminToken: cfg.AdminToken,
		})
		srv := &http.Server{
			Addr:              cfg.Addr,
			Handler:           app.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// Fill in artists and release dates for items saved without them, so music
		// groups by artist. Runs in the background and never blocks startup.
		go func() {
			res, err := app.BackfillMetadata(ctx)
			switch {
			case err != nil:
				slog.ErrorContext(ctx, "metadata backfill failed", slog.Any("error", err))
			case res.Checked > 0:
				slog.InfoContext(
					ctx,
					"metadata backfill done",
					slog.Int("checked", res.Checked),
					slog.Int("updated", res.Updated),
				)
			}
		}()

		go func() {
			<-ctx.Done()

			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_ = srv.Shutdown(sctx)
		}()

		slog.InfoContext(ctx, "listening", slog.String("addr", cfg.Addr), slog.String("database", path))

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}

		return nil
	},
}

func init() {
	registerServeFlags(serveCmd.Flags())
	rootCmd.AddCommand(serveCmd)
}
