// Package reporting formats Sheriff findings into structured reports.
//
// It defines the report model (a set of findings plus a package count) and
// renderers for human-readable text and JSON. Diagnostic presentation is kept
// separate from analysis and from the CLI so that future output formats (for
// example HTML) can reuse the same model.
package reporting
