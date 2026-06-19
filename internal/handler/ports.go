package handler

import (
	"net/http"

	"wt/internal/wt/devserver"
	"wt/internal/wt/ports"
	"wt/internal/wt/proxy"
)

type portState struct {
	Port      int    `json:"port"`
	Listening bool   `json:"listening"`
	PID       int    `json:"pid,omitempty"`
	Proc      string `json:"proc,omitempty"`
	Service   string `json:"service,omitempty"` // dev service名（service i = base+i）
}

type portItem struct {
	Repo         string      `json:"repo"`
	WtName       string      `json:"wt_name"`
	Branch       string      `json:"branch,omitempty"`
	PortBase     int         `json:"port_base"`
	PortRange    string      `json:"port_range,omitempty"`
	Ports        []portState `json:"ports"`
	HasDevConfig bool        `json:"has_dev_config"`
	Running      bool        `json:"running"`
	Degraded     bool        `json:"degraded,omitempty"`    // running しているが記録済みサービスの一部が停止している（縮退）
	Domain       string      `json:"domain,omitempty"`      // <label>.wt.localhost when a domain service exists
	DomainPort   int         `json:"domain_port,omitempty"` // localhost port of the domain(=user-facing)サービス。「開く」の遷移先
}

type doctorRow struct {
	Port    int    `json:"port"`
	PID     int    `json:"pid,omitempty"`
	Proc    string `json:"proc,omitempty"`
	Managed bool   `json:"managed"`
	Owner   string `json:"owner,omitempty"`
}

// ListListeners returns every listening TCP port on the machine, flagged as
// wt-managed or foreign (port doctor).
func (h *Handler) ListListeners(w http.ResponseWriter, _ *http.Request) {
	rows, err := ports.Doctor()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]doctorRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, doctorRow{
			Port:    r.Port,
			PID:     r.PID,
			Proc:    r.Proc,
			Managed: r.Managed,
			Owner:   r.Owner,
		})
	}
	jsonOK(w, out)
}

// ListPorts returns the dev-band port allocation and live status for every
// worktree across all wt-managed containers.
func (h *Handler) ListPorts(w http.ResponseWriter, _ *http.Request) {
	rows, err := ports.Status()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// domain map keyed by repo/wtname for worktrees exposing a domain service.
	domainOf := map[string]string{}
	if routes, err := proxy.Routes(); err == nil {
		for _, rt := range routes {
			domainOf[rt.Repo+"/"+rt.WtName] = rt.Domain()
		}
	}
	items := make([]portItem, 0, len(rows))
	for _, r := range rows {
		// Resolve the effective dev config once: its service order maps port
		// base+i → service i, used to label each port (api/web/admin).
		cfg, source, _ := devserver.EffectiveConfig(r.Path)
		states := make([]portState, 0, len(r.Ports))
		for i, p := range r.Ports {
			st := portState{Port: p.Port, Listening: p.Listening, PID: p.PID, Proc: p.Proc}
			if i < len(cfg.Services) {
				st.Service = cfg.Services[i].Name
			}
			states = append(states, st)
		}
		// domainPort: the localhost port of the first domain-exposed service.
		// It is the user-facing UI (not the API), so the list's「開く」targets it.
		domainPort := 0
		for i := range cfg.Services {
			if cfg.Services[i].Domain {
				domainPort = r.PortBase + i
				break
			}
		}
		alive, total := devserver.RunStatus(r.Path)
		items = append(items, portItem{
			Repo:         r.Repo,
			WtName:       r.WtName,
			Branch:       r.Branch,
			PortBase:     r.PortBase,
			PortRange:    ports.RangeString(r.PortBase),
			Ports:        states,
			HasDevConfig: source != devserver.SourceNone,
			Running:      alive > 0,
			Degraded:     total > 0 && alive < total,
			Domain:       domainOf[r.Repo+"/"+r.WtName],
			DomainPort:   domainPort,
		})
	}
	jsonOK(w, items)
}
