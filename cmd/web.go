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
	"wt/internal/wt/proxy"
	"wt/internal/wt/settings"
)

func registerWebCmd(parent *cobra.Command) {
	var (
		port      int
		proxyPort int
		noProxy   bool
	)
	c := &cobra.Command{
		Use:   "web",
		Short: "SPA + JSON API を 127.0.0.1 で起動（内蔵 proxy も既定で同時起動）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := settings.Load()

			// Resolve effective proxy config: CLI flags override settings.toml.
			effProxyPort := s.Proxy.Port
			if cmd.Flags().Changed("proxy-port") {
				effProxyPort = proxyPort
			}
			effProxyEnabled := s.Proxy.Enabled
			if noProxy {
				effProxyEnabled = false
			}

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			h := handler.New(effProxyPort)
			srv := &http.Server{
				Addr:    addr,
				Handler: h.Routes(static.Handler()),
			}
			slog.Info("wt web listening", "addr", "http://"+addr)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "http://%s\n", addr)

			if effProxyEnabled {
				if err := h.StartBuiltinProxy(); err != nil {
					slog.Warn("built-in proxy failed to start", "port", effProxyPort, "err", err)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "warning: 内蔵 proxy の起動に失敗しました（port=%d）: %v\n", effProxyPort, err)
				} else {
					proxyAddr := fmt.Sprintf("127.0.0.1:%d", effProxyPort)
					slog.Info("wt proxy listening", "addr", "http://"+proxyAddr, "suffix", proxy.DomainSuffix)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wt proxy listening on http://%s (suffix %s)\n", proxyAddr, proxy.DomainSuffix)
				}
			}

			// Auto-serve AutoStart worktrees in the background so the UI is
			// available immediately while their dev servers spin up.
			go func() {
				if n := autostart.ServeAutoStart(cmd.OutOrStdout()); n > 0 {
					slog.Info("auto-start worktrees served", "count", n)
				}
			}()

			if rc := s.IdleReaper; rc.Enabled {
				r := autostart.NewReaper(time.Duration(rc.TTLMinutes)*time.Minute, time.Duration(rc.IntervalMinutes)*time.Minute)
				go r.Run(cmd.Context(), cmd.OutOrStdout())
			}

			// Auto-prune ghost port allocations (removed worktree, lingering
			// port_base) periodically (once a day by default) so they don't
			// withhold blocks from the dev band forever.
			if pc := s.PortReaper; pc.Enabled {
				pr := autostart.NewPortReaper(time.Duration(pc.IntervalMinutes) * time.Minute)
				go pr.Run(cmd.Context(), cmd.OutOrStdout())
			}

			return srv.ListenAndServe()
		},
	}
	c.Flags().IntVarP(&port, "port", "p", 8090, "リッスンポート")
	c.Flags().IntVar(&proxyPort, "proxy-port", settings.DefaultProxyPort, "内蔵 proxy のリッスンポート（settings.toml [proxy].port を上書き）")
	c.Flags().BoolVar(&noProxy, "no-proxy", false, "内蔵 proxy を起動しない（settings.toml [proxy].enabled を上書き）")
	parent.AddCommand(c)
}
