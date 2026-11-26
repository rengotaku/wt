package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"wt/internal/wt/autostart"
	"wt/internal/wt/proxy"
	"wt/internal/wt/settings"
)

const daemonLong = `常駐して次を行う（HTTP API は持たない）。

  - 内蔵 proxy（*.wt.localhost → 各 worktree の dev サーバー）。settings.toml [proxy]
  - AutoStart の worktree を起動時に serve する（wt tree autostart で設定）
  - idle / port / health の各 reaper。settings.toml [idle_reaper] [port_reaper] [health_reaper]

LaunchAgent などから起動する。SIGINT / SIGTERM で終了する。`

func registerDaemonCmd(parent *cobra.Command) {
	var (
		proxyPort int
		noProxy   bool
	)
	c := &cobra.Command{
		Use:   "daemon",
		Short: "内蔵 proxy・AutoStart・reaper を常駐実行する",
		Long:  daemonLong,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := settings.Load()
			if cmd.Flags().Changed("proxy-port") {
				s.Proxy.Port = proxyPort
			}
			if noProxy {
				s.Proxy.Enabled = false
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runDaemon(ctx, cmd.OutOrStdout(), &s)
		},
	}
	c.Flags().IntVar(&proxyPort, "proxy-port", settings.DefaultProxyPort, "内蔵 proxy のリッスンポート（settings.toml [proxy].port を上書き）")
	c.Flags().BoolVar(&noProxy, "no-proxy", false, "内蔵 proxy を起動しない（settings.toml [proxy].enabled を上書き）")
	parent.AddCommand(c)
}

// runDaemon starts every background job and blocks until ctx is cancelled.
func runDaemon(ctx context.Context, out io.Writer, s *settings.Settings) error {
	if s.Proxy.Enabled {
		srv, err := startProxy(s.Proxy)
		if err != nil {
			// The rest of the daemon is still useful without the proxy.
			slog.Warn("built-in proxy failed to start", "port", s.Proxy.Port, "err", err)
			_, _ = fmt.Fprintf(out, "warning: 内蔵 proxy の起動に失敗しました（port=%d）: %v\n", s.Proxy.Port, err)
		} else {
			defer func() { _ = srv.Close() }()
			_, _ = fmt.Fprintf(out, "wt proxy listening on http://%s (suffix %s)\n", srv.Addr, proxy.DomainSuffix)
		}
	}

	go func() {
		if n := autostart.ServeAutoStart(out); n > 0 {
			slog.Info("auto-start worktrees served", "count", n)
		}
	}()

	if rc := s.IdleReaper; rc.Enabled {
		r := autostart.NewReaper(time.Duration(rc.TTLMinutes)*time.Minute, time.Duration(rc.IntervalMinutes)*time.Minute)
		go r.Run(ctx, out)
	}
	// Ghost port allocations (removed worktree, lingering port_base) would
	// otherwise withhold blocks from the dev band forever.
	if pc := s.PortReaper; pc.Enabled {
		pr := autostart.NewPortReaper(time.Duration(pc.IntervalMinutes) * time.Minute)
		go pr.Run(ctx, out)
	}
	// Re-serve dev services that died without an explicit down. #137
	if hc := s.HealthReaper; hc.Enabled {
		hr := autostart.NewHealthReaper(
			time.Duration(hc.IntervalMinutes)*time.Minute,
			time.Duration(hc.CooldownMinutes)*time.Minute,
			hc.MaxRetries,
		)
		go hr.Run(ctx, out)
	}

	<-ctx.Done()
	return nil
}

// startProxy binds the proxy synchronously so a bind failure is reported, then
// serves in the background.
func startProxy(p settings.Proxy) (*http.Server, error) {
	bind := strings.TrimSpace(p.Bind)
	if bind == "" {
		bind = settings.DefaultProxyBind
	}
	addr := net.JoinHostPort(bind, strconv.Itoa(p.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           proxy.Handler(proxy.Routes),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	return srv, nil
}
