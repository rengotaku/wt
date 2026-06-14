// Package core holds helpers shared across wt subpackages.
package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkspaceDirs returns the candidate base directories where wt containers live.
func WorkspaceDirs() []string {
	home := os.Getenv("HOME")
	return []string{
		filepath.Join(home, "Workspace"),
		filepath.Join(home, "MyWorkspace"),
	}
}

// MasterDir returns the directory where ~/code/<repo> symlinks are stored.
// Honors WT_MASTER_DIR.
func MasterDir() string {
	if v := os.Getenv("WT_MASTER_DIR"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), "code")
}

// ResolveMain returns the main worktree path inside container along with its
// folder name (main or master). Returns empty strings when neither is found.
func ResolveMain(container string) (path, name string) {
	for _, candidate := range []string{"main", "master"} {
		p := filepath.Join(container, candidate)
		if _, err := os.Stat(filepath.Join(p, ".git")); err == nil {
			return p, candidate
		}
	}
	return "", ""
}

// FindContainer locates a container directory for the given repo name.
func FindContainer(repo string) (string, error) {
	for _, base := range WorkspaceDirs() {
		p := filepath.Join(base, repo)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("リポジトリが見つかりません: %s", repo)
}

// MetaFile returns the .worktrees.json path for a container.
func MetaFile(container string) string {
	return filepath.Join(container, ".worktrees.json")
}

// LoadMeta reads a container's .worktrees.json into a map. Missing file
// yields an empty map.
func LoadMeta(container string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(MetaFile(container))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, err
	}
	out := map[string]json.RawMessage{}
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveMeta writes the metadata back atomically (tmp + rename).
func SaveMeta(container string, meta map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	dst := MetaFile(container)
	tmp, err := os.CreateTemp(container, ".worktrees-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dst)
}

// EntryConfig holds the _config sub-object of .worktrees.json.
type EntryConfig struct {
	SymlinkCandidates []string `json:"symlink_candidates"`
	GitCryptKey       string   `json:"git_crypt_key,omitempty"`
}

// LoadConfig returns the _config block, defaulting to empty when missing.
func LoadConfig(container string) (EntryConfig, error) {
	meta, err := LoadMeta(container)
	if err != nil {
		return EntryConfig{}, err
	}
	raw, ok := meta["_config"]
	if !ok {
		return EntryConfig{}, nil
	}
	var c EntryConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return EntryConfig{}, err
	}
	return c, nil
}

// SaveConfig writes back the _config block, preserving other entries.
func SaveConfig(container string, cfg EntryConfig) error {
	meta, err := LoadMeta(container)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	meta["_config"] = raw
	return SaveMeta(container, meta)
}

// Entry mirrors a worktree record inside .worktrees.json.
type Entry struct {
	Type        string   `json:"type,omitempty"`
	Created     string   `json:"created,omitempty"`
	Branch      string   `json:"branch,omitempty"`
	Description string   `json:"description,omitempty"`
	Issue       string   `json:"issue,omitempty"`
	Symlinked   []string `json:"symlinked,omitempty"`
	// PortBase is the first port of the worktree's dev port block (9000-9999,
	// BlockSize ports). 0 means no allocation. See internal/wt/ports.
	PortBase int `json:"port_base,omitempty"`
}

// LoadEntries reads worktree entries (skipping _config).
func LoadEntries(container string) (map[string]Entry, error) {
	meta, err := LoadMeta(container)
	if err != nil {
		return nil, err
	}
	out := map[string]Entry{}
	for k, raw := range meta {
		if k == "_config" {
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		out[k] = e
	}
	return out, nil
}

// PutEntry inserts/updates a worktree entry, preserving _config.
func PutEntry(container, name string, entry *Entry) error {
	meta, err := LoadMeta(container)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	meta[name] = raw
	return SaveMeta(container, meta)
}

// DeleteEntry removes a worktree entry.
func DeleteEntry(container, name string) error {
	meta, err := LoadMeta(container)
	if err != nil {
		return err
	}
	if _, ok := meta[name]; !ok {
		return nil
	}
	delete(meta, name)
	return SaveMeta(container, meta)
}

// ListContainers walks the workspace dirs and returns containers that have
// a .worktrees.json (i.e. are wt-managed).
func ListContainers() []string {
	var out []string
	for _, base := range WorkspaceDirs() {
		matches, _ := filepath.Glob(filepath.Join(base, "*", ".worktrees.json"))
		for _, m := range matches {
			out = append(out, filepath.Dir(m))
		}
	}
	return out
}

// GitOutput runs git -C <dir> <args...> and returns trimmed stdout.
func GitOutput(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).Output()
	return strings.TrimSpace(string(out)), err
}

// gitError wraps an exec.ExitError with the captured stderr so callers can
// surface git's diagnostic messages (e.g. "fatal: ...") when wrapping the error.
type gitError struct {
	stderr string
	cause  error
}

func (e *gitError) Error() string { return e.stderr }
func (e *gitError) Unwrap() error { return e.cause }

// GitRun runs git -C <dir> <args...> forwarding stdout/stderr to the terminal.
// Stderr is also captured so that on failure the returned error contains the
// git diagnostic text rather than just "exit status N".
func GitRun(dir string, args ...string) error {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	var stderrBuf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderrBuf.String()); msg != "" {
			return &gitError{stderr: msg, cause: err}
		}
		return err
	}
	return nil
}

// GitCheck runs git -C <dir> <args...> silently and returns true when exit==0.
func GitCheck(dir string, args ...string) bool {
	full := append([]string{"-C", dir}, args...)
	return exec.Command("git", full...).Run() == nil
}

// IsDirty returns true when the worktree has staged/unstaged changes
// (untracked files are excluded — same as wt-tree-rm.sh).
func IsDirty(wtPath string) bool {
	out, err := GitOutput(wtPath, "status", "--porcelain")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "??") {
			return true
		}
	}
	return false
}
