// Package analyzer owns Sheriff's analyzer registry.
//
// The registry is the single place where analysis rules are wired into the
// command. Implementations live in their own packages so this package remains
// a coordination point rather than a home for analysis logic.
package analyzer

import (
	"golang.org/x/tools/go/analysis"

	"github.com/its-ernest/sheriff/internal/analyzer/assert"
	"github.com/its-ernest/sheriff/internal/analyzer/bounds"
	"github.com/its-ernest/sheriff/internal/analyzer/divzero"
	"github.com/its-ernest/sheriff/internal/analyzer/nilptr"
)

// Analyzers returns the analyzers Sheriff runs.
func Analyzers() []*analysis.Analyzer {
	return analyzers
}

var analyzers = []*analysis.Analyzer{
	nilptr.Analyzer,
	divzero.Analyzer,
	bounds.Analyzer,
	assert.Analyzer,
}
