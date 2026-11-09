// Package cuesheet parses the one specific shape of CUE sheet chdman's
// own `extractcd` generates - a single FILE line, one combined raw track
// blob, TRACK/INDEX entries in MM:SS:FF - so a caller can hash each
// track's own byte range directly out of that one file without
// splitting it. This is not a general-purpose CUE parser: a hand-written
// or multi-FILE cue sheet (chdman never generates one) is rejected
// rather than guessed at.
package cuesheet

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

// BytesPerFrame is the raw CD sector size chdman's extracted CD data
// always uses (2352 bytes/sector, the Redbook "frame"), regardless of
// the original track's declared mode (MODE1/2048, MODE1/2352, AUDIO).
const BytesPerFrame = 2352

// FramesPerSecond is the Redbook CD frame rate (75 frames/sec), used to
// convert a cue sheet's MM:SS:FF timecodes to a frame count.
const FramesPerSecond = 75

// Track is one track's type and byte range within the cue's one FILE.
type Track struct {
	Number     int
	Type       string // e.g. "MODE1/2352", "AUDIO"
	Start, End int64  // [Start, End) byte offsets into File
}

var (
	fileRE  = regexp.MustCompile(`^FILE\s+"([^"]*)"`)
	trackRE = regexp.MustCompile(`^\s*TRACK\s+(\d+)\s+(\S+)`)
	indexRE = regexp.MustCompile(`^\s*INDEX\s+(\d+)\s+(\d+):(\d+):(\d+)`)
)

// Parse reads a cue sheet's track layout. fileSize is the referenced
// FILE's actual byte size (the caller has necessarily already read or
// stat'd it), used to resolve the last track's End - a cue sheet has no
// explicit end marker for the final track.
//
// Each track's End is the next track's own INDEX 01 (not INDEX 00): a
// pregap between two tracks is attributed to the end of the preceding
// track's byte range, not the start of the following one - the same
// convention Redump's own per-track dump files use, which is the whole
// point of getting this boundary right (it's what a caller is hashing
// against).
func Parse(data []byte, fileSize int64) (file string, tracks []Track, err error) {
	var starts []int64
	var numbers []int
	var types []string

	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if m := fileRE.FindStringSubmatch(line); m != nil {
			if file != "" {
				return "", nil, fmt.Errorf("cuesheet: more than one FILE line, unsupported")
			}
			file = m[1]
			continue
		}
		if m := trackRE.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			numbers = append(numbers, n)
			types = append(types, m[2])
			starts = append(starts, -1)
			continue
		}
		if m := indexRE.FindStringSubmatch(line); m != nil {
			if len(numbers) == 0 {
				return "", nil, fmt.Errorf("cuesheet: INDEX line before any TRACK")
			}
			idx, _ := strconv.Atoi(m[1])
			if idx != 1 {
				continue // INDEX 00 (pregap start) folds into the previous track's range
			}
			mm, _ := strconv.Atoi(m[2])
			ss, _ := strconv.Atoi(m[3])
			ff, _ := strconv.Atoi(m[4])
			frames := int64(mm)*60*FramesPerSecond + int64(ss)*FramesPerSecond + int64(ff)
			starts[len(starts)-1] = frames * BytesPerFrame
		}
	}
	if err := sc.Err(); err != nil {
		return "", nil, err
	}
	if file == "" {
		return "", nil, fmt.Errorf("cuesheet: no FILE line found")
	}
	if len(numbers) == 0 {
		return "", nil, fmt.Errorf("cuesheet: no TRACK lines found")
	}
	for i, start := range starts {
		if start < 0 {
			return "", nil, fmt.Errorf("cuesheet: TRACK %d has no INDEX 01", numbers[i])
		}
	}

	tracks = make([]Track, len(numbers))
	for i := range numbers {
		end := fileSize
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		tracks[i] = Track{Number: numbers[i], Type: types[i], Start: starts[i], End: end}
	}
	return file, tracks, nil
}
