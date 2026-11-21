package divzero

func consume(int) {}

func zeroLiteralVar() {
	d := 0
	_ = 10 / d // want `division by zero: d is zero`
	_ = 10 % d // want `modulo by zero: d is zero`
}

func zeroValueVar() {
	var d int
	_ = 10 / d // want `division by zero: d is zero`
}

func zeroAfterAssign() {
	d := 1
	d = 0
	_ = 10 / d // want `division by zero: d is zero`
}

func nonZero() {
	d := 2
	_ = 10 / d
}

func reassignedNonZero() {
	d := 0
	d = 2
	_ = 10 / d
}

func unknownParam(d int) {
	_ = 10 / d
}

func guardedNonZero(d int) {
	if d == 0 {
		return
	}
	_ = 10 / d
}

func guardBranches() {
	d := 0
	if d != 0 {
		_ = 10 / d
	} else {
		_ = 10 / d // want `division by zero: d is zero`
	}
}

func zeroBranch() {
	d := 0
	if d == 0 {
		_ = 10 / d // want `division by zero: d is zero`
	}
}

func mayAssign(cond bool) {
	d := 0
	if cond {
		d = 1
	}
	_ = 10 / d
}

func bothNonZero(cond bool) {
	d := 0
	if cond {
		d = 1
	} else {
		d = 2
	}
	_ = 10 / d
}

func bothZero(cond bool) {
	d := 1
	if cond {
		d = 0
	} else {
		d = 0
	}
	_ = 10 / d // want `division by zero: d is zero`
}

func namedResult() (d int) {
	_ = 10 / d // want `division by zero: d is zero`
	return
}

func floatDivision() {
	d := 0.0
	_ = 10.0 / d
}

func floatConversion() {
	d := 0
	_ = 10.0 / float64(d)
}

func compoundAssign() {
	d := 0
	d /= d // want `division by zero: d is zero`
}

func compoundModulo() {
	d := 0
	d %= d // want `modulo by zero: d is zero`
}

func incDec() {
	d := 0
	d++
	_ = 10 / d
}

func addressTaken() {
	d := 0
	p := &d
	*p = 1
	_ = 10 / d
}

func captured() {
	d := 0
	set := func() { d = 1 }
	set()
	_ = 10 / d
}

func loopGuard() {
	d := 0
	for d != 0 {
		_ = 10 / d
	}
}

func loopZero() {
	d := 0
	for i := 0; i < 1; i++ {
		_ = 10 / d // want `division by zero: d is zero`
	}
}

func loopAssignBeforeUse() {
	d := 0
	for i := 0; i < 1; i++ {
		d = 1
		_ = 10 / d
	}
}

func switchGuard() {
	d := 0
	switch {
	case d != 0:
		_ = 10 / d
	}
}

func switchZero() {
	d := 0
	switch {
	case d == 0:
		_ = 10 / d // want `division by zero: d is zero`
	}
}

func switchTagZero() {
	d := 0
	switch d {
	case 0:
		_ = 10 / d // want `division by zero: d is zero`
	case 1:
		_ = 10 / d
	}
}

func switchDefaultUnreachable() {
	d := 0
	switch d {
	case 0:
		d = 1
	default:
		_ = 10 / d
	}
}

func switchDefaultReachable() {
	d := 0
	switch d {
	case 1:
	default:
		_ = 10 / d // want `division by zero: d is zero`
	}
}

func constantExpression() {
	d := 1 - 1
	_ = 10 / d // want `division by zero: d is zero`
}

func nonZeroConstantExpression() {
	d := 2 - 1
	_ = 10 / d
}

func unknownCall() int { return 0 }

func callResult() {
	d := unknownCall()
	_ = 10 / d
}

var global int

func packageVar() {
	_ = 10 / global
}

func uintZero() {
	var d uint
	_ = 10 / d // want `division by zero: d is zero`
}

func namedIntZero() {
	type Count int
	var d Count
	_ = 10 / d // want `division by zero: d is zero`
}

func floatZero() {
	var d float64
	_ = 10.0 / d
}

func complexZero() {
	var d complex128
	_ = 10 / d
}

func divisionInCondition() {
	d := 0
	if 10/d == 1 { // want `division by zero: d is zero`
	}
}

func divisionInCall() {
	d := 0
	consume(10 / d) // want `division by zero: d is zero`
}

func divisionInReturn() int {
	d := 0
	return 10 / d // want `division by zero: d is zero`
}

func divisionInComposite() {
	d := 0
	_ = []int{10 / d} // want `division by zero: d is zero`
}
