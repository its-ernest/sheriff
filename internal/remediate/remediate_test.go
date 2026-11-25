package remediate

import (
	"strings"
	"testing"
)

func TestFixDiscarded(t *testing.T) {
	src := `package p

import "encoding/json"

func f(user any) error {
	data, _ := json.Marshal(user)
	_ = data
	return nil
}
`
	findings := []Finding{{Analyzer: "discarded", Position: "p.go:6:8", Message: "discarded error"}}
	out, res := FixFile("p.go", []byte(src), findings)
	if res.Applied != 1 {
		t.Fatalf("applied = %d, want 1 (review: %v)", res.Applied, res.Review)
	}
	got := string(out)
	for _, want := range []string{
		`data, err := json.Marshal(user)`,
		`if err != nil {`,
		`fmt.Errorf("json.Marshal(user): %w", err)`,
		`"fmt"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fixed source missing %q\n%s", want, got)
		}
	}
}

func TestFixReviewNoErrorReturn(t *testing.T) {
	src := `package p

import "encoding/json"

func f(user any) {
	data, _ := json.Marshal(user)
	_ = data
}
`
	findings := []Finding{{Analyzer: "discarded", Position: "p.go:6:8", Message: "discarded error"}}
	_, res := FixFile("p.go", []byte(src), findings)
	if res.Applied != 0 || len(res.Review) != 1 {
		t.Fatalf("applied = %d, review = %v; want 0 applied and 1 review", res.Applied, res.Review)
	}
}

func TestFixReviewErrorNotLast(t *testing.T) {
	src := `package p

import "encoding/json"

func f(user any) (error, int) {
	data, _ := json.Marshal(user)
	_ = data
	return nil, 0
}
`
	findings := []Finding{{Analyzer: "discarded", Position: "p.go:6:8", Message: "discarded error"}}
	_, res := FixFile("p.go", []byte(src), findings)
	if res.Applied != 0 || len(res.Review) != 1 {
		t.Fatalf("applied = %d, review = %v; want 0 applied and 1 review", res.Applied, res.Review)
	}
}
