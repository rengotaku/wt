package cmd

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/spf13/cobra"

	"wt/internal/handler"
	"wt/internal/static"
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
			return srv.ListenAndServe()
		},
	}
	c.Flags().IntVarP(&port, "port", "p", 8090, "リッスンポート")
	parent.AddCommand(c)
}
