// Package proxy implements wt's built-in reverse proxy. It listens on a single
// port and routes requests by Host header (<label>.<repo>.wt.localhost) to the
// allocated port of each worktree's domain-exposed dev service, so worktree
// servers can be reached by name instead of remembering port numbers. Including
// the repo name keeps labels from colliding across repos (every repo has a
// "main" worktree).
package proxy

import (
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
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

// Domain returns the full host for a route (e.g. "issue10.myrepo.wt.localhost").
func (r Route) Domain() string { return r.Label + "." + r.Repo + DomainSuffix }

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

// hostParts extracts "<label>" and "<repo>" from
// "<label>.<repo>.wt.localhost[:port]". The label is sanitized and never
// contains a ".", so the first dot separates it from the repo name (which may
// itself contain dots). Returns ok=false for hosts missing the repo segment.
func hostParts(host string) (label, repo string, ok bool) {
	host = strings.SplitN(host, ":", 2)[0]
	if !strings.HasSuffix(host, DomainSuffix) {
		return "", "", false
	}
	rest := strings.TrimSuffix(host, DomainSuffix)
	label, repo, found := strings.Cut(rest, ".")
	if !found || label == "" || repo == "" {
		return "", "", false
	}
	return label, repo, true
}

// Handler reverse-proxies by Host. routesFn supplies the current routing table
// (re-evaluated per request so newly started worktrees are picked up live).
func Handler(routesFn func() ([]Route, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		label, repo, ok := hostParts(r.Host)
		if !ok {
			// `.wt.localhost` は RFC 6761 によりどの端末でもその端末自身の
			// loopback へ解決されるため、LAN の別端末からは名前で到達できない。
			// そこで IP 等での直アクセスには 404 を返さず、起動中の worktree と
			// その直リンク（dev サーバは 0.0.0.0 で listen している）を一覧で
			// 返して入口にする。
			writeLANIndex(w, r, routesFn)
			return
		}
		routes, err := routesFn()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, rt := range routes {
			if rt.Label == label && rt.Repo == repo {
				target := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", rt.Port)}
				httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, fmt.Sprintf("'%s.%s' に対応する worktree がありません（wt serve で起動済みか確認してください）", label, repo), http.StatusBadGateway)
	})
}

// writeLANIndex renders the worktree list for requests that did not arrive via
// a `<label>.<repo>.wt.localhost` Host — typically `http://<host-ip>:<proxy>/`
// from a phone or another PC on the LAN.
//
// The proxy cannot serve those by name: `.localhost` always resolves to the
// requesting device's own loopback, so name-based routing is only ever usable
// on the host itself. Each dev server already listens on all interfaces, so the
// useful thing to hand back is its direct URL on this host's address.
func writeLANIndex(w http.ResponseWriter, r *http.Request, routesFn func() ([]Route, error)) {
	routes, err := routesFn()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reuse the address the client actually reached us on, so the links work
	// regardless of which interface or hostname was used.
	hostOnly := r.Host
	if h, _, splitErr := net.SplitHostPort(r.Host); splitErr == nil {
		hostOnly = h
	}

	var b strings.Builder
	b.WriteString(`<!doctype html><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>wt — 起動中の worktree</title>`)
	b.WriteString(`<style>body{font-family:system-ui,sans-serif;margin:2rem auto;max-width:40rem;padding:0 1rem;line-height:1.6}` +
		`h1{font-size:1.25rem}li{margin:.5rem 0}code{background:#8881;padding:.1rem .3rem;border-radius:.2rem}` +
		`p{color:#666;font-size:.9rem}</style>`)
	b.WriteString(`<h1>起動中の worktree</h1>`)

	if len(routes) == 0 {
		b.WriteString(`<p>起動中の dev サーバがありません（<code>wt serve</code> で起動してください）。</p>`)
	} else {
		b.WriteString(`<ul>`)
		for _, rt := range routes {
			link := "http://" + net.JoinHostPort(hostOnly, strconv.Itoa(rt.Port)) + "/"
			b.WriteString(`<li><a href="` + html.EscapeString(link) + `">` +
				html.EscapeString(rt.Repo) + " / " + html.EscapeString(rt.Label) +
				`</a> <code>` + html.EscapeString(link) + `</code></li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`<p>この一覧は <code>` + html.EscapeString(DomainSuffix) +
		`</code> 以外の Host で来たときに出ます。<code>` + html.EscapeString(DomainSuffix) +
		`</code> は規格上どの端末でもその端末自身を指すため、LAN の別端末からは名前ではなく上のポート直リンクを使ってください。</p>`)
	b.WriteString(`<p>リンクが開けない場合、その worktree の dev サーバが loopback のみで ` +
		`listen しています（proxy の bind とは別）。リポジトリ側の dev コマンドに ` +
		`<code>--host 0.0.0.0</code> 相当を足してください（Vite なら <code>--host</code>、` +
		`uvicorn なら <code>--host 0.0.0.0</code>）。</p>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, b.String())
}
