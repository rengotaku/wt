package out

import (
	"bytes"
	"strings"
	"testing"
)

func sampleTable() *Table {
	return &Table{
		Meta:   [][2]string{{"daemon", "running"}},
		Header: []string{"REPO", "STATE", "URL"},
		Rows:   [][]string{{"app", "running", "http://x"}, {"lib", "idle", ""}, {"日本", "a\tb", "c\nd"}},
	}
}

func TestWriteTSV(t *testing.T) {
	var b bytes.Buffer
	if err := WriteTSV(&b, sampleTable()); err != nil {
		t.Fatal(err)
	}
	want := "# daemon=running\nREPO\tSTATE\tURL\napp\trunning\thttp://x\nlib\tidle\t-\n日本\ta b\tc d\n"
	if b.String() != want {
		t.Errorf("got\n%q\nwant\n%q", b.String(), want)
	}
}

func TestWriteRich(t *testing.T) {
	var plain bytes.Buffer
	if err := WriteRich(&plain, sampleTable(), "title", false); err != nil {
		t.Fatal(err)
	}
	s := plain.String()
	if strings.Contains(s, "\x1b[") {
		t.Error("color=false must not emit ANSI codes")
	}
	for _, want := range []string{"title\n", "daemon: running", "┌", "│ app ", "└"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	var widths []int
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "│") || strings.HasPrefix(l, "┌") || strings.HasPrefix(l, "└") {
			widths = append(widths, DisplayWidth(l))
		}
	}
	for _, w := range widths {
		if w != widths[0] {
			t.Fatalf("ragged rows (wide characters must count as 2): %v", widths)
		}
	}

	var colored bytes.Buffer
	_ = WriteRich(&colored, sampleTable(), "", true)
	if !strings.Contains(colored.String(), Green+"running"+Reset) {
		t.Error("color=true must color known states")
	}
}

func TestRender(t *testing.T) {
	data := map[string]int{"n": 1}
	for _, c := range []struct {
		o    Opts
		want string
	}{
		{Opts{}, "REPO\tSTATE\tURL"},
		{Opts{JSON: true}, `"n": 1`},
		{Opts{Rich: true}, "┌"},
	} {
		var b bytes.Buffer
		if err := Render(&b, &c.o, sampleTable(), data, ""); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), c.want) {
			t.Errorf("%+v: missing %q in %q", c.o, c.want, b.String())
		}
	}
	both := Opts{JSON: true, Rich: true}
	if err := both.Validate(); err == nil {
		t.Error("--json with --rich: want error")
	}
}

func TestCell_DropsControlCharacters(t *testing.T) {
	if got := Cell("api\x1b[2J\x07x\x7f"); got != "api[2Jx" {
		t.Errorf("got %q", got)
	}
}
