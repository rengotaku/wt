package clierr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{errors.New("boom"), 1},
		{Usagef("bad"), 2},
		{NotFoundf("x"), 3},
		{Preconditionf("dirty"), 4},
		{External("git failed", errors.New("exit 128")), 5},
		{fmt.Errorf("wrapped: %w", NotFoundf("x")), 3},
		{errors.New(`unknown command "nope" for "wt"`), 2},
		{errors.New("unknown flag: --nope"), 2},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestWrite(t *testing.T) {
	var text bytes.Buffer
	Write(&text, NotFoundf("worktree が見つかりません: a/b"), false)
	if text.String() != "Error: worktree が見つかりません: a/b\n" {
		t.Errorf("text: %q", text.String())
	}

	var js bytes.Buffer
	Write(&js, NotFoundf("missing"), true)
	var got struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(js.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", js.String(), err)
	}
	if got.Error.Code != "not_found" || got.Error.Message != "missing" {
		t.Errorf("json: %+v", got)
	}
}
