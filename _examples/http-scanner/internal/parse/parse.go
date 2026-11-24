// Package parse contains small parsing helpers for HTTP responses.
package parse

// FirstByte returns the first byte of buf. It assumes buf is non-empty.
func FirstByte(buf []byte) byte {
	return buf[0]
}

// PeekEmpty demonstrates parsing an empty buffer.
func PeekEmpty() byte {
	var buf []byte
	return FirstByte(buf)
}
