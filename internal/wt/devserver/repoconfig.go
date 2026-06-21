package devserver

import "wt/internal/wt/core"

// repoconfig.go manages the repository-wide default dev config stored in the
// container's .worktrees.json (_config.dev_services). This default applies to
// every worktree that has no per-worktree override, and is what `wt dev` edits.

// toCore converts runtime services into stored-metadata services.
func toCore(services []Service) []core.DevService {
	out := make([]core.DevService, 0, len(services))
	for _, s := range services {
		out = append(out, core.DevService{Name: s.Name, Cmd: s.Cmd, Domain: s.Domain})
	}
	return out
}

// LoadRepoDefault reads the repository-wide default dev config from the
// container's metadata. It returns an empty Config when no default is set.
func LoadRepoDefault(container string) (Config, error) {
	rc, err := core.LoadConfig(container)
	if err != nil {
		return Config{}, err
	}
	return fromCore(rc.DevServices), nil
}

// SaveRepoDefault validates cfg and persists it as the repository default,
// preserving the other _config fields (symlink_candidates, git_crypt_key, ...).
func SaveRepoDefault(container string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	rc, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	rc.DevServices = toCore(cfg.Services)
	return core.SaveConfig(container, rc)
}

// ClearRepoDefault removes all repository-default dev services, preserving the
// other _config fields.
func ClearRepoDefault(container string) error {
	rc, err := core.LoadConfig(container)
	if err != nil {
		return err
	}
	rc.DevServices = nil
	return core.SaveConfig(container, rc)
}

// UpsertRepoService adds svc to the repository default, or replaces the existing
// service with the same name (keeping its position). updated reports whether an
// existing service was replaced (vs appended).
func UpsertRepoService(container string, svc Service) (updated bool, err error) {
	cfg, err := LoadRepoDefault(container)
	if err != nil {
		return false, err
	}
	next := make([]Service, 0, len(cfg.Services)+1)
	for _, s := range cfg.Services {
		if s.Name == svc.Name {
			updated = true
			next = append(next, svc)
			continue
		}
		next = append(next, s)
	}
	if !updated {
		next = append(next, svc)
	}
	return updated, SaveRepoDefault(container, Config{Services: next})
}

// RemoveRepoService removes the repository-default service with the given name.
// found reports whether such a service existed. Removing the last service clears
// the default rather than failing Validate (which requires >=1 service).
func RemoveRepoService(container, name string) (found bool, err error) {
	cfg, err := LoadRepoDefault(container)
	if err != nil {
		return false, err
	}
	next := make([]Service, 0, len(cfg.Services))
	for _, s := range cfg.Services {
		if s.Name == name {
			found = true
			continue
		}
		next = append(next, s)
	}
	if !found {
		return false, nil
	}
	if len(next) == 0 {
		return true, ClearRepoDefault(container)
	}
	return true, SaveRepoDefault(container, Config{Services: next})
}

// FindRepoService returns the repository-default service with the given name.
func FindRepoService(container, name string) (svc Service, found bool, err error) {
	cfg, err := LoadRepoDefault(container)
	if err != nil {
		return Service{}, false, err
	}
	for _, s := range cfg.Services {
		if s.Name == name {
			return s, true, nil
		}
	}
	return Service{}, false, nil
}
