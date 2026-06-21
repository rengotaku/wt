package devserver

import (
	"testing"

	"wt/internal/wt/core"
)

func mustUpsert(t *testing.T, container string, svc Service) {
	t.Helper()
	if _, err := UpsertRepoService(container, svc); err != nil {
		t.Fatalf("upsert %s: %v", svc.Name, err)
	}
}

func TestRepoDefault_UpsertAddAndUpdate(t *testing.T) {
	container := t.TempDir()

	cfg, err := LoadRepoDefault(container)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Services) != 0 {
		t.Fatalf("expected empty, got %d", len(cfg.Services))
	}

	updated, err := UpsertRepoService(container, Service{Name: "api", Cmd: "go run . -p ${port}"})
	if err != nil {
		t.Fatalf("add api: %v", err)
	}
	if updated {
		t.Error("first add should not report updated=true")
	}
	mustUpsert(t, container, Service{Name: "web", Cmd: "npm run dev -- --port ${port}", Domain: true})

	cfg, _ = LoadRepoDefault(container)
	if len(cfg.Services) != 2 || cfg.Services[0].Name != "api" || cfg.Services[1].Name != "web" {
		t.Fatalf("unexpected services: %+v", cfg.Services)
	}
	if !cfg.Services[1].Domain {
		t.Error("web should have domain=true")
	}

	// Update api in place: cmd changes, declaration order preserved (port=base+i).
	updated, err = UpsertRepoService(container, Service{Name: "api", Cmd: "go run . web -p ${port}"})
	if err != nil {
		t.Fatalf("update api: %v", err)
	}
	if !updated {
		t.Error("update should report updated=true")
	}
	cfg, _ = LoadRepoDefault(container)
	if len(cfg.Services) != 2 || cfg.Services[0].Name != "api" ||
		cfg.Services[0].Cmd != "go run . web -p ${port}" {
		t.Fatalf("update did not keep position/cmd: %+v", cfg.Services)
	}
}

func TestRepoDefault_Remove(t *testing.T) {
	container := t.TempDir()
	mustUpsert(t, container, Service{Name: "api", Cmd: "a ${port}"})
	mustUpsert(t, container, Service{Name: "web", Cmd: "b ${port}"})

	found, err := RemoveRepoService(container, "nope")
	if err != nil {
		t.Fatalf("remove missing: %v", err)
	}
	if found {
		t.Error("removing missing should report found=false")
	}

	found, err = RemoveRepoService(container, "api")
	if err != nil {
		t.Fatalf("remove api: %v", err)
	}
	if !found {
		t.Error("remove api should report found=true")
	}
	cfg, _ := LoadRepoDefault(container)
	if len(cfg.Services) != 1 || cfg.Services[0].Name != "web" {
		t.Fatalf("after remove: %+v", cfg.Services)
	}

	// Removing the last service clears the default instead of failing Validate.
	if _, err := RemoveRepoService(container, "web"); err != nil {
		t.Fatalf("remove last: %v", err)
	}
	cfg, _ = LoadRepoDefault(container)
	if len(cfg.Services) != 0 {
		t.Fatalf("expected empty after removing last: %+v", cfg.Services)
	}
}

func TestRepoDefault_FindService(t *testing.T) {
	container := t.TempDir()
	mustUpsert(t, container, Service{Name: "api", Cmd: "a ${port}", Domain: true})

	svc, found, err := FindRepoService(container, "api")
	if err != nil || !found {
		t.Fatalf("find api: found=%v err=%v", found, err)
	}
	if svc.Cmd != "a ${port}" || !svc.Domain {
		t.Errorf("unexpected svc: %+v", svc)
	}
	if _, found, _ := FindRepoService(container, "missing"); found {
		t.Error("missing service should not be found")
	}
}

func TestRepoDefault_SaveValidates(t *testing.T) {
	container := t.TempDir()
	// Empty cmd must fail validation and must not persist anything.
	if _, err := UpsertRepoService(container, Service{Name: "api", Cmd: ""}); err == nil {
		t.Fatal("expected validation error for empty cmd")
	}
	cfg, _ := LoadRepoDefault(container)
	if len(cfg.Services) != 0 {
		t.Fatalf("invalid add must not persist: %+v", cfg.Services)
	}
}

func TestRepoDefault_PreservesOtherConfigFields(t *testing.T) {
	container := t.TempDir()
	if err := core.SaveConfig(container, core.EntryConfig{
		SymlinkCandidates: []string{"node_modules"},
		GitCryptKey:       "/keys/x",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	mustUpsert(t, container, Service{Name: "api", Cmd: "a ${port}"})

	rc, err := core.LoadConfig(container)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(rc.SymlinkCandidates) != 1 || rc.SymlinkCandidates[0] != "node_modules" {
		t.Errorf("symlink_candidates not preserved: %+v", rc.SymlinkCandidates)
	}
	if rc.GitCryptKey != "/keys/x" {
		t.Errorf("git_crypt_key not preserved: %q", rc.GitCryptKey)
	}
	if len(rc.DevServices) != 1 || rc.DevServices[0].Name != "api" {
		t.Errorf("dev_services not written: %+v", rc.DevServices)
	}

	// Clear removes dev services but keeps the other _config fields.
	if err := ClearRepoDefault(container); err != nil {
		t.Fatalf("clear: %v", err)
	}
	rc, _ = core.LoadConfig(container)
	if len(rc.DevServices) != 0 {
		t.Errorf("clear should remove dev_services: %+v", rc.DevServices)
	}
	if len(rc.SymlinkCandidates) != 1 {
		t.Errorf("clear must preserve symlink_candidates: %+v", rc.SymlinkCandidates)
	}
}
