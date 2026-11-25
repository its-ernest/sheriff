// Package measure computes simple measurements.
package measure

// First returns the first item.
func First(items []string) string {
	return items[0] // index out of range if empty
}

// FirstEmpty demonstrates indexing an empty slice.
func FirstEmpty() string {
	var items []string
	return First(items) // bounds
}

// Average divides total by count.
func Average(total, count int) int {
	return total / count // division by zero if count is 0
}

// Zero demonstrates division by zero.
func Zero() int {
	var count int
	return Average(100, count) // divzero
}
