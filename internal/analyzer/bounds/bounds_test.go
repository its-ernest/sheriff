package bounds

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata, err := filepath.Abs("../../../testdata")
	if err != nil {
		t.Fatalf("resolve testdata directory: %v", err)
	}
	analysistest.Run(t, testdata, Analyzer, "bounds")
}
