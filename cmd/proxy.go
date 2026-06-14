package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"wt/internal/wt/proxy"
)

func registerProxyCmd(parent *cobra.Command) {
	var port int
	c := &cobra.Command{
		Use:   "proxy",
		Short: "*.wt.localhost のリバースプロキシを起動（worktree のサーバーに名前でアクセス）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			srv := &http.Server{
				Addr:              addr,
				Handler:           proxy.Handler(proxy.Routes),
				ReadHeaderTimeout: 5 * time.Second,
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "wt proxy listening on http://%s\n", addr)
			if routes, err := proxy.Routes(); err == nil {
				for _, r := range routes {
					_, _ = fmt.Fprintf(out, "  http://%s:%d → 127.0.0.1:%d (%s/%s)\n",
						r.Domain(), port, r.Port, r.Repo, r.WtName)
				}
			}
			return srv.ListenAndServe()
		},
	}
	c.Flags().IntVarP(&port, "port", "p", 8088, "リッスンポート")
	parent.AddCommand(c)
}
