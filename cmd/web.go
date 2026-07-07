package cmd

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"wt/internal/handler"
	"wt/internal/static"
	"wt/internal/wt/autostart"
	"wt/internal/wt/settings"
)

func registerWebCmd(parent *cobra.Command) {
	var port int
	c := &cobra.Command{
		Use:   "web",
		Short: "SPA + JSON API を 127.0.0.1 で起動",
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			h := handler.New()
			srv := &http.Server{
				Addr:    addr,
				Handler: h.Routes(static.Handler()),
			}
			slog.Info("wt web listening", "addr", "http://"+addr)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "http://%s\n", addr)
			// Auto-serve pinned worktrees in the background so the UI is
			// available immediately while pinned dev servers spin up.
			go func() {
				if n := autostart.ServePinned(cmd.OutOrStdout()); n > 0 {
					slog.Info("pinned worktrees auto-served", "count", n)
				}
			}()

			if rc := settings.Load().IdleReaper; rc.Enabled {
				r := autostart.NewReaper(time.Duration(rc.TTLMinutes)*time.Minute, time.Duration(rc.IntervalMinutes)*time.Minute)
				go r.Run(cmd.Context(), cmd.OutOrStdout())
			}

			return srv.ListenAndServe()
		},
	}
	c.Flags().IntVarP(&port, "port", "p", 8090, "リッスンポート")
	parent.AddCommand(c)
}
