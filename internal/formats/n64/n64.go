// Package n64 detects and normalizes N64 ROM byte order to big-endian
// (.z64), the format MAME/RetroArch cores expect.
package n64

import (
	"bytes"
	"fmt"
	"os"
)

// Order identifies a ROM's on-disk byte order by its first 4 header bytes.
type Order int

const (
	OrderUnknown Order = iota
	OrderZ64           // already big-endian (.z64 native order)
	OrderV64           // byte-swapped pairs (.v64)
	OrderN64           // word-swapped (.n64)
)

var headers = map[Order][]byte{
	OrderZ64: {0x80, 0x37, 0x12, 0x40},
	OrderV64: {0x37, 0x80, 0x40, 0x12},
	OrderN64: {0x40, 0x12, 0x37, 0x80},
}

// Detect reads path's header and returns its byte Order.
func Detect(path string) (Order, error) {
	f, err := os.Open(path)
	if err != nil {
		return OrderUnknown, err
	}
	defer f.Close()
	buf := make([]byte, 4)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return OrderUnknown, err
	}
	for order, header := range headers {
		if bytes.Equal(buf, header) {
			return order, nil
		}
	}
	return OrderUnknown, nil
}

// ToBigEndian returns data converted from order to big-endian (.z64); it
// errors for OrderUnknown.
func ToBigEndian(data []byte, order Order) ([]byte, error) {
	out := make([]byte, len(data))
	switch order {
	case OrderZ64:
		copy(out, data)
	case OrderV64:
		swapPairs(data, out)
	case OrderN64:
		swapWords(data, out)
	default:
		return nil, fmt.Errorf("unknown N64 byte order")
	}
	return out, nil
}

// swapPairs swaps each adjacent pair of bytes: ABCD -> BADC.
func swapPairs(src, dst []byte) {
	n := len(src) - len(src)%2
	for i := 0; i < n; i += 2 {
		dst[i], dst[i+1] = src[i+1], src[i]
	}
	copy(dst[n:], src[n:])
}

// swapWords reverses each 4-byte word: ABCD -> DCBA.
func swapWords(src, dst []byte) {
	n := len(src) - len(src)%4
	for i := 0; i < n; i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+3], src[i+2], src[i+1], src[i]
	}
	copy(dst[n:], src[n:])
}
