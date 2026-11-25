// Package remediate applies conservative fixes to a source file based on
// Sheriff findings. The first capability fixes discarded errors in functions
// that return an error as their last result.
package remediate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"
)

// Finding is one diagnostic from the report.
type Finding struct {
	Analyzer string
	Position string
	Message  string
}

// Result summarizes a remediation run over one file.
type Result struct {
	Applied int
	Review  []string
}

type edit struct {
	start int
	end   int
	text  string
}

// FixFile applies discarded-error fixes to src. filename is used for parsing
// and messages. Only findings from the "discarded" analyzer are considered.
func FixFile(filename string, src []byte, findings []Finding) ([]byte, Result) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return src, Result{}
	}

	var edits []edit
	var review []string
	applied := 0

	for _, f := range findings {
		if f.Analyzer != "discarded" {
			continue
		}
		line, col, ok := parsePos(f.Position)
		if !ok {
			continue
		}
		assign, fn, blank := findEnclosing(fset, file, line, col)
		if assign == nil || fn == nil || blank == nil {
			review = append(review, fmt.Sprintf("%s: could not locate assignment; review", f.Position))
			continue
		}
		results := flattenResults(resultFields(fn))
		if len(results) == 0 || !isErrorExpr(results[len(results)-1]) {
			review = append(review, fmt.Sprintf("%s: enclosing function does not return an error as its last result; review", f.Position))
			continue
		}
		idx := 0
		for i, lhs := range assign.Lhs {
			if lhs == blank {
				idx = i
				break
			}
		}
		call := callText(assign, idx)
		indent := lineIndent(src, fset.Position(assign.Pos()).Line)
		start := fset.Position(blank.Pos()).Offset
		end := fset.Position(assign.End()).Offset
		edits = append(edits,
			edit{start: start, end: start + 1, text: "err"},
			edit{start: end, end: end, text: fmt.Sprintf("\n%sif err != nil {\n%s\treturn fmt.Errorf(\"%s: %%w\", err)\n%s}", indent, indent, call, indent)},
		)
		applied++
	}

	out := applyEdits(src, edits)
	if applied > 0 && !importsFmt(out) {
		out = addFmtImport(out)
	}
	return out, Result{Applied: applied, Review: review}
}

func parsePos(pos string) (int, int, bool) {
	i := strings.LastIndexByte(pos, ':')
	if i < 0 {
		return 0, 0, false
	}
	j := strings.LastIndexByte(pos[:i], ':')
	if j < 0 {
		return 0, 0, false
	}
	line, err1 := strconv.Atoi(pos[j+1 : i])
	col, err2 := strconv.Atoi(pos[i+1:])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return line, col, true
}

// findEnclosing returns the assignment with a blank at line:col, its enclosing
// function, and the blank identifier.
func findEnclosing(fset *token.FileSet, file *ast.File, line, col int) (*ast.AssignStmt, ast.Node, *ast.Ident) {
	var assign *ast.AssignStmt
	var fn ast.Node
	var blank *ast.Ident
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return true
		}
		if a, ok := n.(*ast.AssignStmt); ok && assign == nil {
			for _, lhs := range a.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name != "_" {
					continue
				}
				p := fset.Position(id.Pos())
				if p.Line == line && p.Column == col {
					assign = a
					blank = id
					for k := len(stack) - 1; k >= 0; k-- {
						switch stack[k].(type) {
						case *ast.FuncDecl, *ast.FuncLit:
							fn = stack[k]
						}
						if fn != nil {
							break
						}
					}
				}
			}
		}
		stack = append(stack, n)
		return true
	})
	return assign, fn, blank
}

func callText(assign *ast.AssignStmt, i int) string {
	if len(assign.Rhs) == 1 {
		return types.ExprString(assign.Rhs[0])
	}
	if i < len(assign.Rhs) {
		return types.ExprString(assign.Rhs[i])
	}
	return "operation"
}

func resultFields(fn ast.Node) *ast.FieldList {
	switch f := fn.(type) {
	case *ast.FuncDecl:
		return f.Type.Results
	case *ast.FuncLit:
		return f.Type.Results
	}
	return nil
}

func flattenResults(fl *ast.FieldList) []ast.Expr {
	var out []ast.Expr
	if fl == nil {
		return out
	}
	for _, field := range fl.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			out = append(out, field.Type)
		}
	}
	return out
}

func isErrorExpr(e ast.Expr) bool {
	id, ok := unparen(e).(*ast.Ident)
	return ok && id.Name == "error"
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

func lineIndent(src []byte, line int) string {
	start := lineStart(src, line)
	end := start
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	return string(src[start:end])
}

func lineStart(src []byte, line int) int {
	if line <= 1 {
		return 0
	}
	count := 1
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			count++
			if count == line {
				return i + 1
			}
		}
	}
	return 0
}

func importsFmt(src []byte) bool {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ImportsOnly)
	if err != nil {
		return false
	}
	for _, imp := range file.Imports {
		if imp.Path.Value == `"fmt"` {
			return true
		}
	}
	return false
}

func addFmtImport(src []byte) []byte {
	s := string(src)
	idx := strings.Index(s, "import")
	if idx < 0 {
		return src
	}
	rest := s[idx+len("import"):]
	trimmed := strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(trimmed, "(") {
		pos := idx + len("import") + (len(rest) - len(trimmed)) + 1
		return []byte(s[:pos] + "\n\t\"fmt\"" + s[pos:])
	}
	lineEnd := strings.IndexByte(rest, '\n')
	if lineEnd < 0 {
		lineEnd = len(rest)
	}
	return []byte(s[:idx] + "import (\n\t\"fmt\"\n\t" + strings.TrimSpace(rest[:lineEnd]) + "\n)" + s[idx+len("import")+lineEnd:])
}

func applyEdits(src []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := src
	for _, e := range edits {
		if e.start < 0 || e.end > len(out) || e.start > e.end {
			continue
		}
		next := append([]byte{}, out[:e.start]...)
		next = append(next, e.text...)
		next = append(next, out[e.end:]...)
		out = next
	}
	return out
}
