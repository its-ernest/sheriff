package bounds

func consume(int) {}

func indexArray() {
	var a [3]int
	i := 3
	_ = a[i] // want `index out of range: index 3 for length 3`
}

func indexArraySafe() {
	var a [3]int
	i := 2
	_ = a[i]
}

func indexArrayNegative() {
	var a [3]int
	i := -1
	_ = a[i] // want `index out of range: index -1 for length 3`
}

func indexArrayReassigned() {
	var a [3]int
	i := 3
	i = 2
	_ = a[i]
}

func indexArrayLen() {
	var a [3]int
	i := len(a)
	_ = a[i] // want `index out of range: index 3 for length 3`
}

func indexPointerArray() {
	p := new([3]int)
	i := 3
	_ = p[i] // want `index out of range: index 3 for length 3`
}

func indexSliceLiteral() {
	s := []int{1, 2, 3}
	i := 3
	_ = s[i] // want `index out of range: index 3 for length 3`
}

func indexKeyedLiteral() {
	s := []int{2: 0}
	i := 3
	_ = s[i] // want `index out of range: index 3 for length 3`
}

func indexMake() {
	s := make([]int, 2)
	i := 2
	_ = s[i] // want `index out of range: index 2 for length 2`
}

func indexMakeTrackedLength() {
	n := 2
	s := make([]int, n)
	i := 2
	_ = s[i] // want `index out of range: index 2 for length 2`
}

func indexMakeUnknownLength(n int) {
	s := make([]int, n)
	_ = s[0]
}

func indexAfterReslice() {
	s := make([]int, 5)
	s = s[:2]
	i := 2
	_ = s[i] // want `index out of range: index 2 for length 2`
}

func indexAfterAppend() {
	s := make([]int, 2)
	s = append(s, 3)
	i := 2
	_ = s[i]
}

func indexNilSlice() {
	var s []int
	_ = s[0] // want `index out of range: index 0 for length 0`
}

func indexStringLiteral() {
	s := "abc"
	i := 3
	_ = s[i] // want `index out of range: index 3 for length 3`
}

func indexEmptyString() {
	var s string
	_ = s[0] // want `index out of range: index 0 for length 0`
}

func indexUnknown(a []int, i int) {
	_ = a[i]
}

func indexMap(m map[int]int, i int) {
	_ = m[i]
}

func indexNamedSliceResult() (s []int) {
	_ = s[0] // want `index out of range: index 0 for length 0`
	return
}

func indexNamedIntResult() (i int) {
	var a [3]int
	_ = a[i]
	return
}

func indexBothBranches(cond bool) {
	var a [3]int
	i := 0
	if cond {
		i = 3
	} else {
		i = 3
	}
	_ = a[i] // want `index out of range: index 3 for length 3`
}

func indexMayAssign(cond bool) {
	var a [3]int
	i := 0
	if cond {
		i = 3
	}
	_ = a[i]
}

func indexAddressTaken() {
	var a [3]int
	i := 3
	p := &i
	*p = 0
	_ = a[i]
}

func indexCaptured() {
	var a [3]int
	i := 3
	reset := func() { i = 0 }
	reset()
	_ = a[i]
}

func indexIncDec() {
	var a [3]int
	i := 2
	i++
	_ = a[i]
}

func indexLoop() {
	var a [3]int
	i := 3
	for j := 0; j < 1; j++ {
		_ = a[i] // want `index out of range: index 3 for length 3`
	}
}

func indexLoopAssign() {
	var a [3]int
	i := 3
	for j := 0; j < 1; j++ {
		i = 0
		_ = a[i]
	}
}

func indexInCondition() {
	var a [3]int
	i := 3
	if a[i] == 0 { // want `index out of range: index 3 for length 3`
	}
}

func indexInCall() {
	var a [3]int
	i := 3
	consume(a[i]) // want `index out of range: index 3 for length 3`
}

func sliceArrayLowNegative() {
	var a [3]int
	low := -1
	_ = a[low:] // want `slice bounds out of range: low index -1 is negative`
}

func sliceArrayHighNegative() {
	var a [3]int
	high := -1
	_ = a[:high] // want `slice bounds out of range: high index -1 is negative`
}

func sliceArrayLowHigh() {
	var a [3]int
	low := 2
	high := 1
	_ = a[low:high] // want `slice bounds out of range: low index 2 is greater than high index 1`
}

func sliceArrayHigh() {
	var a [3]int
	high := 4
	_ = a[:high] // want `slice bounds out of range: high index 4 is greater than length 3`
}

func sliceArraySafe() {
	var a [3]int
	low := 1
	high := 3
	_ = a[low:high]
}

func sliceStringHigh() {
	s := "abc"
	high := 4
	_ = s[:high] // want `slice bounds out of range: high index 4 is greater than length 3`
}

func sliceWithinCapacity() {
	s := make([]int, 2, 10)
	high := 5
	_ = s[:high]
}

func sliceLowBeyondLength() {
	s := make([]int, 2, 10)
	low := 3
	_ = s[low:] // want `slice bounds out of range: low index 3 is greater than high index 2`
}

func threeIndexArrayMax() {
	var a [3]int
	max := 4
	_ = a[:2:max] // want `slice bounds out of range: max index 4 is greater than length 3`
}

func threeIndexMaxBelowHigh() {
	var a [5]int
	high := 4
	max := 3
	_ = a[:high:max] // want `slice bounds out of range: max index 3 is less than high index 4`
}

func threeIndexSliceWithinCapacity() {
	s := make([]int, 2, 10)
	max := 5
	_ = s[:2:max]
}

func sliceUnknown(s []int, low int) {
	_ = s[low:]
}
