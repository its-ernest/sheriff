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
	"go/constant"
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

// state is the intraprocedural analysis state: the nilness of pointer
// variables and the constant integer values of integer variables. The integer
// constants let the analyzer evaluate call arguments and branch conditions.
type state struct {
	nilness map[*types.Var]nilness
	ints    map[*types.Var]int64
	funcs   map[*types.Var]*types.Func
}

func newState() state {
	return state{
		nilness: make(map[*types.Var]nilness),
		ints:    make(map[*types.Var]int64),
		funcs:   make(map[*types.Var]*types.Func),
	}
}

func forget(s state, v *types.Var) {
	delete(s.nilness, v)
	delete(s.ints, v)
	delete(s.funcs, v)
}

func forgetAll(s state, vars map[*types.Var]struct{}) {
	for v := range vars {
		forget(s, v)
	}
}

type analyzer struct {
	pass     *analysis.Pass
	escaped  map[*types.Var]struct{}
	funcs    map[*types.Func]*ast.FuncDecl
	reported map[token.Pos]bool
	depth    int

	// evaluating guards against recursive call-result evaluation. evalUnknown
	// is set when a callee uses control flow the evaluator does not model.
	evaluating  map[*types.Func]bool
	evalUnknown bool
	// resultVar is the named single pointer result while evaluating a call,
	// used to evaluate a bare return.
	resultVar *types.Var
}

func run(pass *analysis.Pass) (any, error) {
	a := &analyzer{
		pass:       pass,
		funcs:      make(map[*types.Func]*ast.FuncDecl),
		reported:   make(map[token.Pos]bool),
		evaluating: make(map[*types.Func]bool),
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
	a.analyzeFuncSeed(ft, body, state{})
}

// analyzeFuncSeed analyzes a function body with an optional seed state. The
// seed supplies known values for parameters, which is how tracked argument
// values are propagated from a call site into a same-package callee.
func (a *analyzer) analyzeFuncSeed(ft *ast.FuncType, body *ast.BlockStmt, seed state) {
	prevEscaped := a.escaped
	a.escaped = a.findEscaped(body)
	s := clone(seed)
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
	fn := a.calleeFunc(call, s.funcs)
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
	seed := newState()
	seeded := false
	for i, arg := range call.Args {
		if i >= len(params) {
			break
		}
		p := params[i]
		if p == nil {
			continue
		}
		if isPointer(p.Type()) {
			n := a.exprNilness(arg, s)
			if n == unknown {
				continue
			}
			seed.nilness[p] = n
			seeded = true
			continue
		}
		if isInteger(p.Type()) {
			if x, ok := a.exprInt(arg, s); ok {
				seed.ints[p] = x
				seeded = true
			}
		}
	}
	if !seeded {
		return
	}
	a.depth++
	a.analyzeFuncSeed(decl.Type, decl.Body, seed)
	a.depth--
}

// calleeFunc resolves a direct call to a package-level function declared in the
// analyzed package, either referenced by name or held in a tracked function-value
// variable. Calls through methods and imported functions are not resolved.
func (a *analyzer) calleeFunc(call *ast.CallExpr, funcs map[*types.Var]*types.Func) *types.Func {
	id, ok := unparen(call.Fun).(*ast.Ident)
	if !ok {
		return nil
	}
	switch obj := a.pass.TypesInfo.Uses[id].(type) {
	case *types.Func:
		if obj.Pkg() != a.pass.Pkg {
			return nil
		}
		if sig, ok := obj.Type().(*types.Signature); !ok || sig.Recv() != nil {
			return nil
		}
		return obj
	case *types.Var:
		fn, ok := funcs[obj]
		if !ok || fn.Pkg() != a.pass.Pkg {
			return nil
		}
		return fn
	}
	return nil
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
			v := a.varOf(name)
			if v == nil || !a.track(v) {
				continue
			}
			if isPointer(v.Type()) {
				s.nilness[v] = isNil
			}
			if isInteger(v.Type()) {
				s.ints[v] = 0
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
		if v := a.varOf(st.X); v != nil && a.track(v) {
			forget(s, v)
		}
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
	forgetAll(s, assigned)
	return s
}

func (a *analyzer) analyzeRange(st *ast.RangeStmt, s state) state {
	if isPointerToArray(a.typeOf(st.X)) {
		a.reportNilness(st.X.Pos(), st.X, a.exprNilness(st.X, s))
	}
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
	set := false
	if !hasDefault {
		out = clone(s)
		set = true
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
		if !set {
			out = res
			set = true
		} else {
			out = merge(out, res)
		}
	}
	if !set {
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
	if tag == nil || !a.track(tag) || !isPointer(tag.Type()) || s.nilness[tag] != isNil {
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

	tagState := s.nilness[tag]
	caseState := a.caseNilness(cc, s)
	// Only a definite mismatch proves the case unreachable. A maybeNil tag may
	// still match a nil or non-nil case.
	if definite(tagState) && definite(caseState) && tagState != caseState {
		return false
	}
	// If the tag is nil and the case may or may not match nil, do not claim
	// the tag is definitely nil inside the body.
	if tagState == isNil && caseState == unknown {
		s.nilness[tag] = unknown
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

	if (st.Tok == token.DEFINE || st.Tok == token.ASSIGN) && len(st.Lhs) == len(st.Rhs) {
		values := make([]value, len(st.Rhs))
		for i, rhs := range st.Rhs {
			values[i] = a.eval(rhs, s)
		}
		for i, lhs := range st.Lhs {
			a.assign(lhs, values[i], s)
		}
		return s
	}

	// Compound assignments and one right-hand side feeding several variables
	// (a call, map lookup, type assertion, and so on) provide no definite
	// value information.
	for _, lhs := range st.Lhs {
		a.assign(lhs, value{nilness: unknown}, s)
	}
	return s
}

// value is the nilness, integer constant and function held by an expression.
type value struct {
	nilness nilness
	intVal  int64
	hasInt  bool
	fn      *types.Func
}

func (a *analyzer) eval(expr ast.Expr, s state) value {
	v := value{nilness: a.exprNilness(expr, s)}
	v.intVal, v.hasInt = a.exprInt(expr, s)
	v.fn, _ = a.exprFunc(expr)
	return v
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
				v := a.varOf(name)
				if v == nil || !a.track(v) {
					continue
				}
				if isPointer(v.Type()) {
					s.nilness[v] = isNil
				}
				if isInteger(v.Type()) {
					s.ints[v] = 0
				}
			}
			continue
		}
		for _, val := range vs.Values {
			a.analyzeExpr(val, s)
		}
		if len(vs.Values) == len(vs.Names) {
			values := make([]value, len(vs.Values))
			for i, val := range vs.Values {
				values[i] = a.eval(val, s)
			}
			for i, name := range vs.Names {
				if v := a.varOf(name); v != nil && a.track(v) {
					a.assign(name, values[i], s)
				}
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
	if a.reported[pos] {
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
			return s.nilness[v]
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
		return a.evalCall(e, evalEnv{nilness: s.nilness, ints: s.ints, funcs: s.funcs})
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

func isInteger(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsInteger != 0
}

func isFunc(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Signature)
	return ok
}

// exprFunc resolves an expression to a same-package function referenced by
// name. It is used to track function values held in local variables.
func (a *analyzer) exprFunc(expr ast.Expr) (*types.Func, bool) {
	id, ok := unparen(expr).(*ast.Ident)
	if !ok {
		return nil, false
	}
	fn, ok := a.pass.TypesInfo.Uses[id].(*types.Func)
	if !ok || fn.Pkg() != a.pass.Pkg {
		return nil, false
	}
	if _, ok := a.funcs[fn]; !ok {
		return nil, false
	}
	return fn, true
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

func (a *analyzer) assign(lhs ast.Expr, val value, s state) {
	v := a.varOf(lhs)
	if v == nil || !a.track(v) {
		return
	}
	if isPointer(v.Type()) {
		if val.nilness == unknown {
			delete(s.nilness, v)
		} else {
			s.nilness[v] = val.nilness
		}
	}
	if isInteger(v.Type()) {
		if val.hasInt {
			s.ints[v] = val.intVal
		} else {
			delete(s.ints, v)
		}
	}
	if isFunc(v.Type()) {
		if val.fn != nil {
			s.funcs[v] = val.fn
		} else {
			delete(s.funcs, v)
		}
	}
}

// exprInt returns the constant integer value of an expression when it can be
// established from a constant expression or a tracked local variable.
func (a *analyzer) exprInt(expr ast.Expr, s state) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.Int {
		return constant.Int64Val(tv.Value)
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.exprInt(e.X, s)
	case *ast.Ident:
		if v := a.varOf(e); v != nil && a.track(v) {
			x, ok := s.ints[v]
			return x, ok
		}
	case *ast.UnaryExpr:
		if e.Op == token.ADD || e.Op == token.SUB {
			x, ok := a.exprInt(e.X, s)
			if !ok {
				return 0, false
			}
			if e.Op == token.SUB {
				return -x, true
			}
			return x, true
		}
	case *ast.BinaryExpr:
		x, xok := a.exprInt(e.X, s)
		y, yok := a.exprInt(e.Y, s)
		if xok && yok {
			switch e.Op {
			case token.ADD:
				return x + y, true
			case token.SUB:
				return x - y, true
			case token.MUL:
				return x * y, true
			case token.QUO:
				if y != 0 {
					return x / y, true
				}
			case token.REM:
				if y != 0 {
					return x % y, true
				}
			case token.AND:
				return x & y, true
			case token.OR:
				return x | y, true
			case token.XOR:
				return x ^ y, true
			case token.SHL:
				return x << uint64(y), true
			case token.SHR:
				return x >> uint64(y), true
			}
		}
	}
	return 0, false
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
		s.nilness[v] = isNil
	} else {
		s.nilness[v] = nonNil
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
	out := newState()
	for v, n := range s.nilness {
		out.nilness[v] = n
	}
	for v, n := range s.ints {
		out.ints[v] = n
	}
	for v, n := range s.funcs {
		out.funcs[v] = n
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
	for v, bn := range b.nilness {
		an, ok := out.nilness[v]
		if !ok {
			an = unknown
		}
		out.nilness[v] = join(an, bn)
	}
	for v, an := range out.nilness {
		if _, ok := b.nilness[v]; !ok {
			out.nilness[v] = join(an, unknown)
		}
	}
	for v, an := range out.ints {
		if bv, ok := b.ints[v]; !ok || bv != an {
			delete(out.ints, v)
		}
	}
	for v, an := range out.funcs {
		if bv, ok := b.funcs[v]; !ok || bv != an {
			delete(out.funcs, v)
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

func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}

// evalEnv is a lightweight environment for evaluating a call's result nilness
// path-sensitively. It tracks pointer nilness and integer constants for the
// function being evaluated. It is separate from the intraprocedural state so
// that evaluation cannot disturb the main analysis.
type evalEnv struct {
	nilness map[*types.Var]nilness
	ints    map[*types.Var]int64
	funcs   map[*types.Var]*types.Func
}

func newEvalEnv() evalEnv {
	return evalEnv{
		nilness: make(map[*types.Var]nilness),
		ints:    make(map[*types.Var]int64),
		funcs:   make(map[*types.Var]*types.Func),
	}
}

func (e evalEnv) clone() evalEnv {
	out := newEvalEnv()
	for v, n := range e.nilness {
		out.nilness[v] = n
	}
	for v, n := range e.ints {
		out.ints[v] = n
	}
	for v, n := range e.funcs {
		out.funcs[v] = n
	}
	return out
}

// evalCall evaluates the nilness of a call's single pointer result for the
// given caller environment, using the call's actual arguments. It is
// context-sensitive: the same function called with different arguments can
// produce different results, and branch conditions that are decidable from the
// bound arguments are pruned. Recursive cycles yield unknown.
func (a *analyzer) evalCall(call *ast.CallExpr, caller evalEnv) nilness {
	fn := a.calleeFunc(call, caller.funcs)
	if fn == nil {
		return unknown
	}
	resultVar, ok := pointerResult(fn)
	if !ok {
		return unknown
	}
	decl := a.funcs[fn]
	if decl == nil || decl.Body == nil {
		return unknown
	}
	if a.evaluating[fn] {
		return unknown
	}

	env := a.seedEnv(fn, call, caller)
	if resultVar.Name() != "" {
		env.nilness[resultVar] = isNil
	}

	prevEscaped := a.escaped
	prevResult := a.resultVar
	prevUnknown := a.evalUnknown

	a.evaluating[fn] = true
	a.evalUnknown = false
	a.escaped = a.findEscaped(decl.Body)
	a.resultVar = resultVar

	returns := a.evalBlock(decl.Body, &env)
	decided := !a.evalUnknown

	a.escaped = prevEscaped
	a.resultVar = prevResult
	a.evalUnknown = prevUnknown
	delete(a.evaluating, fn)

	if !decided {
		return unknown
	}
	return joinNilness(returns)
}

// seedEnv binds the call's arguments to the callee's parameters.
func (a *analyzer) seedEnv(fn *types.Func, call *ast.CallExpr, caller evalEnv) evalEnv {
	env := newEvalEnv()
	params := a.paramVars(fn)
	for i, arg := range call.Args {
		if i >= len(params) {
			break
		}
		p := params[i]
		if p == nil {
			continue
		}
		if isPointer(p.Type()) {
			if n := a.evalNil(arg, caller); n != unknown {
				env.nilness[p] = n
			}
			continue
		}
		if isInteger(p.Type()) {
			if v, ok := a.evalInt(arg, caller); ok {
				env.ints[p] = v
			}
			continue
		}
		if isFunc(p.Type()) {
			if fn, ok := a.exprFunc(arg); ok {
				env.funcs[p] = fn
			}
		}
	}
	return env
}

func (a *analyzer) evalBlock(block *ast.BlockStmt, env *evalEnv) []nilness {
	var returns []nilness
	a.evalBlockInto(block, env, &returns)
	return returns
}

// evalBlockInto evaluates statements in order and reports whether the block
// definitely ends the flow (so following statements are unreachable).
func (a *analyzer) evalBlockInto(block *ast.BlockStmt, env *evalEnv, returns *[]nilness) bool {
	for _, stmt := range block.List {
		if a.evalStmt(stmt, env, returns) {
			return true
		}
	}
	return false
}

// evalStmt evaluates one statement and reports whether it definitely ends the
// flow.
func (a *analyzer) evalStmt(stmt ast.Stmt, env *evalEnv, returns *[]nilness) bool {
	switch st := stmt.(type) {
	case *ast.AssignStmt:
		a.evalAssign(st, env)
	case *ast.DeclStmt:
		a.evalDecl(st, env)
	case *ast.IncDecStmt:
		if v := a.varOf(st.X); v != nil {
			delete(env.ints, v)
		}
	case *ast.ExprStmt:
		// The value is discarded; no effect on the environment.
	case *ast.ReturnStmt:
		*returns = append(*returns, a.evalReturn(st, env))
		return true
	case *ast.BranchStmt:
		return true
	case *ast.IfStmt:
		return a.evalIf(st, env, returns)
	case *ast.BlockStmt:
		return a.evalBlockInto(st, env, returns)
	case *ast.LabeledStmt:
		return a.evalStmt(st.Stmt, env, returns)
	default:
		// Loops, switch, select and goto are not modeled. Give up on the call
		// rather than guess.
		a.evalUnknown = true
	}
	return false
}

func (a *analyzer) evalIf(st *ast.IfStmt, env *evalEnv, returns *[]nilness) bool {
	if st.Init != nil {
		a.evalStmt(st.Init, env, returns)
	}
	if v, known := a.evalCond(st.Cond, *env); known {
		if v {
			return a.evalBlockInto(st.Body, env, returns)
		}
		if st.Else != nil {
			return a.evalStmt(st.Else, env, returns)
		}
		return false
	}
	thenEnv := env.clone()
	elseEnv := env.clone()
	thenTerm := a.evalBlockInto(st.Body, &thenEnv, returns)
	elseTerm := false
	if st.Else != nil {
		elseTerm = a.evalStmt(st.Else, &elseEnv, returns)
	}
	*env = mergeEval(thenEnv, elseEnv)
	return thenTerm && elseTerm
}

func (a *analyzer) evalAssign(st *ast.AssignStmt, env *evalEnv) {
	if (st.Tok == token.DEFINE || st.Tok == token.ASSIGN) && len(st.Lhs) == len(st.Rhs) {
		for i, lhs := range st.Lhs {
			a.evalAssignTo(lhs, st.Rhs[i], env)
		}
		return
	}
	for _, lhs := range st.Lhs {
		a.evalForget(lhs, env)
	}
}

func (a *analyzer) evalAssignTo(lhs, rhs ast.Expr, env *evalEnv) {
	v := a.varOf(lhs)
	if v == nil || !a.track(v) {
		return
	}
	if isPointer(v.Type()) {
		if n := a.evalNil(rhs, *env); n != unknown {
			env.nilness[v] = n
		} else {
			delete(env.nilness, v)
		}
	}
	if isInteger(v.Type()) {
		if x, ok := a.evalInt(rhs, *env); ok {
			env.ints[v] = x
		} else {
			delete(env.ints, v)
		}
	}
	if isFunc(v.Type()) {
		if fn, ok := a.exprFunc(rhs); ok {
			env.funcs[v] = fn
		} else {
			delete(env.funcs, v)
		}
	}
}

func (a *analyzer) evalForget(lhs ast.Expr, env *evalEnv) {
	if v := a.varOf(lhs); v != nil {
		delete(env.nilness, v)
		delete(env.ints, v)
		delete(env.funcs, v)
	}
}

func (a *analyzer) evalDecl(st *ast.DeclStmt, env *evalEnv) {
	gen, ok := st.Decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.VAR {
		return
	}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if len(vs.Values) == 0 {
			for _, name := range vs.Names {
				v := a.varOf(name)
				if v == nil || !a.track(v) {
					continue
				}
				if isPointer(v.Type()) {
					env.nilness[v] = isNil
				}
				if isInteger(v.Type()) {
					env.ints[v] = 0
				}
			}
			continue
		}
		if len(vs.Values) == len(vs.Names) {
			for i, name := range vs.Names {
				a.evalAssignTo(name, vs.Values[i], env)
			}
			continue
		}
		for _, name := range vs.Names {
			a.evalForget(name, env)
		}
	}
}

func (a *analyzer) evalReturn(st *ast.ReturnStmt, env *evalEnv) nilness {
	if len(st.Results) == 1 {
		return a.evalNil(st.Results[0], *env)
	}
	if len(st.Results) == 0 && a.resultVar != nil && a.resultVar.Name() != "" {
		if n, ok := env.nilness[a.resultVar]; ok {
			return n
		}
	}
	return unknown
}

func (a *analyzer) evalNil(expr ast.Expr, env evalEnv) nilness {
	if expr == nil {
		return unknown
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.IsNil() {
		return isNil
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.evalNil(e.X, env)
	case *ast.Ident:
		if v := a.varOf(e); v != nil && a.track(v) {
			if n, ok := env.nilness[v]; ok {
				return n
			}
		}
		return unknown
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
		return a.evalCall(e, env)
	}
	return unknown
}

func (a *analyzer) evalInt(expr ast.Expr, env evalEnv) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.Int {
		return constant.Int64Val(tv.Value)
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.evalInt(e.X, env)
	case *ast.Ident:
		if v := a.varOf(e); v != nil && a.track(v) {
			x, ok := env.ints[v]
			return x, ok
		}
	case *ast.UnaryExpr:
		if e.Op == token.ADD || e.Op == token.SUB {
			x, ok := a.evalInt(e.X, env)
			if !ok {
				return 0, false
			}
			if e.Op == token.SUB {
				return -x, true
			}
			return x, true
		}
	case *ast.BinaryExpr:
		x, xok := a.evalInt(e.X, env)
		y, yok := a.evalInt(e.Y, env)
		if xok && yok {
			switch e.Op {
			case token.ADD:
				return x + y, true
			case token.SUB:
				return x - y, true
			case token.MUL:
				return x * y, true
			case token.QUO:
				if y != 0 {
					return x / y, true
				}
			case token.REM:
				if y != 0 {
					return x % y, true
				}
			case token.AND:
				return x & y, true
			case token.OR:
				return x | y, true
			case token.XOR:
				return x ^ y, true
			case token.SHL:
				return x << uint64(y), true
			case token.SHR:
				return x >> uint64(y), true
			}
		}
	}
	return 0, false
}

func (a *analyzer) evalCond(expr ast.Expr, env evalEnv) (bool, bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.evalCond(e.X, env)
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			if v, ok := a.evalCond(e.X, env); ok {
				return !v, true
			}
		}
	case *ast.BinaryExpr:
		switch e.Op {
		case token.LAND:
			l, lok := a.evalCond(e.X, env)
			if lok && !l {
				return false, true
			}
			r, rok := a.evalCond(e.Y, env)
			if rok && !r {
				return false, true
			}
			if lok && rok {
				return l && r, true
			}
		case token.LOR:
			l, lok := a.evalCond(e.X, env)
			if lok && l {
				return true, true
			}
			r, rok := a.evalCond(e.Y, env)
			if rok && r {
				return true, true
			}
			if lok && rok {
				return l || r, true
			}
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			x, xok := a.evalInt(e.X, env)
			y, yok := a.evalInt(e.Y, env)
			if xok && yok {
				return compareInts(e.Op, x, y), true
			}
		}
	}
	return false, false
}

func compareInts(op token.Token, x, y int64) bool {
	switch op {
	case token.EQL:
		return x == y
	case token.NEQ:
		return x != y
	case token.LSS:
		return x < y
	case token.LEQ:
		return x <= y
	case token.GTR:
		return x > y
	case token.GEQ:
		return x >= y
	}
	return false
}

func mergeEval(a, b evalEnv) evalEnv {
	out := newEvalEnv()
	for v, an := range a.nilness {
		bn := b.nilness[v]
		out.nilness[v] = join(an, bn)
	}
	for v, an := range a.ints {
		if bn, ok := b.ints[v]; ok && bn == an {
			out.ints[v] = an
		}
	}
	for v, an := range a.funcs {
		if bn, ok := b.funcs[v]; ok && bn == an {
			out.funcs[v] = an
		}
	}
	return out
}
