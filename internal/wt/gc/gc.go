// Package gc cross-repo garbage-collects worktrees by various filters.
package gc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wt/internal/wt/core"
	"wt/internal/wt/tree"
)

// Options captures CLI flags for `wt tree gc`.
type Options struct {
	Merged     bool
	OlderThan  string // "30d", "24h"
	NoTmux     bool
	DryRun     bool
	Yes        bool
	KeepTmux   bool
	KeepBranch bool
	Force      bool
}

var olderThanRegex = regexp.MustCompile(`^(\d+)([dh])$`)

// Run executes the cross-repo GC workflow.
func Run(out io.Writer, opts Options) error {
	olderSecs, err := parseOlderThan(opts.OlderThan)
	if err != nil {
		return err
	}

	items, err := tree.RmEntries()
	if err != nil {
		return err
	}

	type cand struct {
		entry  tree.RmEntry
		branch string
		info   string
	}
	var cands []cand

	prCache := map[string][]mergedPR{}
	if opts.Merged {
		_, _ = fmt.Fprintln(out, "🧹 マージ済み PR を取得中...")
		for i := range items {
			if _, ok := prCache[items[i].MainDir]; ok {
				continue
			}
			prCache[items[i].MainDir] = fetchMergedPRs(items[i].MainDir)
		}
	}

	for i := range items {
		it := &items[i]
		if it.WtName == "main" || it.WtName == "master" {
			continue
		}
		if core.IsDirty(it.WtPath) && !opts.Force {
			continue
		}

		branch, _ := core.GitOutput(it.WtPath, "branch", "--show-current")
		info := ""

		if opts.Merged {
			if branch == "" {
				continue
			}
			matched := false
			for _, pr := range prCache[it.MainDir] {
				if pr.HeadRefName != branch {
					continue
				}
				date := pr.MergedAt
				if len(date) > 10 {
					date = date[:10]
				}
				info += fmt.Sprintf("PR #%d merged %s", pr.Number, date)
				matched = true
				break
			}
			if !matched {
				continue
			}
		}

		if olderSecs > 0 {
			lastCT, _ := core.GitOutput(it.WtPath, "log", "-1", "--format=%ct")
			ct, _ := strconv.ParseInt(strings.TrimSpace(lastCT), 10, 64)
			if time.Since(time.Unix(ct, 0)).Seconds() <= float64(olderSecs) {
				continue
			}
			lastDate, _ := core.GitOutput(it.WtPath, "log", "-1", "--format=%ci")
			date := strings.TrimSpace(lastDate)
			if i := strings.Index(date, " "); i > 0 {
				date = date[:i]
			}
			if info != "" {
				info += "  "
			}
			info += "last commit " + date
		}

		if opts.NoTmux {
			if exec.Command("tmux", "has-session", "-t", it.WtName).Run() == nil {
				continue
			}
		}

		cands = append(cands, cand{entry: *it, branch: branch, info: info})
	}

	if len(cands) == 0 {
		_, _ = fmt.Fprintln(out, "条件に合う worktree はありません")
		return nil
	}

	display := func(c cand) string {
		brn := c.branch
		if brn != "" {
			brn += "  "
		}
		return fmt.Sprintf("%s/%s   %s%s", c.entry.Repo, c.entry.WtName, brn, c.info)
	}

	if opts.DryRun || !opts.Yes {
		_, _ = fmt.Fprintf(out, "🔍 %d 件の候補:\n", len(cands))
		for i := range cands {
			_, _ = fmt.Fprintln(out, display(cands[i]))
		}
		if !opts.Yes && !opts.DryRun {
			_, _ = fmt.Fprintln(out, "実行するには --yes を指定してください")
		}
		return nil
	}

	rmOpts := tree.RmOptions{
		KeepTmux:   opts.KeepTmux,
		KeepBranch: opts.KeepBranch,
		Force:      opts.Force,
	}
	for i := range cands {
		c := &cands[i]
		if c.branch == "" {
			_, _ = fmt.Fprintf(out, "⚠️  ブランチ不明のためスキップ: %s\n", c.entry.WtPath)
			continue
		}
		iterOpts := rmOpts
		iterOpts.Branch = c.branch
		iterOpts.Repo = c.entry.Repo
		_ = tree.Rm(out, iterOpts)
	}
	return nil
}

func parseOlderThan(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	m := olderThanRegex.FindStringSubmatch(s)
	if m == nil {
		return 0, errors.New("--older-than の形式が不正（例: 30d, 24h）: " + s)
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	switch m[2] {
	case "d":
		return n * 86400, nil
	case "h":
		return n * 3600, nil
	}
	return 0, errors.New("--older-than の単位は d または h")
}

type mergedPR struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	MergedAt    string `json:"mergedAt"`
}

func fetchMergedPRs(mainDir string) []mergedPR {
	remote, err := core.GitOutput(mainDir, "remote", "get-url", "origin")
	if err != nil {
		return nil
	}
	slug := remote
	if i := strings.Index(slug, "github.com"); i >= 0 {
		slug = slug[i+len("github.com"):]
		if slug != "" && (slug[0] == ':' || slug[0] == '/') {
			slug = slug[1:]
		}
	}
	slug = strings.TrimSuffix(slug, ".git")
	if slug == "" {
		return nil
	}
	out, err := exec.Command("gh", "pr", "list",
		"--state", "merged",
		"--json", "number,headRefName,mergedAt",
		"--limit", "200",
		"--repo", slug).Output()
	if err != nil {
		return nil
	}
	var prs []mergedPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil
	}
	return prs
}
