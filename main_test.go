package main

import "testing"

func TestWantsJSON(t *testing.T) {
	cases := map[string]struct {
		args []string
		want bool
	}{
		"none":       {[]string{"repo", "ls"}, false},
		"flag":       {[]string{"repo", "ls", "--json"}, true},
		"eq true":    {[]string{"repo", "ls", "--json=true"}, true},
		"eq 1":       {[]string{"--json=1"}, true},
		"eq false":   {[]string{"--json=false"}, false},
		"after --":   {[]string{"x", "--", "--json"}, false},
		"last wins":  {[]string{"--json", "--json=false"}, false},
		"other flag": {[]string{"--jsonx"}, false},
	}
	for name, c := range cases {
		if got := wantsJSON(c.args); got != c.want {
			t.Errorf("%s: wantsJSON(%v) = %v, want %v", name, c.args, got, c.want)
		}
	}
}
