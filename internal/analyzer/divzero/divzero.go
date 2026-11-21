// Package divzero reports integer division or modulo by a divisor that is
// provably zero.
//
// The analysis is intraprocedural and conservative. It tracks local integer
// variables through declarations, assignments, nil-like zero checks and
// branches. Values whose zero-ness cannot be established are treated as
// unknown. Floating-point and complex division are ignored because they do not
// panic.
package divzero

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports integer division or modulo where the divisor is provably
// zero within the enclosing function.
var Analyzer = &analysis.Analyzer{
	Name: "divzero",
	Doc:  "report integer division or modulo by a divisor that is provably zero",
	Run:  run,
}

type zeroState uint8

const (
	unknown zeroState = iota
	isZero
	nonZero
)

type state map[*types.Var]zeroState

type analyzer struct {
	pass     *analysis.Pass
	escaped  map[*types.Var]struct{}
	funcs    map[*types.Func]*ast.FuncDecl
	reported map[token.Pos]bool
	depth    int
}

func run(pass *analysis.Pass) (any, error) {
	a := &analyzer{
		pass:     pass,
		funcs:    make(map[*types.Func]*ast.FuncDecl),
		reported: make(map[token.Pos]bool),
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
// values seeded into its integer parameters, so a division or modulo by a
// parameter is reported even though the callee is a separate function. It runs
// only at the top level so that analysis does not recurse transitively through
// nested calls.
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
		if p == nil || !isInteger(p.Type()) {
			continue
		}
		n := a.exprZero(arg, s)
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

// findEscaped identifies local variables that can change without an explicit
// assignment in the analyzed function. Address-taken and closure-captured
// variables are excluded from tracking.
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

// initResults marks named integer results as zero: they start with their zero
// value and are therefore zero until assigned.
func (a *analyzer) initResults(ft *ast.FuncType, s state) {
	if ft.Results == nil {
		return
	}
	for _, field := range ft.Results.List {
		for _, name := range field.Names {
			if v := a.varOf(name); v != nil && a.track(v) && isInteger(v.Type()) {
				s[v] = isZero
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
		a.assign(st.X, unknown, s)
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

	out := clone(s)
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
		out = merge(out, a.analyzeStmtList(cc.Body, cs))
	}
	return out
}

// refineDefault determines whether the default clause is reachable. A zero tag
// with a case that definitely matches zero makes the default clause
// unreachable.
func (a *analyzer) refineDefault(st *ast.SwitchStmt, s state) (state, bool) {
	if st.Tag == nil {
		return s, true
	}
	tag := a.varOf(st.Tag)
	if tag == nil || !a.track(tag) || !isInteger(tag.Type()) || s[tag] != isZero {
		return s, true
	}
	for _, clause := range st.Body.List {
		cc, ok := clause.(*ast.CaseClause)
		if !ok || len(cc.List) == 0 {
			continue
		}
		for _, expr := range cc.List {
			if a.exprZero(expr, s) == isZero {
				return s, false
			}
		}
	}
	return s, true
}

// refineCase narrows the state for a case clause and reports whether the case
// body is reachable. A tag switch does not change the tag's value, so the tag
// keeps its pre-switch state unless the case definitely cannot match.
func (a *analyzer) refineCase(st *ast.SwitchStmt, cc *ast.CaseClause, s state) bool {
	if st.Tag == nil {
		if len(cc.List) == 1 {
			a.refine(cc.List[0], true, s)
		}
		return true
	}
	tag := a.varOf(st.Tag)
	if tag == nil || !a.track(tag) || !isInteger(tag.Type()) {
		return true
	}

	tagState := s[tag]
	caseState := a.caseZero(cc, s)
	if tagState != unknown && caseState != unknown && tagState != caseState {
		return false
	}
	if tagState == isZero && caseState == unknown {
		s[tag] = unknown
	}
	return true
}

func (a *analyzer) caseZero(cc *ast.CaseClause, s state) zeroState {
	first := unknown
	for i, expr := range cc.List {
		n := a.exprZero(expr, s)
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
		a.analyzeExpr(lhs, s)
	}
	for _, rhs := range st.Rhs {
		a.analyzeExpr(rhs, s)
	}

	if st.Tok == token.QUO_ASSIGN || st.Tok == token.REM_ASSIGN {
		if len(st.Rhs) == 1 {
			a.checkDivisor(st.Tok, st.Rhs[0], st.Pos(), s)
		}
	}

	// Compound assignments depend on the previous value, so the result is not
	// known from the right-hand side alone.
	if st.Tok != token.ASSIGN && st.Tok != token.DEFINE {
		for _, lhs := range st.Lhs {
			a.assign(lhs, unknown, s)
		}
		return s
	}

	if len(st.Lhs) == len(st.Rhs) {
		values := make([]zeroState, len(st.Rhs))
		for i, rhs := range st.Rhs {
			values[i] = a.exprZero(rhs, s)
		}
		for i, lhs := range st.Lhs {
			a.assign(lhs, values[i], s)
		}
		return s
	}

	// One right-hand side feeding several variables (a call, map lookup, type
	// assertion, and so on) provides no definite zero-ness information.
	for _, lhs := range st.Lhs {
		a.assign(lhs, unknown, s)
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
				if v := a.varOf(name); v != nil && a.track(v) && isInteger(v.Type()) {
					s[v] = isZero
				}
			}
			continue
		}
		for _, val := range vs.Values {
			a.analyzeExpr(val, s)
		}
		if len(vs.Values) == len(vs.Names) {
			values := make([]zeroState, len(vs.Values))
			for i, val := range vs.Values {
				values[i] = a.exprZero(val, s)
			}
			for i, name := range vs.Names {
				if v := a.varOf(name); v != nil && a.track(v) && isInteger(v.Type()) {
					s[v] = values[i]
				}
			}
			continue
		}
		for _, name := range vs.Names {
			if v := a.varOf(name); v != nil && a.track(v) && isInteger(v.Type()) {
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
	switch e := expr.(type) {
	case *ast.ParenExpr:
		a.analyzeExpr(e.X, s)
	case *ast.BinaryExpr:
		if e.Op == token.QUO || e.Op == token.REM {
			a.checkDivisor(e.Op, e.Y, e.Pos(), s)
		}
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Y, s)
	case *ast.UnaryExpr:
		a.analyzeExpr(e.X, s)
	case *ast.StarExpr:
		a.analyzeExpr(e.X, s)
	case *ast.SelectorExpr:
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
	case *ast.CallExpr:
		a.analyzeExpr(e.Fun, s)
		for _, arg := range e.Args {
			a.analyzeExpr(arg, s)
		}
		a.propagateCall(e, s)
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

func (a *analyzer) checkDivisor(op token.Token, divisor ast.Expr, pos token.Pos, s state) {
	if !isInteger(a.typeOf(divisor)) || a.exprZero(divisor, s) != isZero {
		return
	}
	kind := "division"
	if op == token.REM || op == token.REM_ASSIGN {
		kind = "modulo"
	}
	if a.reported[pos] {
		return
	}
	a.reported[pos] = true
	a.pass.Reportf(pos, "%s by zero: %s is zero", kind, types.ExprString(divisor))
}

func (a *analyzer) exprZero(expr ast.Expr, s state) zeroState {
	if expr == nil {
		return unknown
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.Value != nil {
		if tv.Value.Kind() != constant.Int {
			return unknown
		}
		if constant.Sign(tv.Value) == 0 {
			return isZero
		}
		return nonZero
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.exprZero(e.X, s)
	case *ast.Ident:
		if v := a.varOf(e); v != nil && a.track(v) {
			return s[v]
		}
	case *ast.UnaryExpr:
		if e.Op == token.ADD || e.Op == token.SUB {
			n := a.exprZero(e.X, s)
			if n == isZero || n == nonZero {
				return n
			}
		}
	}
	return unknown
}

func (a *analyzer) isZeroExpr(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	tv, ok := a.pass.TypesInfo.Types[expr]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
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

func isInteger(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsInteger != 0
}

func (a *analyzer) assign(lhs ast.Expr, n zeroState, s state) {
	v := a.varOf(lhs)
	if v == nil || !a.track(v) || !isInteger(v.Type()) {
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
			a.refineZeroComparison(e, truth, s)
		}
	}
}

func (a *analyzer) refineZeroComparison(e *ast.BinaryExpr, truth bool, s state) {
	var other ast.Expr
	switch {
	case a.isZeroExpr(e.X):
		other = e.Y
	case a.isZeroExpr(e.Y):
		other = e.X
	default:
		return
	}
	v := a.varOf(other)
	if v == nil || !a.track(v) || !isInteger(v.Type()) {
		return
	}
	equal := e.Op == token.EQL
	if truth == equal {
		s[v] = isZero
	} else {
		s[v] = nonZero
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

func merge(a, b state) state {
	out := clone(a)
	for v, bn := range b {
		an, ok := out[v]
		if !ok || an != bn {
			out[v] = unknown
		}
	}
	for v, an := range a {
		if _, ok := b[v]; !ok && an != unknown {
			out[v] = unknown
		}
	}
	return out
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
