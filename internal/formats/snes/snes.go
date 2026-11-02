// Package snes detects and strips the 512-byte SNES copier header.
package snes

// HeaderSize is the size of a SNES copier header.
const HeaderSize = 512

// HasHeader reports whether a file of the given size carries a copier
// header: its size is a multiple of 1024 bytes offset by 512.
func HasHeader(size int64) bool {
	return size%1024 == HeaderSize
}
