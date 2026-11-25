package config

import (
	"strings"
	"testing"
)

const sample = `
[project]
name = "myapp"

[analysis]
dependencies = true
vet = true
tests = true
races = true
vulnerabilities = true

[remediation]
dependencies = false
modules = [
    ".",
    "all"
]

[report]
html = true
json = true
output = ".sheriff/reports"
`

func TestParse(t *testing.T) {
	cfg, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project.Name != "myapp" {
		t.Errorf("name = %q", cfg.Project.Name)
	}
	if !cfg.Analysis.Dependencies || !cfg.Analysis.Vet || !cfg.Analysis.Tests || !cfg.Analysis.Races || !cfg.Analysis.Vulnerabilities {
		t.Errorf("analysis = %+v", cfg.Analysis)
	}
	if cfg.Remediation.Dependencies {
		t.Error("remediation.dependencies should be false")
	}
	if len(cfg.Remediation.Modules) != 2 || cfg.Remediation.Modules[0] != "." || cfg.Remediation.Modules[1] != "all" {
		t.Errorf("modules = %v", cfg.Remediation.Modules)
	}
	if !cfg.Report.HTML || !cfg.Report.JSON {
		t.Errorf("report = %+v", cfg.Report)
	}
	if cfg.Report.Output != ".sheriff/reports" {
		t.Errorf("output = %q", cfg.Report.Output)
	}
}

func TestParseCommentsAndDefaults(t *testing.T) {
	cfg, err := Parse(strings.NewReader("; a comment\n[report]\nhtml = true # inline\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Report.HTML {
		t.Error("html should be true")
	}
	if cfg.Report.Output != "." {
		t.Errorf("default output = %q", cfg.Report.Output)
	}
}

func TestParseArrayMultiline(t *testing.T) {
	cfg, err := Parse(strings.NewReader("[remediation]\nmodules = [\n  \"a\",\n  \"b@v1.0.0\",\n]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Remediation.Modules) != 2 || cfg.Remediation.Modules[1] != "b@v1.0.0" {
		t.Errorf("modules = %v", cfg.Remediation.Modules)
	}
}

func TestParseBuild(t *testing.T) {
	cfg, err := Parse(strings.NewReader("[project]\nname = \"myapp\"\n\n[build]\noutput = \"myapp\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Build.Output != "myapp" {
		t.Errorf("build.output = %q", cfg.Build.Output)
	}
	if cfg.Project.Name != "myapp" {
		t.Errorf("project.name = %q", cfg.Project.Name)
	}
}
