package mediasample

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/processmetrics"
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

// TestRunSheetsRepairsSharedMatroskaKeyframesWithRealFFmpeg samples an
// H.264 Matroska file every 10 seconds whose keyframes lie 40 seconds apart,
// a shade per 10 seconds. The first sample's entry starts at the keyframe
// and the three after it, sharing that keyframe, at their sample times. All
// four decode the keyframe, so they must read as repeats and be repaired to
// their own shades.
func TestRunSheetsRepairsSharedMatroskaKeyframesWithRealFFmpeg(t *testing.T) {
	ffmpeg, caps := realFFmpeg(t)
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	grays := []int{20, 60, 100, 140, 180, 220}
	var args, inputs []string
	for i, gray := range grays {
		args = append(args, "-f", "lavfi", "-i", fmt.Sprintf("color=c=0x%02x%02x%02x:s=128x72:r=25:d=10", gray, gray, gray))
		inputs = append(inputs, fmt.Sprintf("[%d:v]", i))
	}
	clip := filepath.Join(t.TempDir(), "long-gop.mkv")
	args = append(append([]string{"-hide_banner", "-loglevel", "error"}, args...),
		"-filter_complex", strings.Join(inputs, "")+"concat=n=6:v=1:a=0,format=yuv420p[v]", "-map", "[v]",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "1000", "-keyint_min", "1000", "-sc_threshold", "0", "-bf", "2", clip)
	if output, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot encode the clip with libx264: %v: %s", err, output)
	}
	req := Request{
		Input:   clip,
		Samples: &Samples{Seconds: []float64{5, 15, 25, 35, 45, 55}},
		Sheets:  &SheetsOutput{TileWidth: 64, TileHeight: 36, Columns: 3, Rows: 2, Quality: 90},
		Threads: 1,
	}
	if err := caps.Require(req); err != nil {
		t.Skipf("ffmpeg cannot sample sheets: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	result, err := Runner{FFmpegPath: ffmpeg, FFprobePath: ffprobe, Workload: processmetrics.Trickplay}.Run(ctx, req)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(result.Sheets[0].JPEG))
	if err != nil {
		t.Fatal(err)
	}
	ycc := img.(*image.YCbCr)
	for cell, gray := range grays {
		x, y := (cell%3)*64+32, (cell/3)*36+18
		if got := int(ycc.Y[ycc.YOffset(x, y)]); got < gray-8 || got > gray+8 {
			t.Errorf("cell %d luma %d, want %d (frames %+v)", cell, got, gray, result.SheetFrames)
		}
	}
}
