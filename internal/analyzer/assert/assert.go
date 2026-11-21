// Package assert reports type assertions that provably cannot succeed.
//
// The analysis is intraprocedural and conservative. It tracks local interface
// variables through declarations and assignments, recording the concrete
// dynamic type stored in them when that type is statically known. A single-value
// type assertion is reported only when the recorded dynamic type is
// incompatible with the asserted type, so the assertion can never succeed and
// would panic. Values whose dynamic type cannot be established are treated as
// unknown rather than guessed.
package assert

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports type assertions that provably cannot succeed within the
// enclosing function.
var Analyzer = &analysis.Analyzer{
	Name: "assert",
	Doc:  "report type assertions that provably cannot succeed",
	Run:  run,
}

// state records the known concrete dynamic type of local interface variables.
// A missing entry means the dynamic type is unknown.
type state map[*types.Var]types.Type

type analyzer struct {
	pass    *analysis.Pass
	escaped map[*types.Var]struct{}
	commaOK map[*ast.TypeAssertExpr]bool
}

func run(pass *analysis.Pass) (any, error) {
	a := &analyzer{pass: pass}
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch fn := n.(type) {
			case *ast.FuncDecl:
				if fn.Body != nil {
					a.analyzeFunc(fn.Type, fn.Body)
				}
			case *ast.FuncLit:
				a.analyzeFunc(fn.Type, fn.Body)
			}
			return true
		})
	}
	return nil, nil
}

func (a *analyzer) analyzeFunc(ft *ast.FuncType, body *ast.BlockStmt) {
	a.escaped = a.findEscaped(body)
	a.commaOK = a.findCommaOK(body)
	s := make(state)
	a.analyzeBlock(body, s)
}

// findEscaped identifies local variables whose value can change without an
// explicit assignment in the analyzed function. Address-taken and
// closure-captured variables are excluded from tracking.
func (a *analyzer) findEscaped(body *ast.BlockStmt) map[*types.Var]struct{} {
	escaped := make(map[*types.Var]struct{})
	ast.Inspect(body, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.UnaryExpr:
			if e.Op == token.AND {
				if v := a.varOf(e.X); v != nil && a.local(v) {
					escaped[v] = struct{}{}
				}
			}
		case *ast.FuncLit:
			ast.Inspect(e.Body, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok {
					if v := a.varOf(id); v != nil && a.local(v) {
						escaped[v] = struct{}{}
					}
				}
				return true
			})
			return false
		}
		return true
	})
	return escaped
}

// findCommaOK records type assertions used in the two-value "comma ok" form,
// which never panics and is therefore not reported.
func (a *analyzer) findCommaOK(body *ast.BlockStmt) map[*ast.TypeAssertExpr]bool {
	commaOK := make(map[*ast.TypeAssertExpr]bool)
	record := func(rhs ast.Expr, n int) {
		if n != 2 {
			return
		}
		if ta, ok := unparen(rhs).(*ast.TypeAssertExpr); ok {
			commaOK[ta] = true
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.AssignStmt:
			if len(e.Rhs) == 1 {
				record(e.Rhs[0], len(e.Lhs))
			}
		case *ast.ValueSpec:
			if len(e.Values) == 1 {
				record(e.Values[0], len(e.Names))
			}
		}
		return true
	})
	return commaOK
}

func (a *analyzer) local(v *types.Var) bool {
	return v != nil && !v.IsField() && v.Pkg() == a.pass.Pkg && v.Parent() != a.pass.Pkg.Scope()
}

func (a *analyzer) track(v *types.Var) bool {
	if !a.local(v) {
		return false
	}
	_, escaped := a.escaped[v]
	return !escaped
}

func (a *analyzer) analyzeBlock(block *ast.BlockStmt, s state) state {
	return a.analyzeStmtList(block.List, s)
}

func (a *analyzer) analyzeStmtList(stmts []ast.Stmt, s state) state {
	for _, stmt := range stmts {
		s = a.analyzeStmt(stmt, s)
		if terminates(stmt) {
			break
		}
	}
	return s
}

// terminates reports whether a statement ends the current straight-line flow.
// It is used only to avoid inspecting unreachable statements.
func terminates(stmt ast.Stmt) bool {
	switch st := stmt.(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.BlockStmt:
		return len(st.List) > 0 && terminates(st.List[len(st.List)-1])
	case *ast.IfStmt:
		return st.Else != nil && terminates(st.Body) && terminates(st.Else)
	}
	return false
}

func (a *analyzer) analyzeStmt(stmt ast.Stmt, s state) state {
	switch st := stmt.(type) {
	case *ast.AssignStmt:
		return a.analyzeAssign(st, s)
	case *ast.DeclStmt:
		return a.analyzeDecl(st, s)
	case *ast.ExprStmt:
		a.analyzeExpr(st.X, s)
	case *ast.ReturnStmt:
		for _, result := range st.Results {
			a.analyzeExpr(result, s)
		}
	case *ast.IfStmt:
		return a.analyzeIf(st, s)
	case *ast.BlockStmt:
		return a.analyzeBlock(st, s)
	case *ast.ForStmt:
		return a.analyzeFor(st, s)
	case *ast.RangeStmt:
		return a.analyzeRange(st, s)
	case *ast.SwitchStmt:
		return a.analyzeSwitch(st, s)
	case *ast.TypeSwitchStmt:
		return a.analyzeTypeSwitch(st, s)
	case *ast.SelectStmt:
		return a.analyzeSelect(st, s)
	case *ast.GoStmt:
		a.analyzeExpr(st.Call, s)
	case *ast.DeferStmt:
		a.analyzeExpr(st.Call, s)
	case *ast.SendStmt:
		a.analyzeExpr(st.Chan, s)
		a.analyzeExpr(st.Value, s)
	case *ast.IncDecStmt:
		a.analyzeExpr(st.X, s)
	case *ast.LabeledStmt:
		return a.analyzeStmt(st.Stmt, s)
	}
	return s
}

func (a *analyzer) analyzeIf(st *ast.IfStmt, s state) state {
	if st.Init != nil {
		s = a.analyzeStmt(st.Init, s)
	}
	a.analyzeExpr(st.Cond, s)

	thenEnd := a.analyzeBlock(st.Body, clone(s))
	elseEnd := clone(s)
	if st.Else != nil {
		elseEnd = a.analyzeStmt(st.Else, clone(s))
	}
	return merge(thenEnd, elseEnd)
}

func (a *analyzer) analyzeFor(st *ast.ForStmt, s state) state {
	if st.Init != nil {
		s = a.analyzeStmt(st.Init, s)
	}
	if st.Cond != nil {
		a.analyzeExpr(st.Cond, s)
	}

	bodyState := clone(s)
	a.analyzeBlock(st.Body, bodyState)
	if st.Post != nil {
		a.analyzeStmt(st.Post, bodyState)
	}

	assigned := a.assignedVars(st.Body)
	if st.Post != nil {
		addVars(assigned, a.assignedVars(st.Post))
	}
	if st.Cond != nil {
		addVars(assigned, a.assignedVars(st.Cond))
	}
	forgetAll(s, assigned)
	return s
}

func (a *analyzer) analyzeRange(st *ast.RangeStmt, s state) state {
	a.analyzeExpr(st.X, s)

	bodyState := clone(s)
	if v := a.varOf(st.Key); v != nil && a.track(v) {
		forget(bodyState, v)
	}
	if v := a.varOf(st.Value); v != nil && a.track(v) {
		forget(bodyState, v)
	}
	a.analyzeBlock(st.Body, bodyState)

	assigned := a.assignedVars(st.Body)
	if v := a.varOf(st.Key); v != nil {
		assigned[v] = struct{}{}
	}
	if v := a.varOf(st.Value); v != nil {
		assigned[v] = struct{}{}
	}
	forgetAll(s, assigned)
	return s
}

func (a *analyzer) analyzeSwitch(st *ast.SwitchStmt, s state) state {
	if st.Init != nil {
		s = a.analyzeStmt(st.Init, s)
	}
	if st.Tag != nil {
		a.analyzeExpr(st.Tag, s)
	}

	out := clone(s)
	for _, clause := range st.Body.List {
		cc, ok := clause.(*ast.CaseClause)
		if !ok {
			continue
		}
		cs := clone(s)
		for _, expr := range cc.List {
			a.analyzeExpr(expr, cs)
		}
		out = merge(out, a.analyzeStmtList(cc.Body, cs))
	}
	return out
}

func (a *analyzer) analyzeTypeSwitch(st *ast.TypeSwitchStmt, s state) state {
	if st.Init != nil {
		s = a.analyzeStmt(st.Init, s)
	}
	if st.Assign != nil {
		s = a.analyzeStmt(st.Assign, s)
	}

	out := clone(s)
	for _, clause := range st.Body.List {
		cc, ok := clause.(*ast.CaseClause)
		if !ok {
			continue
		}
		// Case expressions are types, not runtime values.
		out = merge(out, a.analyzeStmtList(cc.Body, clone(s)))
	}
	return out
}

func (a *analyzer) analyzeSelect(st *ast.SelectStmt, s state) state {
	out := clone(s)
	for _, clause := range st.Body.List {
		cc, ok := clause.(*ast.CommClause)
		if !ok {
			continue
		}
		cs := clone(s)
		if cc.Comm != nil {
			cs = a.analyzeStmt(cc.Comm, cs)
		}
		out = merge(out, a.analyzeStmtList(cc.Body, cs))
	}
	return out
}

func (a *analyzer) analyzeAssign(st *ast.AssignStmt, s state) state {
	for _, lhs := range st.Lhs {
		a.analyzeExpr(lhs, s)
	}
	for _, rhs := range st.Rhs {
		a.analyzeExpr(rhs, s)
	}

	if (st.Tok == token.DEFINE || st.Tok == token.ASSIGN) && len(st.Lhs) == len(st.Rhs) {
		for i, lhs := range st.Lhs {
			t, ok := a.exprConcrete(st.Rhs[i])
			a.assign(lhs, t, ok, s)
		}
		return s
	}

	// Compound assignments and multiple-value right-hand sides provide no
	// definite dynamic type information.
	for _, lhs := range st.Lhs {
		a.assign(lhs, nil, false, s)
	}
	return s
}

func (a *analyzer) analyzeDecl(st *ast.DeclStmt, s state) state {
	gen, ok := st.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return s
	}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if len(vs.Values) == 0 {
			for _, name := range vs.Names {
				if v := a.varOf(name); v != nil && a.track(v) {
					forget(s, v)
				}
			}
			continue
		}
		for _, val := range vs.Values {
			a.analyzeExpr(val, s)
		}
		if len(vs.Values) == len(vs.Names) {
			for i, name := range vs.Names {
				t, ok := a.exprConcrete(vs.Values[i])
				a.assign(name, t, ok, s)
			}
			continue
		}
		for _, name := range vs.Names {
			if v := a.varOf(name); v != nil && a.track(v) {
				forget(s, v)
			}
		}
	}
	return s
}

func (a *analyzer) analyzeExpr(expr ast.Expr, s state) {
	if expr == nil {
		return
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.IsType() {
		return
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		a.analyzeExpr(e.X, s)
	case *ast.TypeAssertExpr:
		a.checkAssertion(e, s)
		a.analyzeExpr(e.X, s)
	case *ast.IndexExpr:
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Index, s)
	case *ast.IndexListExpr:
		a.analyzeExpr(e.X, s)
		for _, index := range e.Indices {
			a.analyzeExpr(index, s)
		}
	case *ast.SliceExpr:
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Low, s)
		a.analyzeExpr(e.High, s)
		a.analyzeExpr(e.Max, s)
	case *ast.BinaryExpr:
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Y, s)
	case *ast.UnaryExpr:
		a.analyzeExpr(e.X, s)
	case *ast.StarExpr:
		a.analyzeExpr(e.X, s)
	case *ast.SelectorExpr:
		a.analyzeExpr(e.X, s)
	case *ast.CallExpr:
		if tv, ok := a.pass.TypesInfo.Types[e.Fun]; !ok || !tv.IsType() {
			a.analyzeExpr(e.Fun, s)
		}
		for _, arg := range e.Args {
			a.analyzeExpr(arg, s)
		}
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			a.analyzeExpr(elt, s)
		}
	case *ast.KeyValueExpr:
		a.analyzeExpr(e.Key, s)
		a.analyzeExpr(e.Value, s)
	case *ast.Ellipsis:
		a.analyzeExpr(e.Elt, s)
	}
}

// checkAssertion reports a single-value type assertion whose asserted type is
// incompatible with the tracked dynamic type of the operand.
func (a *analyzer) checkAssertion(e *ast.TypeAssertExpr, s state) {
	// A type switch guard (x.(type)) is not an assertion.
	if e.Type == nil {
		return
	}
	// The two-value form never panics and is therefore not reported.
	if a.commaOK[e] {
		return
	}
	v := a.varOf(e.X)
	if v == nil || !a.track(v) || !isInterface(v.Type()) {
		return
	}
	dyn, ok := s[v]
	if !ok {
		return
	}
	asserted := a.pass.TypesInfo.TypeOf(e.Type)
	if asserted == nil {
		return
	}
	if _, isParam := asserted.(*types.TypeParam); isParam {
		return
	}

	if iface, ok := asserted.Underlying().(*types.Interface); ok {
		if !types.Implements(dyn, iface) {
			a.pass.Reportf(e.Pos(), "impossible type assertion: %s is %s, which does not implement %s",
				types.ExprString(e.X), a.typeString(dyn), a.typeString(asserted))
		}
		return
	}
	if !types.Identical(dyn, asserted) {
		a.pass.Reportf(e.Pos(), "impossible type assertion: %s is %s, not %s",
			types.ExprString(e.X), a.typeString(dyn), a.typeString(asserted))
	}
}

// exprConcrete returns the concrete dynamic type of an expression when it can
// be established statically. Interface-typed and nil expressions yield no
// concrete type.
func (a *analyzer) exprConcrete(expr ast.Expr) (types.Type, bool) {
	if expr == nil {
		return nil, false
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.IsNil() {
		return nil, false
	}
	t := a.typeOf(expr)
	if t == nil || isInterface(t) {
		return nil, false
	}
	return t, true
}

func (a *analyzer) assign(lhs ast.Expr, t types.Type, known bool, s state) {
	v := a.varOf(lhs)
	if v == nil || !a.track(v) || !isInterface(v.Type()) {
		return
	}
	if known {
		s[v] = t
	} else {
		forget(s, v)
	}
}

func (a *analyzer) varOf(expr ast.Expr) *types.Var {
	id, ok := unparen(expr).(*ast.Ident)
	if !ok || id.Name == "_" {
		return nil
	}
	v, _ := a.pass.TypesInfo.ObjectOf(id).(*types.Var)
	return v
}

func (a *analyzer) typeOf(expr ast.Expr) types.Type {
	if expr == nil {
		return nil
	}
	return a.pass.TypesInfo.TypeOf(expr)
}

func (a *analyzer) typeString(t types.Type) string {
	return types.TypeString(t, types.RelativeTo(a.pass.Pkg))
}

func isInterface(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}

func (a *analyzer) assignedVars(node ast.Node) map[*types.Var]struct{} {
	out := make(map[*types.Var]struct{})
	if node == nil {
		return out
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range e.Lhs {
				if v := a.varOf(lhs); v != nil {
					out[v] = struct{}{}
				}
			}
		case *ast.IncDecStmt:
			if v := a.varOf(e.X); v != nil {
				out[v] = struct{}{}
			}
		case *ast.RangeStmt:
			if v := a.varOf(e.Key); v != nil {
				out[v] = struct{}{}
			}
			if v := a.varOf(e.Value); v != nil {
				out[v] = struct{}{}
			}
		}
		return true
	})
	return out
}

func clone(s state) state {
	out := make(state, len(s))
	for v, t := range s {
		out[v] = t
	}
	return out
}

func merge(a, b state) state {
	out := clone(a)
	for v, bv := range b {
		av, ok := out[v]
		if !ok || !types.Identical(av, bv) {
			delete(out, v)
		}
	}
	for v := range out {
		if _, ok := b[v]; !ok {
			delete(out, v)
		}
	}
	return out
}

func forget(s state, v *types.Var) {
	delete(s, v)
}

func forgetAll(s state, vars map[*types.Var]struct{}) {
	for v := range vars {
		forget(s, v)
	}
}

func addVars(dst, src map[*types.Var]struct{}) {
	for v := range src {
		dst[v] = struct{}{}
	}
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}
