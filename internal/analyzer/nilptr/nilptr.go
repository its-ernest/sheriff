// Package nilptr reports dereferences of pointers that are provably nil.
//
// The analysis is intraprocedural and deliberately conservative. It tracks
// local pointer variables through declarations, assignments and simple nil
// checks, and reports a dereference only when the pointer is definitely nil on
// every path that reaches it. Values whose nilness cannot be established are
// treated as unknown rather than guessed.
package nilptr

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports dereferences of pointers that are provably nil within the
// enclosing function.
var Analyzer = &analysis.Analyzer{
	Name: "nilptr",
	Doc:  "report dereferences of pointers that are provably nil",
	Run:  run,
}

type nilness uint8

const (
	unknown nilness = iota
	isNil
	nonNil
	// maybeNil means the pointer may be nil or non-nil: for example a value
	// produced by a function with mixed return paths. It is distinct from
	// unknown, which means nilness could not be established at all.
	maybeNil
)

type state map[*types.Var]nilness

type analyzer struct {
	pass     *analysis.Pass
	escaped  map[*types.Var]struct{}
	funcs    map[*types.Func]*ast.FuncDecl
	reported map[token.Pos]bool
	depth    int

	// Return-value summaries for same-package functions. summaries caches the
	// nilness of a function's single pointer result; computing guards against
	// recursive summary computation.
	summaries map[*types.Func]nilness
	computing map[*types.Func]bool

	// collect is non-nil while a function body is analyzed to compute its
	// return summary; return statements append their result nilness to it.
	collect    []nilness
	collecting bool
	// resultVar is the named single pointer result during summary computation,
	// used to evaluate a bare return.
	resultVar *types.Var
	// suppress disables diagnostics while computing summaries.
	suppress bool
}

func run(pass *analysis.Pass) (any, error) {
	a := &analyzer{
		pass:      pass,
		funcs:     make(map[*types.Func]*ast.FuncDecl),
		reported:  make(map[token.Pos]bool),
		summaries: make(map[*types.Func]nilness),
		computing: make(map[*types.Func]bool),
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil {
				continue
			}
			if obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func); ok {
				a.funcs[obj] = fn
			}
		}
	}
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
	a.analyzeFuncSeed(ft, body, nil)
}

// analyzeFuncSeed analyzes a function body with an optional seed state. The
// seed supplies known values for parameters, which is how tracked argument
// values are propagated from a call site into a same-package callee.
func (a *analyzer) analyzeFuncSeed(ft *ast.FuncType, body *ast.BlockStmt, seed state) {
	prevEscaped := a.escaped
	a.escaped = a.findEscaped(body)
	s := make(state, len(seed))
	for v, n := range seed {
		s[v] = n
	}
	a.initResults(ft, s)
	a.analyzeBlock(body, s)
	a.escaped = prevEscaped
}

// propagateCall analyzes a same-package callee with the tracked argument
// values seeded into its parameters, so operations on those parameters are
// reported even though the callee is a separate function. It runs only at the
// top level so that analysis does not recurse transitively through nested
// calls.
func (a *analyzer) propagateCall(call *ast.CallExpr, s state) {
	if a.depth > 0 {
		return
	}
	fn := a.calleeFunc(call)
	if fn == nil {
		return
	}
	decl := a.funcs[fn]
	if decl == nil || decl.Body == nil {
		return
	}
	params := a.paramVars(fn)
	if len(params) == 0 {
		return
	}
	seed := make(state)
	seeded := false
	for i, arg := range call.Args {
		if i >= len(params) {
			break
		}
		p := params[i]
		if p == nil || !isPointer(p.Type()) {
			continue
		}
		n := a.exprNilness(arg, s)
		if n == unknown {
			continue
		}
		seed[p] = n
		seeded = true
	}
	if !seeded {
		return
	}
	a.depth++
	a.analyzeFuncSeed(decl.Type, decl.Body, seed)
	a.depth--
}

// calleeFunc resolves a direct call to a package-level function declared in the
// analyzed package. Calls through function values, methods and imported
// functions are not resolved.
func (a *analyzer) calleeFunc(call *ast.CallExpr) *types.Func {
	id, ok := unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil
	}
	fn, _ := a.pass.TypesInfo.Uses[id].(*types.Func)
	if fn == nil || fn.Pkg() != a.pass.Pkg {
		return nil
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() != nil {
		return nil
	}
	return fn
}

func (a *analyzer) paramVars(fn *types.Func) []*types.Var {
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil
	}
	params := sig.Params()
	vars := make([]*types.Var, params.Len())
	for i := range vars {
		vars[i] = params.At(i)
	}
	return vars
}

// findEscaped identifies local variables whose value can change without an
// explicit assignment in the analyzed function. Address-taken variables can be
// modified through the pointer, and closure-captured variables can be modified
// when the closure runs. Both are excluded from tracking.
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

// initResults marks named pointer results as nil: they start with their zero
// value and are therefore nil until assigned.
func (a *analyzer) initResults(ft *ast.FuncType, s state) {
	if ft.Results == nil {
		return
	}
	for _, field := range ft.Results.List {
		for _, name := range field.Names {
			if v := a.varOf(name); v != nil && a.track(v) && isPointer(v.Type()) {
				s[v] = isNil
			}
		}
	}
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
// It is used only to avoid inspecting unreachable statements; it is not a full
// control-flow analysis.
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
		if a.collecting {
			a.collect = append(a.collect, a.returnNilness(st, s))
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

	thenState := clone(s)
	elseState := clone(s)
	a.refine(st.Cond, true, thenState)
	a.refine(st.Cond, false, elseState)

	thenEnd := a.analyzeBlock(st.Body, thenState)
	elseEnd := elseState
	if st.Else != nil {
		elseEnd = a.analyzeStmt(st.Else, elseState)
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

	// Analyze the body with the condition known to be true. This is the state
	// for the first iteration; variables assigned by the loop are forgotten
	// afterwards because later iterations may differ.
	bodyState := clone(s)
	if st.Cond != nil {
		a.refine(st.Cond, true, bodyState)
	}
	bodyState = a.analyzeBlock(st.Body, bodyState)
	if st.Post != nil {
		a.analyzeStmt(st.Post, bodyState)
	}

	assigned := a.assignedVars(st.Body)
	if st.Post != nil {
		for v := range a.assignedVars(st.Post) {
			assigned[v] = struct{}{}
		}
	}
	for v := range assigned {
		if _, ok := s[v]; ok {
			s[v] = unknown
		}
	}
	return s
}

func (a *analyzer) analyzeRange(st *ast.RangeStmt, s state) state {
	if isPointerToArray(a.typeOf(st.X)) {
		a.reportNilness(st.X.Pos(), st.X, a.exprNilness(st.X, s))
	}
	a.analyzeExpr(st.X, s)

	bodyState := clone(s)
	if v := a.varOf(st.Key); v != nil && a.track(v) {
		bodyState[v] = unknown
	}
	if v := a.varOf(st.Value); v != nil && a.track(v) {
		bodyState[v] = unknown
	}
	a.analyzeBlock(st.Body, bodyState)

	assigned := a.assignedVars(st.Body)
	if v := a.varOf(st.Key); v != nil {
		assigned[v] = struct{}{}
	}
	if v := a.varOf(st.Value); v != nil {
		assigned[v] = struct{}{}
	}
	for v := range assigned {
		if _, ok := s[v]; ok {
			s[v] = unknown
		}
	}
	return s
}

func (a *analyzer) analyzeSwitch(st *ast.SwitchStmt, s state) state {
	if st.Init != nil {
		s = a.analyzeStmt(st.Init, s)
	}
	if st.Tag != nil {
		a.analyzeExpr(st.Tag, s)
	}

	// Without a default clause, control can reach the statement after the
	// switch without any case matching; that path keeps the pre-switch state.
	// A default clause handles the no-match case instead, so it must not be
	// merged in as an extra path.
	hasDefault := false
	for _, clause := range st.Body.List {
		if cc, ok := clause.(*ast.CaseClause); ok && len(cc.List) == 0 {
			hasDefault = true
		}
	}

	var out state
	if !hasDefault {
		out = clone(s)
	}
	for _, clause := range st.Body.List {
		cc, ok := clause.(*ast.CaseClause)
		if !ok {
			continue
		}
		cs := clone(s)
		if len(cc.List) == 0 {
			var reachable bool
			cs, reachable = a.refineDefault(st, cs)
			if !reachable {
				continue
			}
		} else {
			for _, expr := range cc.List {
				a.analyzeExpr(expr, cs)
			}
			if !a.refineCase(st, cc, cs) {
				continue
			}
		}
		res := a.analyzeStmtList(cc.Body, cs)
		if out == nil {
			out = res
		} else {
			out = merge(out, res)
		}
	}
	if out == nil {
		out = clone(s)
	}
	return out
}

// refineDefault determines whether the default clause is reachable and what is
// known about a pointer tag inside it. A nil tag with a case that definitely
// matches nil makes the default clause unreachable.
func (a *analyzer) refineDefault(st *ast.SwitchStmt, s state) (state, bool) {
	if st.Tag == nil {
		return s, true
	}
	tag := a.varOf(st.Tag)
	if tag == nil || !a.track(tag) || !isPointer(tag.Type()) || s[tag] != isNil {
		return s, true
	}
	for _, clause := range st.Body.List {
		cc, ok := clause.(*ast.CaseClause)
		if !ok || len(cc.List) == 0 {
			continue
		}
		for _, expr := range cc.List {
			if a.exprNilness(expr, s) == isNil {
				return s, false
			}
		}
	}
	return s, true
}

// refineCase narrows the state for a case clause and reports whether the case
// body is reachable. A tag switch does not change the tag's value, so the tag
// keeps its pre-switch nilness unless the case definitely cannot match.
func (a *analyzer) refineCase(st *ast.SwitchStmt, cc *ast.CaseClause, s state) bool {
	if st.Tag == nil {
		if len(cc.List) == 1 {
			a.refine(cc.List[0], true, s)
		}
		return true
	}
	tag := a.varOf(st.Tag)
	if tag == nil || !a.track(tag) || !isPointer(tag.Type()) {
		return true
	}

	tagState := s[tag]
	caseState := a.caseNilness(cc, s)
	// Only a definite mismatch proves the case unreachable. A maybeNil tag may
	// still match a nil or non-nil case.
	if definite(tagState) && definite(caseState) && tagState != caseState {
		return false
	}
	// If the tag is nil and the case may or may not match nil, do not claim
	// the tag is definitely nil inside the body.
	if tagState == isNil && caseState == unknown {
		s[tag] = unknown
	}
	return true
}

func (a *analyzer) caseNilness(cc *ast.CaseClause, s state) nilness {
	first := unknown
	for i, expr := range cc.List {
		n := a.exprNilness(expr, s)
		if n == unknown {
			return unknown
		}
		if i == 0 {
			first = n
		} else if n != first {
			return unknown
		}
	}
	return first
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
		a.analyzeLHS(lhs, s)
	}
	for _, rhs := range st.Rhs {
		a.analyzeExpr(rhs, s)
	}

	if len(st.Lhs) == len(st.Rhs) {
		values := make([]nilness, len(st.Rhs))
		for i, rhs := range st.Rhs {
			values[i] = a.exprNilness(rhs, s)
		}
		for i, lhs := range st.Lhs {
			a.assign(lhs, values[i], s)
		}
		return s
	}

	// One right-hand side feeding several variables (a call, map lookup, type
	// assertion, and so on) provides no definite nilness information.
	for _, lhs := range st.Lhs {
		a.assign(lhs, unknown, s)
	}
	return s
}

func (a *analyzer) analyzeLHS(expr ast.Expr, s state) {
	switch e := unparen(expr).(type) {
	case *ast.StarExpr:
		a.checkDeref(e.X, e.Pos(), s)
		a.analyzeExpr(e.X, s)
	case *ast.SelectorExpr:
		a.checkField(e, s)
		a.analyzeExpr(e.X, s)
	case *ast.IndexExpr:
		a.checkPointerArrayIndex(e, s)
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Index, s)
	default:
		a.analyzeExpr(expr, s)
	}
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
				if v := a.varOf(name); v != nil && a.track(v) && isPointer(v.Type()) {
					s[v] = isNil
				}
			}
			continue
		}
		for _, val := range vs.Values {
			a.analyzeExpr(val, s)
		}
		if len(vs.Values) == len(vs.Names) {
			values := make([]nilness, len(vs.Values))
			for i, val := range vs.Values {
				values[i] = a.exprNilness(val, s)
			}
			for i, name := range vs.Names {
				if v := a.varOf(name); v != nil && a.track(v) && isPointer(v.Type()) {
					s[v] = values[i]
				}
			}
			continue
		}
		for _, name := range vs.Names {
			if v := a.varOf(name); v != nil && a.track(v) && isPointer(v.Type()) {
				s[v] = unknown
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
	case *ast.StarExpr:
		a.checkDeref(e.X, e.Pos(), s)
		a.analyzeExpr(e.X, s)
	case *ast.SelectorExpr:
		a.checkField(e, s)
		a.analyzeExpr(e.X, s)
	case *ast.IndexExpr:
		a.checkPointerArrayIndex(e, s)
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Index, s)
	case *ast.IndexListExpr:
		a.analyzeExpr(e.X, s)
		for _, index := range e.Indices {
			a.analyzeExpr(index, s)
		}
	case *ast.SliceExpr:
		a.checkPointerArraySlice(e, s)
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Low, s)
		a.analyzeExpr(e.High, s)
		a.analyzeExpr(e.Max, s)
	case *ast.CallExpr:
		if tv, ok := a.pass.TypesInfo.Types[e.Fun]; !ok || !tv.IsType() {
			a.analyzeExpr(e.Fun, s)
		}
		for _, arg := range e.Args {
			a.analyzeExpr(arg, s)
		}
		a.propagateCall(e, s)
	case *ast.UnaryExpr:
		a.analyzeExpr(e.X, s)
	case *ast.BinaryExpr:
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Y, s)
	case *ast.TypeAssertExpr:
		a.analyzeExpr(e.X, s)
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

func (a *analyzer) checkDeref(ptr ast.Expr, pos token.Pos, s state) {
	a.reportNilness(pos, ptr, a.exprNilness(ptr, s))
}

func (a *analyzer) checkField(sel *ast.SelectorExpr, s state) {
	selection, ok := a.pass.TypesInfo.Selections[sel]
	if !ok || selection.Kind() != types.FieldVal {
		return
	}
	a.reportNilness(sel.Pos(), sel.X, a.exprNilness(sel.X, s))
}

func (a *analyzer) checkPointerArrayIndex(index *ast.IndexExpr, s state) {
	if !isPointerToArray(a.typeOf(index.X)) {
		return
	}
	a.reportNilness(index.Pos(), index.X, a.exprNilness(index.X, s))
}

func (a *analyzer) checkPointerArraySlice(slice *ast.SliceExpr, s state) {
	if !isPointerToArray(a.typeOf(slice.X)) {
		return
	}
	a.reportNilness(slice.Pos(), slice.X, a.exprNilness(slice.X, s))
}

// reportNilness reports a dereference according to the pointer's nilness: a
// definite nil is a definite finding, while maybeNil is a potential-risk
// finding. unknown is not reported.
func (a *analyzer) reportNilness(pos token.Pos, ptr ast.Expr, n nilness) {
	switch n {
	case isNil:
		a.reportf(pos, "nil pointer dereference: %s is nil", types.ExprString(ptr))
	case maybeNil:
		a.reportf(pos, "nil pointer dereference: %s may be nil", types.ExprString(ptr))
	}
}

func (a *analyzer) reportf(pos token.Pos, format string, args ...any) {
	if a.suppress || a.reported[pos] {
		return
	}
	a.reported[pos] = true
	a.pass.Reportf(pos, format, args...)
}

func (a *analyzer) exprNilness(expr ast.Expr, s state) nilness {
	if expr == nil {
		return unknown
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.IsNil() {
		return isNil
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.exprNilness(e.X, s)
	case *ast.Ident:
		if v := a.varOf(e); v != nil && a.track(v) {
			return s[v]
		}
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return nonNil
		}
	case *ast.CallExpr:
		if a.isNew(e) {
			return nonNil
		}
		if a.isNilConversion(e) {
			return isNil
		}
		if fn := a.calleeFunc(e); fn != nil {
			return a.summaryOf(fn)
		}
	}
	return unknown
}

func (a *analyzer) isNew(call *ast.CallExpr) bool {
	id, ok := unparen(call.Fun).(*ast.Ident)
	if !ok || id.Name != "new" {
		return false
	}
	b, ok := a.pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && b.Name() == "new"
}

func (a *analyzer) isNilConversion(call *ast.CallExpr) bool {
	tv, ok := a.pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() || len(call.Args) != 1 {
		return false
	}
	return a.isNilExpr(call.Args[0])
}

func (a *analyzer) isNilExpr(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.IsNil() {
		return true
	}
	if p, ok := expr.(*ast.ParenExpr); ok {
		return a.isNilExpr(p.X)
	}
	return false
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

func isPointer(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Pointer)
	return ok
}

func isPointerToArray(t types.Type) bool {
	if t == nil {
		return false
	}
	p, ok := t.Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	_, ok = p.Elem().Underlying().(*types.Array)
	return ok
}

func (a *analyzer) assign(lhs ast.Expr, n nilness, s state) {
	v := a.varOf(lhs)
	if v == nil || !a.track(v) || !isPointer(v.Type()) {
		return
	}
	s[v] = n
}

func (a *analyzer) refine(cond ast.Expr, truth bool, s state) {
	switch e := cond.(type) {
	case *ast.ParenExpr:
		a.refine(e.X, truth, s)
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			a.refine(e.X, !truth, s)
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.LAND:
			if truth {
				a.refine(e.X, true, s)
				a.refine(e.Y, true, s)
			}
		case token.LOR:
			if !truth {
				a.refine(e.X, false, s)
				a.refine(e.Y, false, s)
			}
		case token.EQL, token.NEQ:
			a.refineNilComparison(e, truth, s)
		}
	}
}

func (a *analyzer) refineNilComparison(e *ast.BinaryExpr, truth bool, s state) {
	var other ast.Expr
	switch {
	case a.isNilExpr(e.X):
		other = e.Y
	case a.isNilExpr(e.Y):
		other = e.X
	default:
		return
	}
	v := a.varOf(other)
	if v == nil || !a.track(v) || !isPointer(v.Type()) {
		return
	}
	// p == nil is true exactly when p is nil; p != nil is the inverse.
	equal := e.Op == token.EQL
	if truth == equal {
		s[v] = isNil
	} else {
		s[v] = nonNil
	}
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
	for v, n := range s {
		out[v] = n
	}
	return out
}

// join combines two nilness facts known on different control-flow paths. A
// definite isNil and nonNil combine to maybeNil; unknown dominates because it
// means nilness could not be established.
func join(x, y nilness) nilness {
	if x == y {
		return x
	}
	if x == unknown || y == unknown {
		return unknown
	}
	if x == maybeNil || y == maybeNil {
		return maybeNil
	}
	return maybeNil
}

func merge(a, b state) state {
	out := clone(a)
	for v, bn := range b {
		an, ok := out[v]
		if !ok {
			an = unknown
		}
		out[v] = join(an, bn)
	}
	for v, an := range a {
		if _, ok := b[v]; !ok {
			out[v] = join(an, unknown)
		}
	}
	return out
}

func joinNilness(vals []nilness) nilness {
	if len(vals) == 0 {
		return unknown
	}
	out := vals[0]
	for _, v := range vals[1:] {
		out = join(out, v)
	}
	return out
}

func definite(n nilness) bool {
	return n == isNil || n == nonNil
}

// pointerResult reports the single pointer result of a function signature, if
// it has exactly one.
func pointerResult(fn *types.Func) (*types.Var, bool) {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Results() == nil || sig.Results().Len() != 1 {
		return nil, false
	}
	v := sig.Results().At(0)
	if !isPointer(v.Type()) {
		return nil, false
	}
	return v, true
}

// summaryOf returns the nilness of a function's single pointer result, computed
// from its reachable return statements. Results are cached. Diagnostics are
// suppressed and interprocedural argument seeding is disabled while a summary
// is computed; recursive cycles yield unknown.
func (a *analyzer) summaryOf(fn *types.Func) nilness {
	if n, ok := a.summaries[fn]; ok {
		return n
	}
	if a.computing[fn] {
		return unknown
	}
	decl := a.funcs[fn]
	resultVar, ok := pointerResult(fn)
	if decl == nil || decl.Body == nil || !ok {
		a.summaries[fn] = unknown
		return unknown
	}

	prevSuppress := a.suppress
	prevDepth := a.depth
	prevCollect := a.collect
	prevCollecting := a.collecting
	prevResult := a.resultVar

	a.computing[fn] = true
	a.suppress = true
	a.depth++
	a.collect = nil
	a.collecting = true
	a.resultVar = resultVar

	a.analyzeFuncSeed(decl.Type, decl.Body, nil)
	returns := a.collect

	a.collect = prevCollect
	a.collecting = prevCollecting
	a.resultVar = prevResult
	a.depth = prevDepth
	a.suppress = prevSuppress
	delete(a.computing, fn)

	n := joinNilness(returns)
	a.summaries[fn] = n
	return n
}

// returnNilness evaluates the nilness produced by a return statement for a
// single pointer result. A bare return uses the named result's current value.
func (a *analyzer) returnNilness(st *ast.ReturnStmt, s state) nilness {
	if len(st.Results) == 1 {
		return a.exprNilness(st.Results[0], s)
	}
	if len(st.Results) == 0 && a.resultVar != nil && a.resultVar.Name() != "" {
		return s[a.resultVar]
	}
	return unknown
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
