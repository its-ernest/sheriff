// Command assert demonstrates a type assertion that can never succeed, which
// Sheriff reports.
//
// A *strings.Reader is passed as an io.Reader to consume, which asserts it to
// *bytes.Buffer. The dynamic type is fixed, so the assertion always fails.
// Running the program panics with an interface conversion error; Sheriff
// follows the argument into consume and reports the impossible assertion there.
package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

func consume(r io.Reader) {
	buf := r.(*bytes.Buffer)
	fmt.Println(buf.Len())
}

func main() {
	var r io.Reader = strings.NewReader("data")
	consume(r)
}
