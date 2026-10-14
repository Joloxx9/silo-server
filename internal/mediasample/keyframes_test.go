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

// listInpoints returns the inpoints of the first concat list a run read.
func listInpoints(stdin io.Reader) []string {
	raw, _ := io.ReadAll(stdin)
	var inpoints []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if at, ok := strings.CutPrefix(line, "inpoint "); ok {
			inpoints = append(inpoints, at)
		}
	}
	return inpoints
}

// TestRunSheetsStartsListEntriesAtKeyframes starts each Matroska list entry
// at the keyframe ffprobe found at or before its sample (an MP4 keeps sample
// times and, with no cell missing, looks nothing up), keeps a sample without one, or sharing the previous sample's, at
// its own time, and falls back to sample times when the lookup fails.
func TestRunSheetsStartsListEntriesAtKeyframes(t *testing.T) {
	for name, tt := range map[string]struct {
		format      string
		probe       string
		probeErr    error
		wantInpoint []string
	}{
		"keyframes":     {probe: "3.500000,K__\n13.200000,K__\n24.000000,K__\n", wantInpoint: []string{"3.5", "13.2", "24", "35"}},
		"none before":   {probe: "13.200000,K__\n", wantInpoint: []string{"5", "13.2", "25", "35"}},
		"lookup failed": {probeErr: errors.New("ffprobe exited"), wantInpoint: []string{"5", "15", "25", "35"}},
		"mp4":           {format: "mov,mp4,m4a,3gp,3g2,mj2", probe: "3.500000,K__\n13.200000,K__\n24.000000,K__\n", wantInpoint: []string{"5", "15", "25", "35"}},
	} {
		format := tt.format
		if format == "" {
			format = "matroska,webm"
		}
		list := &fakeSheets{format: format, frames: []fakeSheetFrame{tagged(10, 5), tagged(20, 15), tagged(30, 25), tagged(40, 35)}}
		var probed, inpoints []string
		runner := Runner{FFprobePath: "ffprobe", Exec: func(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if binary == "ffprobe" {
				probed = args
				if tt.probeErr != nil {
					return tt.probeErr
				}
				_, _ = io.WriteString(stdout, tt.probe)
				return nil
			}
			if slices.Contains(args, "concat") && inpoints == nil {
				inpoints = listInpoints(stdin)
			}
			return list.exec(ctx, binary, args, stdin, stdout, stderr)
		}}
		if _, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		wantProbe := format == "matroska,webm"
		if i := slices.Index(probed, "-read_intervals"); (i >= 0) != wantProbe || (wantProbe && probed[i+1] != "5%+#1,15%+#1,25%+#1,35%+#1") {
			t.Fatalf("%s: ffprobe args %q, want a lookup %t", name, probed, wantProbe)
		}
		if !slices.Equal(inpoints, tt.wantInpoint) {
			t.Fatalf("%s: inpoints %v, want %v", name, inpoints, tt.wantInpoint)
		}
	}
}

// TestRunSheetsLeavesSamplesPastAGapFilled does not repair a sample whose
// keyframe lies far before it, a stretch with no pictures, so it stays
// filled and a damaged file still shows as one.
func TestRunSheetsLeavesSamplesPastAGapFilled(t *testing.T) {
	list := &fakeSheets{format: "matroska,webm", frames: []fakeSheetFrame{tagged(10, 5), tagged(20, 15), tagged(30, 25)}}
	var repaired [][]string
	lists := 0
	runner := Runner{FFprobePath: "ffprobe", Exec: func(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		if binary == "ffprobe" {
			_, _ = io.WriteString(stdout, "3.500000,K__\n")
			return nil
		}
		if slices.Contains(args, "concat") {
			if lists++; lists > 1 {
				repaired = append(repaired, listInpoints(stdin))
			}
		}
		return list.exec(ctx, binary, args, stdin, stdout, stderr)
	}}
	result, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2))
	if err != nil {
		t.Fatal(err)
	}
	for _, inpoints := range repaired {
		if slices.Contains(inpoints, "35") {
			t.Fatalf("repaired the sample 31.5s past its keyframe: %v", repaired)
		}
	}
	if result.SheetFrames.Filled != 1 {
		t.Fatalf("frames %+v, want the sample past the gap filled", result.SheetFrames)
	}
}

// TestRunSheetsRepairsSamplesTheLookupFoundNoKeyframeFor repairs a missing
// sample the lookup found no keyframe for, as before a container's first
// keyframe: a missed lookup is not a stretch without pictures.
func TestRunSheetsRepairsSamplesTheLookupFoundNoKeyframeFor(t *testing.T) {
	list := &fakeSheets{format: "matroska,webm", frames: []fakeSheetFrame{tagged(20, 15), tagged(30, 25), tagged(40, 35)}}
	var repaired [][]string
	lists := 0
	runner := Runner{FFprobePath: "ffprobe", Exec: func(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		if binary == "ffprobe" {
			_, _ = io.WriteString(stdout, "13.200000,K__\n24.000000,K__\n34.000000,K__\n")
			return nil
		}
		if slices.Contains(args, "concat") {
			if lists++; lists > 1 {
				repaired = append(repaired, listInpoints(stdin))
			}
		}
		return list.exec(ctx, binary, args, stdin, stdout, stderr)
	}}
	if _, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2)); err != nil {
		t.Fatal(err)
	}
	if len(repaired) == 0 || !slices.Contains(repaired[0], "5") {
		t.Fatalf("repair lists %v, want the sample at 5 repaired", repaired)
	}
}

// TestRunSheetsLooksUpMP4KeyframesOnlyForRepairs looks keyframes up for an
// MP4 list only once a cell is missing, for the gap check.
func TestRunSheetsLooksUpMP4KeyframesOnlyForRepairs(t *testing.T) {
	list := &fakeSheets{format: "mov,mp4,m4a,3gp,3g2,mj2", frames: []fakeSheetFrame{tagged(10, 5), tagged(20, 15), tagged(30, 25)}}
	var repaired [][]string
	probes, lists := 0, 0
	runner := Runner{FFprobePath: "ffprobe", Exec: func(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		if binary == "ffprobe" {
			probes++
			_, _ = io.WriteString(stdout, "3.500000,K__\n")
			return nil
		}
		if slices.Contains(args, "concat") {
			if lists++; lists > 1 {
				repaired = append(repaired, listInpoints(stdin))
			}
		}
		return list.exec(ctx, binary, args, stdin, stdout, stderr)
	}}
	result, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if probes != 1 {
		t.Fatalf("ffprobe ran %d times, want once, for the missing cell", probes)
	}
	for _, inpoints := range repaired {
		if slices.Contains(inpoints, "35") {
			t.Fatalf("repaired the sample 31.5s past its keyframe: %v", repaired)
		}
	}
	if result.SheetFrames.Filled != 1 {
		t.Fatalf("frames %+v, want the sample past the gap filled", result.SheetFrames)
	}
}

// TestRunSheetsLooksUpMatroskaKeyframesOnce does not look keyframes up again
// for a repair when the list's own lookup found none.
func TestRunSheetsLooksUpMatroskaKeyframesOnce(t *testing.T) {
	list := &fakeSheets{format: "matroska,webm", frames: []fakeSheetFrame{tagged(10, 5), tagged(20, 15), tagged(30, 25)}}
	probes := 0
	runner := Runner{FFprobePath: "ffprobe", Exec: func(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		if binary == "ffprobe" {
			probes++
			return nil
		}
		return list.exec(ctx, binary, args, stdin, stdout, stderr)
	}}
	if _, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2)); err != nil {
		t.Fatal(err)
	}
	if probes != 1 {
		t.Fatalf("ffprobe ran %d times, want once", probes)
	}
}
