package mediasample

import (
	"context"
	"errors"
	"fmt"
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

// TestRunSheetsStartsListEntriesAtKeyframes starts each Matroska list entry
// at the keyframe ffprobe found at or before its sample (an MP4 keeps sample
// times), keeps a sample without one,
// or sharing the previous sample's, at its own time, and falls back to sample
// times when the lookup fails.
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

// TestRunSheetsSkipsRefetchForMostlyEmptyList reads a list that left most of
// its samples empty as a window instead of fetching them one at a time.
func TestRunSheetsSkipsRefetchForMostlyEmptyList(t *testing.T) {
	logIllegal := func(log []string) []string {
		return append(log, "[h264 @ 0x55d0] illegal short term buffer state detected")
	}
	seconds := secondsFrom(5, 10, 20)
	var window []fakeSheetFrame
	for i := range 20 {
		window = append(window, timed(byte(10+i), fmt.Sprint(4+10*i)))
	}
	list := &fakeSheets{format: "matroska,webm", frames: []fakeSheetFrame{tagged(10, 5)}, editLog: logIllegal}
	win := &fakeSheets{format: "matroska,webm", frames: window, packetEnd: 200}
	var runs []string
	calls := 0
	runner := Runner{Exec: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		calls++
		switch {
		case calls == 1 || slices.Contains(args, "concat"):
			return list.exec(ctx, name, args, stdin, stdout, stderr)
		case slices.Contains(args, "-filter_complex"):
			runs = append(runs, "refetch")
		default:
			runs = append(runs, "window")
		}
		return win.exec(ctx, name, args, stdin, stdout, stderr)
	}}
	result, err := runner.Run(t.Context(), sheetsRequest(seconds, 5, 4))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(runs, []string{"window"}) || result.SheetFrames != (SheetFrames{Decoded: 20}) {
		t.Fatalf("runs %q, frames %+v; want one window and every sample", runs, result.SheetFrames)
	}
}

// TestRunSheetsLeavesSamplesPastAGapFilled does not fetch a sample whose
// keyframe lies far before it, a stretch with no pictures, so it stays
// filled and a damaged file still shows as one.
func TestRunSheetsLeavesSamplesPastAGapFilled(t *testing.T) {
	list := &fakeSheets{format: "matroska,webm", frames: []fakeSheetFrame{tagged(10, 5), tagged(20, 15), tagged(30, 25)}}
	refetched := false
	runner := Runner{FFprobePath: "ffprobe", Exec: func(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
		switch {
		case binary == "ffprobe":
			_, _ = io.WriteString(stdout, "3.500000,K__\n")
			return nil
		case slices.Contains(args, "-filter_complex"):
			refetched = true
		}
		return list.exec(ctx, binary, args, stdin, stdout, stderr)
	}}
	result, err := runner.Run(t.Context(), sheetsRequest(secondsFrom(5, 10, 4), 2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if refetched || result.SheetFrames != (SheetFrames{Decoded: 3, Filled: 1}) {
		t.Fatalf("refetched %t, frames %+v; want the sample 31.5s past its keyframe left filled", refetched, result.SheetFrames)
	}
}

func TestKeyframeSkipFollowsProbedCodec(t *testing.T) {
	for line, want := range map[string]string{
		"  Stream #0:0: Video: mpeg2video (Main), yuv420p(tv, top first), 720x576 [SAR 64:45 DAR 16:9], 25 fps": "nointra",
		"  Stream #0:0: Video: h264, yuv420p, 1920x1080 [SAR 1:1 DAR 16:9]":                                     "",
		"  Stream #0:0(eng): Video: hevc (Main 10), yuv420p10le(tv, bt2020nc/bt2020/smpte2084), 3840x2160":      "",
	} {
		header := &inputHeaderParser{}
		header.line("Input #0, matroska,webm, from '/media/a.mkv':")
		header.line(line)
		if got := keyframeSkipFor(header.info.VideoCodec); got != want {
			t.Errorf("%q: codec %q, skip %q, want %q", line, header.info.VideoCodec, got, want)
		}
	}
	req := sheetsRequest(secondsFrom(5, 10, 4), 2, 2)
	req.skipFrame = "nointra"
	list, _, err := buildSheetsArgs(req, Attempt{}, hardwareDecode{}, 0, "GRAPH", "")
	if err != nil {
		t.Fatal(err)
	}
	window := req
	window.Samples = nil
	window.Window = &Window{StartSeconds: 0, DurationSeconds: 40}
	windowArgs, _, err := buildSheetsArgs(window, Attempt{}, hardwareDecode{}, 0, "GRAPH", "packets.framecrc")
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{"list": list, "window": windowArgs} {
		if i := slices.Index(args, "-skip_frame:v"); i < 0 || args[i+1] != "nointra" {
			t.Errorf("%s args %q: want -skip_frame:v nointra", name, args)
		}
	}
	if !keyframeLowDelayFor("mpeg4") || !keyframeLowDelayFor("msmpeg4v3") || keyframeLowDelayFor("h264") || keyframeLowDelayFor("hevc") {
		t.Fatal("low delay should cover MPEG-4 Part 2 only")
	}
	req.lowDelay = true
	list, _, err = buildSheetsArgs(req, Attempt{}, hardwareDecode{}, 0, "GRAPH", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(list, " "), "-skip_frame:v nointra -flags:v +low_delay -protocol_whitelist") {
		t.Fatalf("list args %q: want low delay before the input", list)
	}
	if concatInputArgs[1] != "nokey" {
		t.Fatalf("concatInputArgs changed to %q", concatInputArgs)
	}
}
