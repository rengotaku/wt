// Package buildinfo exposes build- and process-time metadata used to detect
// whether the running binary is out of date relative to its source repository
// (see cmd/web.go and internal/handler/buildinfo.go).
//
// Commit / CommitTime / SourceRepo are populated by ldflags at build time
// (see Makefile). If ldflags are absent (e.g. `go build .` without -X),
// they stay empty and downstream code treats the freshness check as
// "unknown" (fail-closed: no stale badge).
package buildinfo

import (
	"strconv"
	"time"
)

// Commit is the git SHA of HEAD when the binary was built.
var Commit = ""

// CommitTime is the unix seconds (as string) of the build-time HEAD commit.
var CommitTime = ""

// SourceRepo is the absolute path of the git working tree the binary was
// built from. Empty when unknown.
var SourceRepo = ""

// StartTime is the process start time, captured at package init.
var StartTime = time.Now()

// ParsedCommitTime parses CommitTime as unix seconds. Returns zero time on
// error or when unset.
func ParsedCommitTime() time.Time {
	if CommitTime == "" {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(CommitTime, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
