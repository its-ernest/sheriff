// Package config loads Sheriff's configuration file.
//
// The format is INI (a .toml structure is planned later). The first supported
// section is [report], which selects the output formats and directory. The
// other sections are parsed and exposed for upcoming features but do not yet
// change behaviour.
package config

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Config is the parsed configuration.
type Config struct {
	Project     Project
	Analysis    Analysis
	Build       Build
	Remediation Remediation
	Report      Report
}

type Project struct {
	Name string
}

// Build selects the output binary produced by `sheriff build`.
type Build struct {
	Output string
	Mode   string
}

type Analysis struct {
	Dependencies    bool
	Vet             bool
	Tests           bool
	Races           bool
	Vulnerabilities bool
}

type Remediation struct {
	Dependencies bool
	Modules      []string
}

type Report struct {
	HTML   bool
	JSON   bool
	Output string
}

// Default is the configuration used when no file is present.
func Default() Config {
	return Config{Report: Report{Output: "."}}
}

// Load reads and parses the configuration file at path.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads an INI document.
func Parse(r io.Reader) (Config, error) {
	cfg := Default()
	section := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := stripInlineComment(strings.TrimSpace(sc.Text()))
		if line == "" {
			continue
		}
		if line[0] == '[' && line[len(line)-1] == ']' {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if strings.HasPrefix(val, "[") && !strings.Contains(val, "]") {
			for sc.Scan() {
				next := stripInlineComment(strings.TrimSpace(sc.Text()))
				val += "\n" + next
				if strings.Contains(next, "]") {
					break
				}
			}
		}
		cfg.set(section, key, val)
	}
	return cfg, sc.Err()
}

func (c *Config) set(section, key, val string) {
	switch section {
	case "project":
		if key == "name" {
			c.Project.Name = unquote(val)
		}
	case "analysis":
		switch key {
		case "dependencies":
			c.Analysis.Dependencies = parseBool(val)
		case "vet":
			c.Analysis.Vet = parseBool(val)
		case "tests":
			c.Analysis.Tests = parseBool(val)
		case "races":
			c.Analysis.Races = parseBool(val)
		case "vulnerabilities":
			c.Analysis.Vulnerabilities = parseBool(val)
		}
	case "remediation":
		switch key {
		case "dependencies":
			c.Remediation.Dependencies = parseBool(val)
		case "modules":
			c.Remediation.Modules = parseArray(val)
		}
	case "build":
		switch key {
		case "output":
			c.Build.Output = unquote(val)
		case "mode":
			c.Build.Mode = unquote(val)
		}
	case "report":
		switch key {
		case "html":
			c.Report.HTML = parseBool(val)
		case "json":
			c.Report.JSON = parseBool(val)
		case "output":
			c.Report.Output = unquote(val)
		}
	}
}

// stripInlineComment removes a trailing "#"/";" comment that is not inside
// quotes.
func stripInlineComment(s string) string {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case '#', ';':
			if !inQuote {
				return strings.TrimSpace(s[:i])
			}
		}
	}
	return strings.TrimSpace(s)
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, unquote(part))
	}
	return out
}
