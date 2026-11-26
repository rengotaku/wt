// Package out renders the output of wt's read commands in the three shared
// formats: the default tab-separated form (easy for AI and scripts), --json,
// and --rich (formatted for people).
package out

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/cli/clierr"
)

// Opts holds the format flags of one command.
type Opts struct {
	JSON bool
	Rich bool
}

// AddFlags registers --json and --rich on c.
func AddFlags(c *cobra.Command) *Opts {
	o := &Opts{}
	c.Flags().BoolVar(&o.JSON, "json", false, "JSON で出力する")
	c.Flags().BoolVar(&o.Rich, "rich", false, "人が読む表示にする（罫線・色分け。色は端末のときだけ）")
	return o
}

// Validate rejects contradictory flags.
func (o *Opts) Validate() error {
	if o.JSON && o.Rich {
		return clierr.Usagef("--json と --rich は同時に指定できません")
	}
	return nil
}

// Table is tabular output. Meta lines are printed before the header as
// "# key=value" in the default format.
type Table struct {
	Meta   [][2]string
	Header []string
	Rows   [][]string
}

// Render writes t in the format chosen by o. data is what --json encodes;
// title heads the --rich view (may be empty).
func Render(w io.Writer, o *Opts, t *Table, data any, title string) error {
	switch {
	case o.JSON:
		return WriteJSON(w, data)
	case o.Rich:
		return WriteRich(w, t, title, IsTerminal(w))
	default:
		return WriteTSV(w, t)
	}
}

// WriteTSV writes the default format: "# key=value" meta lines, a header,
// then one tab-separated row per item. Tabs and newlines inside values are
// replaced by spaces and empty values are written as "-".
func WriteTSV(w io.Writer, t *Table) error {
	var b strings.Builder
	for _, m := range t.Meta {
		b.WriteString("# " + m[0] + "=" + Cell(m[1]) + "\n")
	}
	b.WriteString(strings.Join(t.Header, "\t") + "\n")
	for _, r := range t.Rows {
		cells := make([]string, len(r))
		for i, c := range r {
			cells[i] = Cell(c)
		}
		b.WriteString(strings.Join(cells, "\t") + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// Cell keeps a value on one cell: tabs and newlines become spaces, other
// control characters (ESC etc., which could rewrite the terminal) are
// dropped, and an empty value is "-".
func Cell(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r < 0x20 || (r >= 0x7f && r < 0xa0):
			return -1
		}
		return r
	}, s)
	if s == "" {
		return "-"
	}
	return s
}

// WriteJSON writes v as indented JSON.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// IsTerminal reports whether w is a character device.
func IsTerminal(w any) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// OnOff renders a bool as "on" / "off".
func OnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Itoa renders 0 as "" (written as "-") and other ints in decimal.
func Itoa(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}
