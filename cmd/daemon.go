package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

LaunchAgent などから起動する。SIGINT / SIGTERM で終了する。
同時に動かせるのは 1 プロセスだけ（ユーザーキャッシュディレクトリの wt/daemon.lock で排他）。
proxy が有効で待ち受けに失敗した場合はエラーで終了する（サービスマネージャーの再起動に任せる）。`

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
				if proxyPort < 1 || proxyPort > 65535 {
					return fmt.Errorf("--proxy-port は 1-65535 の範囲で指定してください: %d", proxyPort)
				}
				s.Proxy.Port = proxyPort
			}
			lock, err := acquireDaemonLock(daemonLockPath())
			if err != nil {
				return err
			}
			defer func() { _ = lock.Close() }()
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
			// Exit instead of running without the proxy: the service manager
			// restarts us, so a transient conflict (e.g. the previous process
			// still holding the port) recovers on its own.
			return fmt.Errorf("内蔵 proxy の起動に失敗しました（port=%d。止めるなら --no-proxy か settings.toml [proxy].enabled=false）: %w", s.Proxy.Port, err)
		}
		defer func() { _ = srv.Close() }()
		_, _ = fmt.Fprintf(out, "wt proxy listening on http://%s (suffix %s)\n", srv.Addr, proxy.DomainSuffix)
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

// daemonLockPath is the lock file shared by every wt daemon of this user.
func daemonLockPath() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "wt", "daemon.lock")
}

// acquireDaemonLock takes an exclusive, non-blocking flock on path. Two daemons
// would both serve the same AutoStart worktrees and overwrite each other's
// running.json, so a second one must refuse to start. The lock is released
// when the returned file is closed or the process exits.
func acquireDaemonLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("wt daemon は既に起動しています（lock: %s）", path)
		}
		return nil, fmt.Errorf("lock を取得できません（%s）: %w", path, err)
	}
	return f, nil
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
