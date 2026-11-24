package nilptr

import "encoding/json"

type T struct {
	Field int
}

func (t *T) Method() {}

func consume(T) {}

func localNilDeref() {
	var p *T
	_ = *p // want `nil pointer dereference: p is nil`
}

func localNilField() {
	var p *T
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func nilConversion() {
	p := (*T)(nil)
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func nonNilValues() {
	p := &T{}
	_ = p.Field
	q := new(T)
	_ = q.Field
	r := &T{}
	r = nil
	_ = r.Field // want `nil pointer dereference: r is nil`
}

func reassignedNonNil() {
	var p *T
	p = &T{}
	_ = p.Field
}

func unknownParam(p *T) {
	_ = p.Field
}

func guardBranch(p *T) {
	if p != nil {
		_ = p.Field
	} else {
		_ = p.Field // want `nil pointer dereference: p is nil`
	}
}

func nilBranch() {
	var p *T
	if p == nil {
		_ = *p // want `nil pointer dereference: p is nil`
	}
}

func mayAssign(cond bool) {
	var p *T
	if cond {
		p = &T{}
	}
	_ = p.Field // want `nil pointer dereference: p may be nil`
}

func bothAssign(cond bool) {
	var p *T
	if cond {
		p = &T{}
	} else {
		p = new(T)
	}
	_ = p.Field
}

func bothNilAfter(cond bool) {
	p := &T{}
	if cond {
		p = nil
	} else {
		p = nil
	}
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func namedResult() (p *T) {
	_ = p.Field // want `nil pointer dereference: p is nil`
	return
}

func pointerArray() {
	var p *[3]int
	_ = p[0]   // want `nil pointer dereference: p is nil`
	_ = p[0:1] // want `nil pointer dereference: p is nil`
}

func nonNilPointerArray() {
	p := &[3]int{}
	_ = p[0]
	_ = p[0:1]
}

func methodValue() {
	var p *T
	p.Method()
}

func nilFuncValue() {
	var f func()
	f()
}

func derefInCall() {
	var p *T
	consume(*p) // want `nil pointer dereference: p is nil`
}

func derefInCondition() {
	var p *T
	if p.Field == 0 { // want `nil pointer dereference: p is nil`
	}
}

func addressTaken() {
	var p *T
	pp := &p
	*pp = &T{}
	_ = p.Field
}

func captured() {
	var p *T
	set := func() { p = &T{} }
	set()
	_ = p.Field
}

func closureLocal() {
	func() {
		var p *T
		_ = p.Field // want `nil pointer dereference: p is nil`
	}()
}

func loopGuard() {
	var p *T
	for p != nil {
		_ = p.Field
	}
}

func loopNil() {
	var p *T
	for i := 0; i < 1; i++ {
		_ = p.Field // want `nil pointer dereference: p is nil`
	}
}

func loopAssignBeforeUse() {
	var p *T
	for i := 0; i < 1; i++ {
		p = &T{}
		_ = p.Field
	}
}

func loopPost(cond bool) {
	var p *T
	for i := 0; i < 1; consume(*p) { // want `nil pointer dereference: p may be nil`
		if cond {
			p = &T{}
		}
		i++
	}
}

func switchGuard() {
	var p *T
	switch {
	case p != nil:
		_ = p.Field
	}
}

func switchNil() {
	var p *T
	switch {
	case p == nil:
		_ = p.Field // want `nil pointer dereference: p is nil`
	}
}

func switchTagNil() {
	var p *T
	switch p {
	case nil:
		_ = p.Field // want `nil pointer dereference: p is nil`
	case &T{}:
		_ = p.Field
	}
}

func switchTagNonNilNilCase() {
	p := &T{}
	switch p {
	case nil:
		_ = p.Field
	}
}

func switchDefaultUnreachable() {
	var p *T
	switch p {
	case nil:
		p = &T{}
	default:
		_ = p.Field
	}
	_ = p.Field
}

func switchDefaultReachable() {
	var p *T
	switch p {
	case &T{}:
	default:
		_ = p.Field // want `nil pointer dereference: p is nil`
	}
}

func shadowedNil() {
	nil := &T{}
	_ = nil.Field
}

func derefParam(p *T) {
	_ = *p // want `nil pointer dereference: p is nil`
}

func callDerefParamWithNil() {
	var p *T
	derefParam(p)
}

func callDerefParamWithNonNil() {
	p := &T{}
	derefParam(p)
}

func callDerefParamWithLiteralNil() {
	derefParam(nil)
}

func callBeforeDecl() {
	var p *T
	laterDefined(p)
}

func laterDefined(p *T) {
	_ = *p // want `nil pointer dereference: p is nil`
}

func fieldParam(p *T) {
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func callFieldParamWithNil() {
	var p *T
	fieldParam(p)
}

func safeCallee(p *T) {
	_ = *p
}

func callSafeCallee() {
	p := &T{}
	safeCallee(p)
}

func guardedCallee(p *T) {
	if p != nil {
		_ = *p
	}
}

func callGuardedCalleeWithNil() {
	var p *T
	guardedCallee(p)
}

func callGuardedCalleeWithNonNil() {
	p := &T{}
	guardedCallee(p)
}

func unknownCallee(p *T) {
	_ = *p
}

func passUnknownThrough(p *T) {
	unknownCallee(p)
}

func alwaysNil() *T { return nil }

func callAlwaysNil() {
	p := alwaysNil()
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func alwaysNonNil() *T { return &T{} }

func callAlwaysNonNil() {
	p := alwaysNonNil()
	_ = p.Field
}

func maybeNilBranch(valid bool) *T {
	if valid {
		return &T{}
	}
	return nil
}

func callMaybeNilBranch() {
	p := maybeNilBranch(true)
	_ = p.Field // want `nil pointer dereference: p may be nil`
}

func propagateMaybeNil() *T {
	return maybeNilBranch(false)
}

func callPropagateMaybeNil() {
	p := propagateMaybeNil()
	_ = p.Field // want `nil pointer dereference: p may be nil`
}

func namedResultNil() (p *T) {
	return
}

func callNamedResultNil() {
	p := namedResultNil()
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func namedResultSet() (p *T) {
	p = &T{}
	return
}

func callNamedResultSet() {
	p := namedResultSet()
	_ = p.Field
}

func unreachableReturn() *T {
	return &T{}
	return nil
}

func callUnreachableReturn() {
	p := unreachableReturn()
	_ = p.Field
}

func recursive(n int) *T {
	if n <= 0 {
		return nil
	}
	return recursive(n - 1)
}

func callRecursive() {
	p := recursive(1)
	_ = p.Field
}

func mutualA(n int) *T {
	if n <= 0 {
		return nil
	}
	return mutualB(n - 1)
}

func mutualB(n int) *T {
	if n <= 0 {
		return &T{}
	}
	return mutualA(n - 1)
}

func callMutual() {
	p := mutualA(1)
	_ = p.Field
}

func callUnknownSource(f func() *T) {
	p := f()
	_ = p.Field
}

type Request struct{ Body []byte }

func bindConfig(req *Request) *T {
	var cfg T
	if err := json.Unmarshal(req.Body, &cfg); err != nil {
		return nil
	}
	return &cfg
}

func prepareConfig(req *Request) *T {
	cfg := bindConfig(req)
	return cfg
}

func startServer(cfg *T) {
	_ = cfg.Field // want `nil pointer dereference: cfg may be nil`
}

func integrationMalformedJSON() {
	req := &Request{Body: []byte(`{"port": "invalid"}`)}
	cfg := prepareConfig(req)
	startServer(cfg)
}

func readID() int { return 10 }

func classify(id int) *T {
	if id > 0 {
		return &T{}
	}
	return nil
}

func callClassifyPositive() {
	p := classify(10)
	_ = p.Field
}

func callClassifyZero() {
	p := classify(0)
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func callClassifyNegative() {
	p := classify(-5)
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func callClassifyUnknown() {
	p := classify(readID())
	_ = p.Field // want `nil pointer dereference: p may be nil`
}

func classifyGE(id int) *T {
	if id >= 1 {
		return &T{}
	}
	return nil
}

func callClassifyGEOne() {
	p := classifyGE(1)
	_ = p.Field
}

func callClassifyGEZero() {
	p := classifyGE(0)
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func callClassifyLocalConst() {
	id := 10
	p := classify(id)
	_ = p.Field
}

func callClassifyLocalZero() {
	id := 0
	p := classify(id)
	_ = p.Field // want `nil pointer dereference: p is nil`
}

func callClassifyMultiAssignConst() {
	id, _ := 1, 1
	p := classify(id)
	_ = p.Field
}

func nilFactory() *T { return nil }

func functionValueNil() {
	factory := nilFactory
	cfg := factory()
	_ = cfg.Field // want `nil pointer dereference: cfg is nil`
}

func nonNilFactory() *T { return &T{} }

func functionValueNonNil() {
	factory := nonNilFactory
	cfg := factory()
	_ = cfg.Field
}

func functionValueReassigned() {
	factory := nonNilFactory
	factory = nilFactory
	cfg := factory()
	_ = cfg.Field // want `nil pointer dereference: cfg is nil`
}

func functionValueUnknown() {
	var factory func() *T
	cfg := factory()
	_ = cfg.Field
}

type MethodHolder struct{ N int }

func (m *MethodHolder) Get() *T { return nil }

func (m *MethodHolder) GetOK() *T { return &T{} }

func (m *MethodHolder) Value() int {
	return m.N // want `nil pointer dereference: m is nil`
}

func methodCallNil() {
	var h MethodHolder
	cfg := h.Get()
	_ = cfg.Field // want `nil pointer dereference: cfg is nil`
}

func methodCallNilPointer() {
	var h *MethodHolder
	cfg := h.Get()
	_ = cfg.Field // want `nil pointer dereference: cfg is nil`
}

func callMethodNonNil() {
	h := &MethodHolder{}
	cfg := h.GetOK()
	_ = cfg.Field
}

func callMethodNilReceiver() {
	var h *MethodHolder
	_ = h.Value()
}

func guardEarlyReturn(p *T) int {
	if p == nil {
		return 0
	}
	return p.Field // safe: p is non-nil past the guard
}
