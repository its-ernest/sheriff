package analyzer

import (
	"testing"

	"golang.org/x/tools/go/analysis"
)

// TestAnalyzersAreValid guards the registry boundary: every registered
// analyzer must be well formed and usable by the official analysis driver.
func TestAnalyzersAreValid(t *testing.T) {
	registered := Analyzers()
	if len(registered) == 0 {
		t.Fatal("Analyzers() returned no analyzers")
	}

	if err := analysis.Validate(registered); err != nil {
		t.Fatalf("invalid analyzer registry: %v", err)
	}

	seen := make(map[string]struct{}, len(registered))
	for _, a := range registered {
		if _, exists := seen[a.Name]; exists {
			t.Errorf("duplicate analyzer name %q", a.Name)
		}
		seen[a.Name] = struct{}{}
	}
}
