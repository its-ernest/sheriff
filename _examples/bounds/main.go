// Command bounds demonstrates an out-of-range slice index that Sheriff
// reports.
//
// An empty slice is passed to first, which reads element zero. Running the
// program panics with an index out of range; Sheriff follows the empty slice
// into first and reports the index against its known length.
package main

import "fmt"

func first(items []string) string {
	return items[0]
}

func main() {
	var items []string
	fmt.Println(first(items))
}
