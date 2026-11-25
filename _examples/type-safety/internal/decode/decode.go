// Package decode decodes JSON values and demonstrates type assertions.
package decode

import (
	"encoding/json"
	"fmt"
)

// Value holds decoded data.
type Value struct {
	Data any
}

// AsString asserts that v is a string.
func AsString(v any) string {
	return v.(string) // impossible if v is not a string
}

// Run demonstrates asserting a non-string value.
func Run() string {
	var v any = 42
	return AsString(v) // assert: v is int, not string
}

// DecodeJSON decodes JSON into a Value.
func DecodeJSON(data []byte) (*Value, error) {
	var v Value
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Describe formats a value.
func Describe(v Value) string {
	return fmt.Sprintf("%v", v.Data)
}
