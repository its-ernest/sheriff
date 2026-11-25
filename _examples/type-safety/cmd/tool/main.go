// Command tool is a small program that demonstrates type-safety defects.
package main

import (
	"fmt"

	"example/type-safety/internal/decode"
	"example/type-safety/internal/measure"
)

func main() {
	fmt.Println(decode.Run())
	fmt.Println(measure.FirstEmpty())
	fmt.Println(measure.Zero())
}
