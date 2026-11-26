// Package clierr defines the error kinds every wt command reports and the
// exit code each maps to, so callers can branch on the code without parsing
// messages.
package clierr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Kind classifies an error. Its exit code is stable.
type Kind string

// Error kinds and their exit codes (see ExitCode).
const (
	KindInternal     Kind = "internal"     // 1: unexpected error
	KindUsage        Kind = "usage"        // 2: bad arguments or flags
	KindNotFound     Kind = "not_found"    // 3: the target does not exist
	KindPrecondition Kind = "precondition" // 4: state forbids it (dirty, already exists, ...)
	KindExternal     Kind = "external"     // 5: git / gh / lsof failed
)

var exitCodes = map[Kind]int{
	KindInternal:     1,
	KindUsage:        2,
	KindNotFound:     3,
	KindPrecondition: 4,
	KindExternal:     5,
}

// Error is a classified error.
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

func newf(k Kind, format string, a ...any) *Error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, a...)}
}

// Usagef reports bad arguments or flags (exit 2).
func Usagef(format string, a ...any) error { return newf(KindUsage, format, a...) }

// NotFoundf reports a missing target (exit 3).
func NotFoundf(format string, a ...any) error { return newf(KindNotFound, format, a...) }

// Preconditionf reports a state that forbids the operation (exit 4).
func Preconditionf(format string, a ...any) error { return newf(KindPrecondition, format, a...) }

// External wraps a failed external command (exit 5).
func External(msg string, err error) error { return &Error{Kind: KindExternal, Msg: msg, Err: err} }

// KindOf returns the kind of err. Unclassified errors from cobra's own
// argument and flag parsing count as usage errors; everything else is
// internal.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	msg := err.Error()
	for _, p := range []string{"unknown command", "unknown flag", "unknown shorthand flag", "accepts ", "requires at least", "requires at most", "invalid argument", "flag needs an argument"} {
		if strings.Contains(msg, p) {
			return KindUsage
		}
	}
	return KindInternal
}

// ExitCode maps err to the process exit code (0 for nil).
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	return exitCodes[KindOf(err)]
}

// Write prints err for the user: one "Error: ..." line, or a JSON object
// {"error":{"code":...,"message":...}} when asJSON.
func Write(w io.Writer, err error, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": string(KindOf(err)), "message": err.Error()},
		})
		return
	}
	_, _ = fmt.Fprintln(w, "Error: "+err.Error())
}
