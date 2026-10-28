package plan

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/giovani/readingdrip/internal/blocks"
)

// Format escreve o plano em formato textual estável, usado por golden tests e
// pelo CLI.
func Format(w io.Writer, docName string, bs []blocks.Block, pl *Plan, p Params) error {
	p = p.normalized()
	target, min, max, err := p.bounds(pl.TotalSeconds)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "# doc: %s  mode=%s  target=%ds  min=%ds  max=%ds\n",
		docName, pl.Mode, target, min, max); err != nil {
		return err
	}
	for _, aviso := range p.Warnings(pl.TargetSeconds, len(pl.Chunks)) {
		if _, err := fmt.Fprintf(w, "# aviso: %s\n", aviso); err != nil {
			return err
		}
	}

	for _, c := range pl.Chunks {
		line := fmt.Sprintf("%03d  [%d..%d]  %-7s %8s  %s  start=%d end=%d",
			c.Seq,
			c.FirstBlock,
			c.LastBlock,
			FormatDuration(c.Seconds),
			formatWords(c.Words)+"w",
			blocks.JoinPath(c.SectionPath),
			boolInt(c.StartsMidSection),
			boolInt(c.EndsMidSection),
		)
		if c.Oversized {
			line += "  oversized=1"
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(w, "# total: %d chunks, %s, coverage=ok\n",
		len(pl.Chunks), FormatDuration(pl.TotalSeconds))
	return err
}

// FormatDuration formata segundos como "42s", "9m12s" ou "100m03s".
func FormatDuration(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	m := seconds / 60
	s := seconds % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}

func formatWords(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
