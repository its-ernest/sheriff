// Package parse contains parsing helpers for scan payloads.
package parse

import (
	"encoding/json"
	"fmt"
)

// Payload is a decoded scan payload.
type Payload struct {
	Name string
}

// Decode parses a payload. The marshal error is discarded, but the function
// returns an error as its last result, so Sheriff can fix it.
func Decode(data []byte) (*Payload, error) {
	encoded, _ := json.Marshal(Payload{}) // discarded error (fixable)
	_ = encoded
	var p Payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Log marshals a payload but discards the error. The function does not return
// an error, so Sheriff reports it for review.
func Log(p Payload) []byte {
	data, _ := json.Marshal(p) // discarded error; no error return -> review
	return data
}

// Describe formats a payload.
func Describe(p Payload) string {
	return fmt.Sprintf("payload %q", p.Name)
}
