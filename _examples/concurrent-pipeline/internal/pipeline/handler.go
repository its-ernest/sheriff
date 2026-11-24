package pipeline

// Handle asserts that v is a string.
func Handle(v any) string {
	msg := v.(string)
	return msg
}

// RunHandler demonstrates handling a non-string message.
func RunHandler() string {
	return Handle(42)
}
