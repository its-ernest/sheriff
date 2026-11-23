package reporting

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReportText(t *testing.T) {
	r := Report{
		Packages: 2,
		Findings: []Finding{
			{Analyzer: "nilptr", Position: "main.go:10:2", Message: "nil pointer dereference: p is nil"},
			{Analyzer: "bounds", Position: "main.go:20:3", Message: "index out of range: index 3 for length 3"},
		},
	}
	var buf bytes.Buffer
	r.WriteText(&buf)
	out := buf.String()
	for _, want := range []string{
		"SHERIFF REPORT",
		"Packages analyzed:  2",
		"Findings detected:  2",
		"[nilptr] main.go:10:2: nil pointer dereference: p is nil",
		"[bounds] main.go:20:3: index out of range: index 3 for length 3",
		"By analyzer:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text report missing %q\n%s", want, out)
		}
	}
}

func TestReportEmpty(t *testing.T) {
	var buf bytes.Buffer
	Report{Packages: 3}.WriteText(&buf)
	if !strings.Contains(buf.String(), "No findings.") {
		t.Fatalf("empty report missing notice: %s", buf.String())
	}
}

func TestReportJSON(t *testing.T) {
	r := Report{
		Packages: 1,
		Findings: []Finding{{Analyzer: "nilptr", Position: "main.go:1:1", Message: "m"}},
	}
	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var got Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Packages != 1 || len(got.Findings) != 1 || got.Findings[0].Analyzer != "nilptr" {
		t.Fatalf("unexpected report: %+v", got)
	}
}
