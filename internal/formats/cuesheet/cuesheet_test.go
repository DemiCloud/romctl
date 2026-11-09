package cuesheet

import "testing"

// Real output from chdman 0.289's extractcd on a single MODE1/2048 track,
// confirmed against an actual chdman binary.
const realSingleTrackCue = `FILE "out.bin" BINARY
  TRACK 01 MODE1/2048
    INDEX 01 00:00:00
`

func TestParseSingleTrack(t *testing.T) {
	file, tracks, err := Parse([]byte(realSingleTrackCue), 8192)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if file != "out.bin" {
		t.Errorf("file = %q", file)
	}
	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1", len(tracks))
	}
	tr := tracks[0]
	if tr.Number != 1 || tr.Type != "MODE1/2048" || tr.Start != 0 || tr.End != 8192 {
		t.Errorf("tracks[0] = %+v", tr)
	}
}

// Synthetic multi-track cue (data track + audio track, no pregap) in the
// same shape chdman's extractcd produces for a multi-track disc.
const syntheticMultiTrackCue = `FILE "out.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    INDEX 01 00:00:04
`

func TestParseMultiTrack(t *testing.T) {
	// Track 1: 4 frames of data = 4*2352 bytes. Track 2 starts right
	// after (no pregap in this fixture) and runs to EOF.
	fileSize := int64(4*BytesPerFrame + 3*BytesPerFrame)
	file, tracks, err := Parse([]byte(syntheticMultiTrackCue), fileSize)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if file != "out.bin" {
		t.Errorf("file = %q", file)
	}
	if len(tracks) != 2 {
		t.Fatalf("len(tracks) = %d, want 2", len(tracks))
	}
	if tracks[0].Start != 0 || tracks[0].End != 4*BytesPerFrame {
		t.Errorf("tracks[0] = %+v, want Start=0 End=%d", tracks[0], 4*BytesPerFrame)
	}
	if tracks[1].Start != 4*BytesPerFrame || tracks[1].End != fileSize {
		t.Errorf("tracks[1] = %+v, want Start=%d End=%d", tracks[1], 4*BytesPerFrame, fileSize)
	}
}

// A pregap (INDEX 00) between tracks must fold into the END of the
// preceding track's range, not the start of the following one - the
// convention Redump's own split-track dumps use.
const pregapCue = `FILE "out.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
  TRACK 02 AUDIO
    INDEX 00 00:00:04
    INDEX 01 00:00:06
`

func TestParsePregapFoldsIntoPrecedingTrack(t *testing.T) {
	fileSize := int64(10 * BytesPerFrame)
	_, tracks, err := Parse([]byte(pregapCue), fileSize)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Track 1's End must be track 2's INDEX 01 (frame 6), not its
	// INDEX 00 (frame 4) - the 2-frame pregap belongs to track 1's range.
	if tracks[0].End != 6*BytesPerFrame {
		t.Errorf("tracks[0].End = %d, want %d (track 2's INDEX 01, pregap included)", tracks[0].End, 6*BytesPerFrame)
	}
	if tracks[1].Start != 6*BytesPerFrame {
		t.Errorf("tracks[1].Start = %d, want %d", tracks[1].Start, 6*BytesPerFrame)
	}
}

func TestParseRejectsMultipleFileLines(t *testing.T) {
	cue := `FILE "a.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 01 00:00:00
FILE "b.bin" BINARY
  TRACK 02 AUDIO
    INDEX 01 00:00:04
`
	if _, _, err := Parse([]byte(cue), 1000); err == nil {
		t.Fatal("expected an error for more than one FILE line")
	}
}

func TestParseRejectsMissingIndex01(t *testing.T) {
	cue := `FILE "out.bin" BINARY
  TRACK 01 MODE1/2352
    INDEX 00 00:00:00
`
	if _, _, err := Parse([]byte(cue), 1000); err == nil {
		t.Fatal("expected an error for a track with no INDEX 01")
	}
}

func TestParseRejectsNoFile(t *testing.T) {
	cue := "  TRACK 01 MODE1/2352\n    INDEX 01 00:00:00\n"
	if _, _, err := Parse([]byte(cue), 1000); err == nil {
		t.Fatal("expected an error for a cue sheet with no FILE line")
	}
}
