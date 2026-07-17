package handler

import (
	"net/http"

	"wt/internal/wt/ports"
	"wt/internal/wt/settings"
)

type devPortsDTO struct {
	Start     int `json:"start"`
	End       int `json:"end"`
	BlockSize int `json:"block_size"`
}

type settingsDTO struct {
	DevPorts devPortsDTO `json:"dev_ports"`
}

func toSettingsDTO(s *settings.Settings) settingsDTO {
	return settingsDTO{DevPorts: devPortsDTO{
		Start:     s.DevPorts.Start,
		End:       s.DevPorts.End,
		BlockSize: ports.BlockSize,
	}}
}

// GetSettings returns the current wt settings.
func (h *Handler) GetSettings(w http.ResponseWriter, _ *http.Request) {
	cur := settings.Load()
	jsonOK(w, toSettingsDTO(&cur))
}

type updateSettingsRequest struct {
	DevPorts struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"dev_ports"`
}

// UpdateSettings validates and persists the dev port band. Other sections
// (Proxy, IdleReaper, etc.) are preserved from the on-disk settings so the
// partial UI update doesn't clobber them.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	s := settings.Load()
	s.DevPorts = settings.DevPorts{Start: req.DevPorts.Start, End: req.DevPorts.End}
	if err := settings.Save(&s); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	next := settings.Load()
	jsonOK(w, toSettingsDTO(&next))
}
