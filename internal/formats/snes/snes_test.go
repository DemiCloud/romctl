package snes

import "testing"

func TestHasHeader(t *testing.T) {
	const kib = 1024
	cases := []struct {
		size int64
		want bool
	}{
		{512 * kib, false},            // headerless 512 KiB dump
		{512*kib + HeaderSize, true},  // same dump with a 512-byte copier header
		{1024 * kib, false},           // headerless 1 MiB dump
		{1024*kib + HeaderSize, true}, // same, with header
		{0, false},
		{HeaderSize, true}, // degenerate: a bare header with no ROM data
	}
	for _, tc := range cases {
		if got := HasHeader(tc.size); got != tc.want {
			t.Errorf("HasHeader(%d) = %v, want %v", tc.size, got, tc.want)
		}
	}
}
