package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fevzisahinler/kxdiff/internal/model"
)

// viewOptions controls how much of the text report is printed.
type viewOptions struct {
	quiet    bool
	onlyFrom bool
	onlyTo   bool
	onlyDiff bool
}

// sections reports which output sections to show. With no --only-* flag all are
// shown; otherwise only the selected ones.
func (v viewOptions) sections() (from, to, diff bool) {
	if !v.onlyFrom && !v.onlyTo && !v.onlyDiff {
		return true, true, true
	}
	return v.onlyFrom, v.onlyTo, v.onlyDiff
}

// renderReport renders the report in the requested format.
func renderReport(out io.Writer, p palette, v viewOptions, output string, r model.DiffReport) error {
	switch output {
	case outputJSON:
		return printJSON(out, r)
	case outputMarkdown:
		return printMarkdown(out, r)
	default:
		return printText(out, p, v, r)
	}
}

// --- text -------------------------------------------------------------------

func printText(out io.Writer, p palette, v viewOptions, r model.DiffReport) error {
	lw := &lineWriter{w: out}

	lw.printf("%s %s  <->  %s\n", p.bold("ENVIRONMENTS:"), r.From, r.To)
	lw.printf("only in %s: %d | only in %s: %d | differs: %d | same: %d\n",
		r.From, len(r.OnlyFrom), r.To, len(r.OnlyTo), len(r.Differs), r.Same)

	for _, w := range r.Warnings {
		lw.printf("  warning %s\n", w)
	}

	showFrom, showTo, showDiff := v.sections()
	if showFrom {
		printBucket(lw, p, "only in "+r.From, refStrings(r.OnlyFrom), p.red)
	}
	if showTo {
		printBucket(lw, p, "only in "+r.To, refStrings(r.OnlyTo), p.green)
	}
	if showDiff {
		lw.printf("\n%s\n", p.bold("differs:"))
		if len(r.Differs) == 0 {
			lw.printf("  (none)\n")
		}
		for _, d := range r.Differs {
			lw.printf("  %s\n", p.yellow(d.Kind+"/"+d.Name))
			for _, f := range d.Fields {
				printField(lw, p, f)
			}
		}
	}
	return lw.err
}

func printBucket(lw *lineWriter, p palette, title string, items []string, color func(string) string) {
	lw.printf("\n%s\n", p.bold(title+":"))
	if len(items) == 0 {
		lw.printf("  (none)\n")
		return
	}
	for _, it := range items {
		lw.printf("  %s\n", color(it))
	}
}

// printField prints one field diff. Single-line values are shown inline; a
// multi-line value is shown as a line block with only the changed lines.
func printField(lw *lineWriter, p palette, f model.FieldDiff) {
	if isMultilineValue(f.From) || isMultilineValue(f.To) {
		lw.printf("      %s:\n", f.Path)
		removed, added := lineDiff(asString(f.From), asString(f.To))
		for _, line := range removed {
			lw.printf("        %s\n", p.red("- "+line))
		}
		for _, line := range added {
			lw.printf("        %s\n", p.green("+ "+line))
		}
		return
	}
	lw.printf("      %s  %s → %s\n", f.Path, p.red(displayValue(f.From)), p.green(displayValue(f.To)))
}

func refStrings(refs []model.ResourceRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Kind+"/"+ref.Name)
	}
	return out
}

// --- json -------------------------------------------------------------------

func printJSON(out io.Writer, r model.DiffReport) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// --- markdown ---------------------------------------------------------------

func printMarkdown(out io.Writer, r model.DiffReport) error {
	lw := &lineWriter{w: out}

	lw.printf("# kxdiff: %s → %s\n\n", r.From, r.To)
	lw.printf("only in `%s`: %d · only in `%s`: %d · differs: %d · same: %d\n\n",
		r.From, len(r.OnlyFrom), r.To, len(r.OnlyTo), len(r.Differs), r.Same)

	markdownRefs(lw, "Only in "+r.From, r.OnlyFrom)
	markdownRefs(lw, "Only in "+r.To, r.OnlyTo)

	if len(r.Differs) > 0 {
		lw.printf("## Differs\n\n")
		for _, d := range r.Differs {
			lw.printf("### %s/%s\n\n", d.Kind, d.Name)
			for _, f := range d.Fields {
				markdownField(lw, f)
			}
			lw.printf("\n")
		}
	}
	return lw.err
}

func markdownRefs(lw *lineWriter, title string, refs []model.ResourceRef) {
	if len(refs) == 0 {
		return
	}
	lw.printf("## %s\n\n", title)
	for _, ref := range refs {
		lw.printf("- `%s/%s`\n", ref.Kind, ref.Name)
	}
	lw.printf("\n")
}

func markdownField(lw *lineWriter, f model.FieldDiff) {
	if isMultilineValue(f.From) || isMultilineValue(f.To) {
		lw.printf("- `%s`:\n\n```diff\n", f.Path)
		removed, added := lineDiff(asString(f.From), asString(f.To))
		for _, line := range removed {
			lw.printf("- %s\n", line)
		}
		for _, line := range added {
			lw.printf("+ %s\n", line)
		}
		lw.printf("```\n")
		return
	}
	lw.printf("- `%s`: `%s` → `%s`\n", f.Path, displayValue(f.From), displayValue(f.To))
}

// displayValue formats a raw value for text/markdown: scalars as-is, scalar
// lists as "[a, b]", maps/nested as compact JSON, and an absent (nil) value as
// "<none>".
func displayValue(v any) string {
	switch t := v.(type) {
	case nil:
		return absent
	case map[string]any:
		return jsonCompact(v)
	case []any:
		return displayList(t)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func displayList(list []any) string {
	parts := make([]string, len(list))
	for i, el := range list {
		switch el.(type) {
		case map[string]any, []any:
			parts[i] = jsonCompact(el)
		default:
			parts[i] = fmt.Sprintf("%v", el)
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func jsonCompact(v any) string {
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

func isMultilineValue(v any) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, "\n")
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

const absent = "<none>"

// --- helpers ----------------------------------------------------------------

// lineDiff returns the lines present only in from (removed) and only in to
// (added), preserving order; lines common to both are omitted.
func lineDiff(from, to string) (removed, added []string) {
	fromLines := strings.Split(from, "\n")
	toLines := strings.Split(to, "\n")
	inFrom, inTo := lineSet(fromLines), lineSet(toLines)
	for _, l := range fromLines {
		if !inTo[l] {
			removed = append(removed, l)
		}
	}
	for _, l := range toLines {
		if !inFrom[l] {
			added = append(added, l)
		}
	}
	return removed, added
}

func lineSet(lines []string) map[string]bool {
	set := make(map[string]bool, len(lines))
	for _, l := range lines {
		set[l] = true
	}
	return set
}

// lineWriter writes formatted lines to w, remembering the first write error so
// callers can check it once at the end.
type lineWriter struct {
	w   io.Writer
	err error
}

func (lw *lineWriter) printf(format string, args ...any) {
	if lw.err == nil {
		_, lw.err = fmt.Fprintf(lw.w, format, args...)
	}
}

// palette renders ANSI colours, or plain text when disabled.
type palette struct{ enabled bool }

func (p palette) red(s string) string    { return p.wrap("31", s) }
func (p palette) green(s string) string  { return p.wrap("32", s) }
func (p palette) yellow(s string) string { return p.wrap("33", s) }
func (p palette) bold(s string) string   { return p.wrap("1", s) }

func (p palette) wrap(code, s string) string {
	if !p.enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// useColor decides whether to colour output: off when --no-color or NO_COLOR is
// set, or when stdout is not a terminal (piped / redirected).
func useColor(noColor bool) bool {
	if noColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTerminal(os.Stdout)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
