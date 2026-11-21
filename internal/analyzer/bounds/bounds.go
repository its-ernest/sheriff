// Package bounds reports indexing and slicing operations that are provably out
// of range.
//
// The analysis is intraprocedural and conservative. It tracks local integer
// variables whose value is a constant, and the length of local arrays, slices
// and strings. A diagnostic is produced only when the operation is out of
// range on every path that reaches it. Values whose range cannot be
// established are treated as unknown rather than guessed.
package bounds

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports indexing and slicing operations that are provably out of
// range within the enclosing function.
var Analyzer = &analysis.Analyzer{
	Name: "bounds",
	Doc:  "report indexing or slicing that is provably out of range",
	Run:  run,
}

type state struct {
	ints map[*types.Var]constant.Value
	lens map[*types.Var]int64
}

type analyzer struct {
	pass    *analysis.Pass
	escaped map[*types.Var]struct{}
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
	s := newState()
	a.initResults(ft, s)
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

// initResults marks named results with their zero value: integers are zero and
// slices and strings are empty.
func (a *analyzer) initResults(ft *ast.FuncType, s *state) {
	if ft.Results == nil {
		return
	}
	for _, field := range ft.Results.List {
		for _, name := range field.Names {
			v := a.varOf(name)
			if v == nil || !a.track(v) {
				continue
			}
			if isInteger(v.Type()) {
				s.ints[v] = constant.MakeInt64(0)
			}
			if isSequenceVar(v.Type()) {
				s.lens[v] = 0
			}
		}
	}
}

func (a *analyzer) analyzeBlock(block *ast.BlockStmt, s *state) *state {
	return a.analyzeStmtList(block.List, s)
}

func (a *analyzer) analyzeStmtList(stmts []ast.Stmt, s *state) *state {
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

func (a *analyzer) analyzeStmt(stmt ast.Stmt, s *state) *state {
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
		a.assign(st.X, nil, false, 0, false, s)
	case *ast.LabeledStmt:
		return a.analyzeStmt(st.Stmt, s)
	}
	return s
}

func (a *analyzer) analyzeIf(st *ast.IfStmt, s *state) *state {
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

func (a *analyzer) analyzeFor(st *ast.ForStmt, s *state) *state {
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

func (a *analyzer) analyzeRange(st *ast.RangeStmt, s *state) *state {
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

func (a *analyzer) analyzeSwitch(st *ast.SwitchStmt, s *state) *state {
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

func (a *analyzer) analyzeTypeSwitch(st *ast.TypeSwitchStmt, s *state) *state {
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

func (a *analyzer) analyzeSelect(st *ast.SelectStmt, s *state) *state {
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

func (a *analyzer) analyzeAssign(st *ast.AssignStmt, s *state) *state {
	for _, lhs := range st.Lhs {
		a.analyzeExpr(lhs, s)
	}
	for _, rhs := range st.Rhs {
		a.analyzeExpr(rhs, s)
	}

	if (st.Tok == token.DEFINE || st.Tok == token.ASSIGN) && len(st.Lhs) == len(st.Rhs) {
		vals := make([]value, len(st.Rhs))
		for i, rhs := range st.Rhs {
			vals[i] = a.eval(rhs, s)
		}
		for i, lhs := range st.Lhs {
			a.assign(lhs, vals[i].intVal, vals[i].hasInt, vals[i].length, vals[i].hasLen, s)
		}
		return s
	}

	// Compound assignments and multiple-value right-hand sides provide no
	// definite value or length information.
	for _, lhs := range st.Lhs {
		a.assign(lhs, nil, false, 0, false, s)
	}
	return s
}

type value struct {
	intVal constant.Value
	hasInt bool
	length int64
	hasLen bool
}

func (a *analyzer) eval(expr ast.Expr, s *state) value {
	v := value{}
	v.intVal, v.hasInt = a.exprInt(expr, s)
	v.length, v.hasLen = a.exprLen(expr, s)
	return v
}

func (a *analyzer) analyzeDecl(st *ast.DeclStmt, s *state) *state {
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
				if isInteger(v.Type()) {
					s.ints[v] = constant.MakeInt64(0)
				}
				if isSequenceVar(v.Type()) {
					s.lens[v] = 0
				}
			}
			continue
		}
		for _, val := range vs.Values {
			a.analyzeExpr(val, s)
		}
		if len(vs.Values) == len(vs.Names) {
			vals := make([]value, len(vs.Values))
			for i, val := range vs.Values {
				vals[i] = a.eval(val, s)
			}
			for i, name := range vs.Names {
				v := a.varOf(name)
				if v == nil || !a.track(v) {
					continue
				}
				a.assign(name, vals[i].intVal, vals[i].hasInt, vals[i].length, vals[i].hasLen, s)
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

func (a *analyzer) analyzeExpr(expr ast.Expr, s *state) {
	if expr == nil {
		return
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.IsType() {
		return
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		a.analyzeExpr(e.X, s)
	case *ast.IndexExpr:
		a.checkIndex(e, s)
		a.analyzeExpr(e.X, s)
		a.analyzeExpr(e.Index, s)
	case *ast.IndexListExpr:
		a.analyzeExpr(e.X, s)
		for _, index := range e.Indices {
			a.analyzeExpr(index, s)
		}
	case *ast.SliceExpr:
		a.checkSlice(e, s)
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

func (a *analyzer) checkIndex(e *ast.IndexExpr, s *state) {
	n, ok := a.exprLen(e.X, s)
	if !ok {
		return
	}
	v, ok := a.exprInt(e.Index, s)
	if !ok {
		return
	}
	idx, ok := constant.Int64Val(v)
	if !ok {
		return
	}
	if idx < 0 || idx >= n {
		a.pass.Reportf(e.Pos(), "index out of range: index %s for length %d", v.String(), n)
	}
}

func (a *analyzer) checkSlice(e *ast.SliceExpr, s *state) {
	kind := a.sequenceKind(e.X)
	if kind == seqNone {
		return
	}
	n, hasLen := a.exprLen(e.X, s)

	low, hasLow := a.constInt64(e.Low, s)
	high, hasHigh := a.constInt64(e.High, s)

	if hasLow && low < 0 {
		a.pass.Reportf(e.Pos(), "slice bounds out of range: low index %d is negative", low)
		return
	}
	if hasHigh && high < 0 {
		a.pass.Reportf(e.Pos(), "slice bounds out of range: high index %d is negative", high)
		return
	}

	// An omitted high bound defaults to the length.
	if !hasHigh && hasLen {
		high, hasHigh = n, true
	}
	if hasLow && hasHigh && low > high {
		a.pass.Reportf(e.Pos(), "slice bounds out of range: low index %d is greater than high index %d", low, high)
		return
	}

	// For slices an explicit high bound may extend up to the capacity, which
	// is not tracked. Arrays and strings are bounded by their length.
	if kind != seqSlice && hasLen && hasHigh && high > n {
		a.pass.Reportf(e.Pos(), "slice bounds out of range: high index %d is greater than length %d", high, n)
		return
	}

	if e.Max != nil {
		max, hasMax := a.constInt64(e.Max, s)
		if hasMax && hasHigh && max < high {
			a.pass.Reportf(e.Pos(), "slice bounds out of range: max index %d is less than high index %d", max, high)
			return
		}
		if hasMax && hasLen && kind == seqArray && max > n {
			a.pass.Reportf(e.Pos(), "slice bounds out of range: max index %d is greater than length %d", max, n)
		}
	}
}

// exprInt returns the constant value of an integer expression when it can be
// established from a constant expression or a tracked local variable.
func (a *analyzer) exprInt(expr ast.Expr, s *state) (constant.Value, bool) {
	if expr == nil {
		return nil, false
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.Int {
		return tv.Value, true
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return a.exprInt(e.X, s)
	case *ast.Ident:
		if v := a.varOf(e); v != nil && a.track(v) {
			val, ok := s.ints[v]
			return val, ok
		}
	case *ast.UnaryExpr:
		if e.Op == token.ADD || e.Op == token.SUB {
			val, ok := a.exprInt(e.X, s)
			if !ok {
				return nil, false
			}
			if e.Op == token.SUB {
				return constant.UnaryOp(token.SUB, val, 0), true
			}
			return val, true
		}
	}
	return nil, false
}

// exprLen returns the length of an array, slice or string expression when it
// can be established statically.
func (a *analyzer) exprLen(expr ast.Expr, s *state) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	if tv, ok := a.pass.TypesInfo.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return int64(len(constant.StringVal(tv.Value))), true
	}

	if t := a.typeOf(expr); t != nil {
		if p, ok := t.Underlying().(*types.Pointer); ok {
			t = p.Elem()
		}
		if arr, ok := t.Underlying().(*types.Array); ok {
			return arr.Len(), true
		}
	}

	switch e := unparen(expr).(type) {
	case *ast.Ident:
		if v := a.varOf(e); v != nil && a.track(v) {
			n, ok := s.lens[v]
			return n, ok
		}
	case *ast.CompositeLit:
		if t := a.typeOf(e); t != nil {
			if _, ok := t.Underlying().(*types.Slice); ok {
				return a.compositeLen(e, s)
			}
		}
	case *ast.CallExpr:
		return a.makeLen(e, s)
	case *ast.SliceExpr:
		return a.sliceLen(e, s)
	}
	return 0, false
}

func (a *analyzer) compositeLen(e *ast.CompositeLit, s *state) (int64, bool) {
	var max int64 = -1
	for _, elt := range e.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			v, ok := a.exprInt(kv.Key, s)
			if !ok {
				return 0, false
			}
			idx, ok := constant.Int64Val(v)
			if !ok || idx < 0 {
				return 0, false
			}
			if idx > max {
				max = idx
			}
			continue
		}
		max++
	}
	return max + 1, true
}

func (a *analyzer) makeLen(call *ast.CallExpr, s *state) (int64, bool) {
	id, ok := unparen(call.Fun).(*ast.Ident)
	if !ok || id.Name != "make" {
		return 0, false
	}
	b, ok := a.pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok || b.Name() != "make" || len(call.Args) < 2 {
		return 0, false
	}
	t := a.typeOf(call)
	if t == nil {
		return 0, false
	}
	if _, ok := t.Underlying().(*types.Slice); !ok {
		return 0, false
	}
	v, ok := a.exprInt(call.Args[1], s)
	if !ok {
		return 0, false
	}
	n, ok := constant.Int64Val(v)
	if !ok || n < 0 {
		return 0, false
	}
	return n, true
}

func (a *analyzer) sliceLen(e *ast.SliceExpr, s *state) (int64, bool) {
	n, ok := a.exprLen(e.X, s)
	if !ok {
		return 0, false
	}
	low := int64(0)
	if e.Low != nil {
		var ok bool
		low, ok = a.constInt64(e.Low, s)
		if !ok || low < 0 {
			return 0, false
		}
	}
	high := n
	if e.High != nil {
		var ok bool
		high, ok = a.constInt64(e.High, s)
		if !ok || high < low || high > n {
			return 0, false
		}
	}
	return high - low, true
}

func (a *analyzer) constInt64(expr ast.Expr, s *state) (int64, bool) {
	if expr == nil {
		return 0, false
	}
	v, ok := a.exprInt(expr, s)
	if !ok {
		return 0, false
	}
	return constant.Int64Val(v)
}

type seqKind uint8

const (
	seqNone seqKind = iota
	seqArray
	seqSlice
	seqString
)

func (a *analyzer) sequenceKind(expr ast.Expr) seqKind {
	t := a.typeOf(expr)
	if t == nil {
		return seqNone
	}
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	switch u := t.Underlying().(type) {
	case *types.Array:
		return seqArray
	case *types.Slice:
		return seqSlice
	case *types.Basic:
		if u.Info()&types.IsString != 0 {
			return seqString
		}
	}
	return seqNone
}

func (a *analyzer) assign(lhs ast.Expr, intVal constant.Value, hasInt bool, length int64, hasLen bool, s *state) {
	v := a.varOf(lhs)
	if v == nil || !a.track(v) {
		return
	}
	if isInteger(v.Type()) {
		if hasInt {
			s.ints[v] = intVal
		} else {
			delete(s.ints, v)
		}
	}
	if isSequenceVar(v.Type()) {
		if hasLen {
			s.lens[v] = length
		} else {
			delete(s.lens, v)
		}
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

func isInteger(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsInteger != 0
}

func isSequenceVar(t types.Type) bool {
	if t == nil {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return true
	case *types.Basic:
		return u.Info()&types.IsString != 0
	}
	return false
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

func newState() *state {
	return &state{
		ints: make(map[*types.Var]constant.Value),
		lens: make(map[*types.Var]int64),
	}
}

func clone(s *state) *state {
	out := &state{
		ints: make(map[*types.Var]constant.Value, len(s.ints)),
		lens: make(map[*types.Var]int64, len(s.lens)),
	}
	for v, n := range s.ints {
		out.ints[v] = n
	}
	for v, n := range s.lens {
		out.lens[v] = n
	}
	return out
}

func merge(a, b *state) *state {
	out := clone(a)
	for v, bv := range b.ints {
		av, ok := out.ints[v]
		if !ok || constant.Compare(av, token.NEQ, bv) {
			delete(out.ints, v)
		}
	}
	for v := range out.ints {
		if _, ok := b.ints[v]; !ok {
			delete(out.ints, v)
		}
	}
	for v, bv := range b.lens {
		av, ok := out.lens[v]
		if !ok || av != bv {
			delete(out.lens, v)
		}
	}
	for v := range out.lens {
		if _, ok := b.lens[v]; !ok {
			delete(out.lens, v)
		}
	}
	return out
}

func forget(s *state, v *types.Var) {
	delete(s.ints, v)
	delete(s.lens, v)
}

func forgetAll(s *state, vars map[*types.Var]struct{}) {
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
