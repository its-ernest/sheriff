// Package store loads and saves users as JSON.
package store

import (
	"encoding/json"
	"fmt"
)

type User struct {
	Name string
}

// Load parses a user from JSON. The marshal error is discarded, but the function
// returns an error as its last result, so Sheriff can fix it.
func Load(data []byte) (*User, error) {
	var u User
	encoded, _ := json.Marshal(u) // discarded error (fixable: error is last)
	_ = encoded
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Save marshals a user but discards the error. The function does not return an
// error, so Sheriff reports it for review.
func Save(u User) []byte {
	data, _ := json.Marshal(u) // discarded error; no error return -> review
	return data
}

// LoadFirst parses a user and returns (error, int). The error is not the last
// result, so Sheriff reports it for review.
func LoadFirst(data []byte) (error, int) {
	var u User
	_ = json.Unmarshal(data, &u) // discarded error; error not last -> review
	return nil, len(u.Name)
}

// Describe formats a user. It handles its error, so there is nothing to report.
func Describe(u User) string {
	return fmt.Sprintf("user %q", u.Name)
}
