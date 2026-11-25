// Package discarded reports errors that are discarded with the blank
// identifier, for example:
//
//	data, _ := json.Marshal(user)
//
// The analyzer is the basis for remediation: it reports the position of the
// discarded error so a remediator can decide whether the enclosing function
// returns an error and, if so, propose a fix.
package discarded

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports discarded errors.
var Analyzer = &analysis.Analyzer{
	Name: "discarded",
	Doc:  "report errors discarded with the blank identifier",
	Run:  run,
}

var errorType = types.Universe.Lookup("error").Type()

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok {
				checkAssign(pass, assign)
			}
			return true
		})
	}
	return nil, nil
}

func checkAssign(pass *analysis.Pass, assign *ast.AssignStmt) {
	types := resultTypes(pass, assign)
	if len(types) == 0 {
		return
	}
	for i, lhs := range assign.Lhs {
		if i >= len(types) {
			break
		}
		if isBlank(lhs) && isError(types[i]) {
			pass.Reportf(lhs.Pos(), "discarded error: %s", callText(assign, i))
		}
	}
}

// resultTypes returns the type of each value produced by the right-hand side,
// aligned with the left-hand side.
func resultTypes(pass *analysis.Pass, assign *ast.AssignStmt) []types.Type {
	if len(assign.Rhs) == len(assign.Lhs) {
		out := make([]types.Type, len(assign.Rhs))
		for i, r := range assign.Rhs {
			out[i] = pass.TypesInfo.TypeOf(r)
		}
		return out
	}
	if len(assign.Rhs) == 1 {
		if tup, ok := pass.TypesInfo.TypeOf(assign.Rhs[0]).(*types.Tuple); ok {
			out := make([]types.Type, tup.Len())
			for i := 0; i < tup.Len(); i++ {
				out[i] = tup.At(i).Type()
			}
			return out
		}
	}
	return nil
}

// callText renders the right-hand side expression for the value at index i.
func callText(assign *ast.AssignStmt, i int) string {
	if len(assign.Rhs) == 1 {
		return types.ExprString(assign.Rhs[0])
	}
	if i < len(assign.Rhs) {
		return types.ExprString(assign.Rhs[i])
	}
	return ""
}

func isBlank(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "_"
}

func isError(t types.Type) bool {
	if t == nil {
		return false
	}
	iface, ok := errorType.Underlying().(*types.Interface)
	return ok && types.Implements(t, iface)
}
