package assert

import (
	"bytes"
	"io"
	"strings"
)

type fooer interface{ Foo() }

type holder struct{ r io.Reader }

func pair() (io.Reader, int) { return bytes.NewBuffer(nil), 0 }

func concreteMismatch() {
	var r io.Reader = strings.NewReader("x")
	_ = r.(*bytes.Buffer) // want `impossible type assertion: r is \*strings\.Reader, not \*bytes\.Buffer`
}

func concreteMismatchAfterReassign() {
	var r io.Reader = strings.NewReader("x")
	r = bytes.NewBuffer(nil)
	_ = r.(*strings.Reader) // want `impossible type assertion: r is \*bytes\.Buffer, not \*strings\.Reader`
}

func interfaceNotImplemented() {
	var r io.Reader = strings.NewReader("x")
	_ = r.(fooer) // want `impossible type assertion: r is \*strings\.Reader, which does not implement fooer`
}

func declaredThenAssigned() {
	var r io.Reader
	r = strings.NewReader("x")
	_ = r.(fooer) // want `impossible type assertion: r is \*strings\.Reader, which does not implement fooer`
}

func bothBranchesSame(cond bool) {
	var r io.Reader = strings.NewReader("x")
	if cond {
		r = bytes.NewBuffer(nil)
	} else {
		r = bytes.NewBuffer(nil)
	}
	_ = r.(*strings.Reader) // want `impossible type assertion: r is \*bytes\.Buffer, not \*strings\.Reader`
}

func concreteSatisfied() {
	var r io.Reader = strings.NewReader("x")
	_ = r.(*strings.Reader)
}

func interfaceSatisfied() {
	var r io.Reader = bytes.NewBuffer(nil)
	_ = r.(io.Writer)
}

func commaOKShort() {
	var r io.Reader = strings.NewReader("x")
	_, ok := r.(*bytes.Buffer)
	_ = ok
}

func commaOKVar() {
	var r io.Reader = strings.NewReader("x")
	var b *bytes.Buffer
	var ok bool
	b, ok = r.(*bytes.Buffer)
	_, _ = b, ok
}

func unknownParameter(r io.Reader) {
	_ = r.(*bytes.Buffer)
}

func typeSwitchOperand(r io.Reader) {
	switch r.(type) {
	case *bytes.Buffer:
	}
}

func mergedBranches(cond bool) {
	var r io.Reader = strings.NewReader("x")
	if cond {
		r = bytes.NewBuffer(nil)
	}
	_ = r.(io.Writer)
}

func addressTaken() {
	var r io.Reader = strings.NewReader("x")
	p := &r
	*p = bytes.NewBuffer(nil)
	_ = r.(fooer)
}

func capturedByClosure() {
	var r io.Reader = strings.NewReader("x")
	reset := func() { r = bytes.NewBuffer(nil) }
	reset()
	_ = r.(fooer)
}

func assignedInLoop() {
	var r io.Reader = strings.NewReader("x")
	for i := 0; i < 1; i++ {
		r = bytes.NewBuffer(nil)
	}
	_ = r.(*strings.Reader)
}

func multipleAssignment() {
	var r io.Reader = strings.NewReader("x")
	r, _ = pair()
	_ = r.(fooer)
}

func fieldOfStruct(h holder) {
	_ = h.r.(fooer)
}

func nilInterface() {
	var r io.Reader
	_ = r.(io.Writer)
}

func concreteShadowing() {
	var r io.Reader = strings.NewReader("x")
	{
		r := "shadow"
		_ = r
	}
	_ = r.(fooer) // want `impossible type assertion: r is \*strings\.Reader, which does not implement fooer`
}

func assertParam(r io.Reader) {
	_ = r.(*bytes.Buffer) // want `impossible type assertion: r is \*strings\.Reader, not \*bytes\.Buffer`
}

func callAssertParamWithReader() {
	var r io.Reader = strings.NewReader("x")
	assertParam(r)
}

func callAssertParamWithBuffer() {
	var r io.Reader = bytes.NewBuffer(nil)
	assertParam(r)
}

func assertInterfaceParam(r io.Reader) {
	_ = r.(fooer) // want `impossible type assertion: r is \*strings\.Reader, which does not implement fooer`
}

func callAssertInterfaceParam() {
	var r io.Reader = strings.NewReader("x")
	assertInterfaceParam(r)
}

func assertParamOK(r io.Reader) {
	_ = r.(*strings.Reader)
}

func callAssertParamOK() {
	var r io.Reader = strings.NewReader("x")
	assertParamOK(r)
}

func assertUnknownCallee(r io.Reader) {
	_ = r.(*bytes.Buffer)
}

func passUnknownAssert(r io.Reader) {
	assertUnknownCallee(r)
}
