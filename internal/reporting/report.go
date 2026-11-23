// Package reporting formats Sheriff findings into structured reports.
//
// A report separates the summary (how many packages were analyzed and how many
// findings were produced) from the findings themselves, and can be rendered as
// a human-readable text block or as JSON. The structure is intentionally stable
// so that the CLI, and later HTML output, can share it.
package reporting

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Finding is a single diagnostic produced by an analyzer.
type Finding struct {
	Analyzer string `json:"analyzer"`
	Position string `json:"position"`
	Message  string `json:"message"`
}

// Report is a structured collection of findings for one analysis run.
type Report struct {
	Packages int       `json:"packages"`
	Findings []Finding `json:"findings"`
}

// WriteText writes a human-readable structured report.
func (r Report) WriteText(w io.Writer) {
	fmt.Fprintln(w, "SHERIFF REPORT")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Packages analyzed:  %d\n", r.Packages)
	fmt.Fprintf(w, "Findings detected:  %d\n", len(r.Findings))

	if len(r.Findings) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No findings.")
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Findings:")
	for _, f := range r.Findings {
		fmt.Fprintf(w, "  [%s] %s: %s\n", f.Analyzer, f.Position, f.Message)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "By analyzer:")
	width := 0
	counts := make(map[string]int)
	for _, f := range r.Findings {
		counts[f.Analyzer]++
		if len(f.Analyzer) > width {
			width = len(f.Analyzer)
		}
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "  %-*s  %d\n", width, name, counts[name])
	}
}

// WriteJSON writes the report as indented JSON.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
