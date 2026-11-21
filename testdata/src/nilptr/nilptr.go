package nilptr

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
	_ = p.Field
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
	for i := 0; i < 1; consume(*p) {
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
