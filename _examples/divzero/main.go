// Command divzero demonstrates an integer division by zero that Sheriff
// reports.
//
// The count is declared as the zero value and passed to average, which divides
// by it. Running the program panics with an integer divide by zero; Sheriff
// follows the zero argument into average and reports the division there.
package main

import "fmt"

func average(total, count int) int {
	return total / count
}

func main() {
	var count int
	fmt.Println(average(100, count))
}
