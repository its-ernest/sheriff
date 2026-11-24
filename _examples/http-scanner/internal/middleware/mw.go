// Package middleware contains request-scoped helpers.
package middleware

import "log"

// Logger is a minimal request logger.
type Logger struct{}

// Log writes a message.
func (l *Logger) Log(msg string) { log.Println(msg) }

// FromContext extracts a *Logger from a context value.
func FromContext(v any) *Logger {
	return v.(*Logger)
}

// Run demonstrates extracting a logger from a non-logger context value.
func Run() {
	var v any = "request-id"
	logger := FromContext(v)
	logger.Log("handled")
}
