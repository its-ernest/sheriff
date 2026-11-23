package reporting

import (
	"fmt"
	"html"
	"io"
	"net/url"
	"sort"
	"strings"
)

// WriteHTML writes a self-contained, interactive HTML report. The page has no
// external dependencies: styles and a small filter script are embedded, so the
// file can be opened directly in a browser or shared as a single artifact.
func (r Report) WriteHTML(w io.Writer) error {
	groups := make(map[string][]Finding)
	var order []string
	for _, f := range r.Findings {
		if _, ok := groups[f.Analyzer]; !ok {
			order = append(order, f.Analyzer)
		}
		groups[f.Analyzer] = append(groups[f.Analyzer], f)
	}
	sort.Strings(order)

	var b strings.Builder
	b.WriteString(htmlHead)
	fmt.Fprintf(&b, "<header>\n  <h1>Sheriff Report</h1>\n")
	fmt.Fprintf(&b, "  <p class=\"summary\">Packages analyzed: %d &middot; Findings detected: %d</p>\n", r.Packages, len(r.Findings))
	b.WriteString("  <input id=\"filter\" type=\"search\" placeholder=\"Filter findings…\" autocomplete=\"off\">\n</header>\n<main>\n")

	if len(r.Findings) == 0 {
		b.WriteString("  <div class=\"empty\">No findings.</div>\n")
	}

	for _, name := range order {
		fmt.Fprintf(&b, "  <details open>\n    <summary>%s <span class=\"count\">(%d)</span></summary>\n    <ul class=\"findings\">\n", html.EscapeString(name), len(groups[name]))
		for _, f := range groups[name] {
			file, _ := splitPos(f.Position)
			search := strings.ToLower(name + " " + f.Position + " " + f.Message)
			b.WriteString("      <li data-search=\"" + html.EscapeString(search) + "\">\n")
			if file != "" {
				u := (&url.URL{Scheme: "file", Path: file}).String()
				fmt.Fprintf(&b, "        <code class=\"loc\"><a href=\"%s\">%s</a></code>\n", html.EscapeString(u), html.EscapeString(f.Position))
			} else {
				fmt.Fprintf(&b, "        <code class=\"loc\">%s</code>\n", html.EscapeString(f.Position))
			}
			fmt.Fprintf(&b, "        <span class=\"msg\">%s</span>\n", html.EscapeString(f.Message))
			b.WriteString("      </li>\n")
		}
		b.WriteString("    </ul>\n  </details>\n")
	}

	b.WriteString("</main>\n")
	b.WriteString(htmlScript)
	b.WriteString("</body>\n</html>\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// splitPos splits a "file:line:col" position into its file and the rest.
func splitPos(pos string) (file, linecol string) {
	i := strings.LastIndexByte(pos, ':')
	if i < 0 {
		return pos, ""
	}
	if j := strings.LastIndexByte(pos[:i], ':'); j >= 0 {
		return pos[:j], pos[j+1:]
	}
	return pos[:i], pos[i+1:]
}

const htmlHead = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sheriff Report</title>
<style>
:root { color-scheme: light dark; }
* { box-sizing: border-box; }
body { margin: 0; font-family: system-ui, -apple-system, "Segoe UI", sans-serif; background: #f6f7f9; color: #1b1b1f; }
header { position: sticky; top: 0; z-index: 1; background: #ffffff; border-bottom: 1px solid #e3e5e8; padding: 16px 24px; }
h1 { margin: 0 0 4px; font-size: 20px; }
.summary { margin: 0; color: #5a5f66; font-size: 14px; }
#filter { margin-top: 12px; width: 100%; max-width: 440px; padding: 8px 12px; border: 1px solid #c9ccd1; border-radius: 8px; font-size: 14px; background: #fff; color: inherit; }
main { max-width: 920px; margin: 0 auto; padding: 24px; }
details { background: #fff; border: 1px solid #e3e5e8; border-radius: 10px; margin-bottom: 12px; padding: 2px 16px; }
summary { cursor: pointer; font-weight: 600; padding: 12px 0; }
.count { color: #6b7280; font-weight: 400; }
ul.findings { list-style: none; margin: 0; padding: 0 0 8px; }
li { padding: 10px 0; border-top: 1px solid #eef0f2; display: flex; flex-direction: column; gap: 2px; }
code.loc { font-family: ui-monospace, "SFMono-Regular", Menlo, monospace; font-size: 13px; }
code.loc a { color: #0b62c4; text-decoration: none; }
code.loc a:hover { text-decoration: underline; }
.msg { font-size: 14px; }
.empty { background: #fff; border: 1px solid #e3e5e8; border-radius: 10px; padding: 24px; text-align: center; color: #5a5f66; }
@media (prefers-color-scheme: dark) {
	body { background: #16181c; color: #e8eaed; }
	header, details, .empty, #filter { background: #1f2227; border-color: #2c3038; }
	.summary, .count, .empty { color: #9aa0a6; }
	li { border-top-color: #2c3038; }
	code.loc a { color: #6cb0ff; }
}
</style>
</head>
<body>
`

const htmlScript = `<script>
const input = document.getElementById('filter');
input.addEventListener('input', () => {
  const q = input.value.toLowerCase();
  document.querySelectorAll('li[data-search]').forEach((li) => {
    li.style.display = li.dataset.search.includes(q) ? '' : 'none';
  });
  document.querySelectorAll('details').forEach((d) => {
    const visible = [...d.querySelectorAll('li[data-search]')].some((li) => li.style.display !== 'none');
    d.style.display = visible ? '' : 'none';
  });
});
</script>
`
