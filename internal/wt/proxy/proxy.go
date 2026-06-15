// Package proxy implements wt's built-in reverse proxy. It listens on a single
// port and routes requests by Host header (<label>.wt.localhost) to the
// allocated port of each worktree's domain-exposed dev service, so worktree
// servers can be reached by name instead of remembering port numbers.
package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"

	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
)

// DomainSuffix is the hostname suffix routed by the proxy.
const DomainSuffix = ".wt.localhost"

var issueRe = regexp.MustCompile(`issue[-_]?(\d+)`)
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Label derives a worktree's domain label: "main" for the main worktree,
// "issue<N>" when the branch references an issue, else a sanitized branch name.
func Label(branch, wtName string) string {
	if wtName == "main" || wtName == "master" || branch == "main" || branch == "master" {
		return "main"
	}
	low := strings.ToLower(branch)
	if m := issueRe.FindStringSubmatch(low); m != nil {
		return "issue" + m[1]
	}
	s := strings.Trim(nonAlnum.ReplaceAllString(low, "-"), "-")
	if s == "" {
		s = strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(wtName), "-"), "-")
	}
	if s == "" {
		s = "wt"
	}
	return s
}

// Route maps a domain label to the worktree's domain-service port.
type Route struct {
	Label  string
	Port   int
	Repo   string
	WtName string
}

// Domain returns the full host for a label (e.g. "issue10.wt.localhost").
func (r Route) Domain() string { return r.Label + DomainSuffix }

// Routes scans every allocated worktree and returns a route for each one whose
// .wt/dev.toml declares a domain-exposed service. The route port is the
// allocated port of that service (base + its declaration index).
func Routes() ([]Route, error) {
	allocs, err := ports.Allocations()
	if err != nil {
		return nil, err
	}
	var routes []Route
	for _, a := range allocs {
		if a.PortBase == 0 {
			continue
		}
		// EffectiveConfig resolves the dev config from metadata (worktree override
		// > repo default) or a committed file, so domain routes work regardless of
		// where the config lives.
		cfg, _, err := devserver.EffectiveConfig(a.Path)
		if err != nil {
			continue
		}
		for i, svc := range cfg.Services {
			if svc.Domain {
				routes = append(routes, Route{
					Label:  Label(a.Branch, a.WtName),
					Port:   a.PortBase + i,
					Repo:   a.Repo,
					WtName: a.WtName,
				})
				break
			}
		}
	}
	return routes, nil
}

// labelFromHost extracts the "<label>" from "<label>.wt.localhost[:port]".
func labelFromHost(host string) (string, bool) {
	host = strings.SplitN(host, ":", 2)[0]
	if !strings.HasSuffix(host, DomainSuffix) {
		return "", false
	}
	return strings.TrimSuffix(host, DomainSuffix), true
}

// Handler reverse-proxies by Host. routesFn supplies the current routing table
// (re-evaluated per request so newly started worktrees are picked up live).
func Handler(routesFn func() ([]Route, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		label, ok := labelFromHost(r.Host)
		if !ok {
			http.Error(w, "アクセスは <label>"+DomainSuffix+" で行ってください", http.StatusNotFound)
			return
		}
		routes, err := routesFn()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, rt := range routes {
			if rt.Label == label {
				target := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", rt.Port)}
				httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, fmt.Sprintf("'%s' に対応する worktree がありません（wt serve で起動済みか確認してください）", label), http.StatusBadGateway)
	})
}
