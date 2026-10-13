package mediasample

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestParseKeyframeTimes(t *testing.T) {
	out := "24.000000,K__\n3.500000,K_D\n8.1,__\nN/A,K__\n13.2,K__\n13.2,K__\n\n"
	if got, want := parseKeyframeTimes([]byte(out)), []float64{3.5, 13.2, 24}; !slices.Equal(got, want) {
		t.Fatalf("keyframes %v, want %v", got, want)
	}
}

// TestRunSheetsStartsListEntriesAtKeyframes starts each list entry at the
// keyframe ffprobe found at or before its sample, keeps a sample without one,
// or sharing the previous sample's, at its own time, and falls back to sample
// times when the lookup fails.
func TestRunSheetsStartsListEntriesAtKeyframes(t *testing.T) {
	for name, tt := range map[string]struct {
		probe       string
		probeErr    error
		wantInpoint []string
	}{
		"keyframes":     {probe: "3.500000,K__\n13.200000,K__\n24.000000,K__\n", wantInpoint: []string{"3.5", "13.2", "24", "35"}},
		"none before":   {probe: "13.200000,K__\n", wantInpoint: []string{"5", "13.2", "25", "35"}},
		"lookup failed": {probeErr: errors.New("ffprobe exited"), wantInpoint: []string{"5", "15", "25", "35"}},
	} {
		list := &fakeSheets{format: "matroska,webm", frames: []fakeSheetFrame{tagged(10, 5), tagged(20, 15), tagged(30, 25), tagged(40, 35)}}
		var probed []string
		var inpoints []string
		runner := Runner{FFprobePath: "ffprobe", Exec: func(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if binary == "ffprobe" {
				probed = args
				if tt.probeErr != nil {
					return tt.probeErr
				}
				_, _ = io.WriteString(stdout, tt.probe)
				return nil
			}
			if slices.Contains(args, "concat") {
				raw, _ := io.ReadAll(stdin)
				for _, line := range strings.Split(string(raw), "\n") {
					if at, ok := strings.CutPrefix(line, "inpoint "); ok {
						inpoints = append(inpoints, at)
					}
				}
			}
			return list.exec(ctx, binary, args, stdin, stdout, stderr)
		}}
		result, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if i := slices.Index(probed, "-read_intervals"); i < 0 || probed[i+1] != "5%+#1,15%+#1,25%+#1,35%+#1" {
			t.Fatalf("%s: ffprobe args %q", name, probed)
		}
		if !slices.Equal(inpoints, tt.wantInpoint) {
			t.Fatalf("%s: inpoints %v, want %v", name, inpoints, tt.wantInpoint)
		}
		if result.SheetFrames != (SheetFrames{Decoded: 4}) {
			t.Fatalf("%s: frames %+v", name, result.SheetFrames)
		}
	}
}

// TestRunSheetsRetriesFailedRefetch tries a refetch run that fails once more
// for the samples it left empty, keeping the frame it did place.
func TestRunSheetsRetriesFailedRefetch(t *testing.T) {
	logDuplicates := func(log []string) []string {
		return append(log, "[hevc @ 0x55d0] Duplicate POC in a sequence: 48.")
	}
	list := &fakeSheets{format: "matroska,webm", frames: []fakeSheetFrame{tagged(10, 5)}, editLog: logDuplicates}
	partial := &fakeSheets{frames: []fakeSheetFrame{tagged(20, 15)}, extraStdout: []byte{1, 2, 3}}
	rest := &fakeSheets{frames: []fakeSheetFrame{tagged(30, 25), tagged(40, 35)}}
	var refetches [][]string
	calls := 0
	runner := Runner{Exec: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		calls++
		if !slices.Contains(args, "-filter_complex") {
			return list.exec(ctx, name, args, stdin, stdout, stderr)
		}
		refetches = append(refetches, args)
		if len(refetches) == 1 {
			_ = partial.exec(ctx, name, args, stdin, stdout, stderr)
			return errors.New("ffmpeg killed")
		}
		return rest.exec(ctx, name, args, stdin, stdout, stderr)
	}}
	result, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(refetches) != 2 || strings.Count(strings.Join(refetches[0], " "), "-i /media/a.mkv") != 3 || strings.Count(strings.Join(refetches[1], " "), "-i /media/a.mkv") != 2 {
		t.Fatalf("refetch runs %d, want three samples and then the two left", len(refetches))
	}
	if got := sheetLumas(t, result, 2, 2); !near(got[0], []int{10, 20, 30, 40}) || result.SheetFrames != (SheetFrames{Decoded: 4}) {
		t.Fatalf("cells %v, frames %+v", got, result.SheetFrames)
	}
}
