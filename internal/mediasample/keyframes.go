package mediasample

import (
	"bufio"
	"bytes"
	"math"
	"slices"
	"strconv"
	"strings"
)

// A list entry starting at its sample time makes the concat demuxer seek to
// the keyframe before it and read every packet, of every stream, from there
// to the sample. With keyframes seconds apart that is a large share of the
// file: a 4K remux read 3.8 GB for 80 samples. Starting each entry at the
// keyframe itself reads only the keyframe and the entry's span. The keyframe
// times come from one ffprobe pass whose read intervals each seek to a
// sample and read a single packet, the keyframe the seek lands on.
//
// Only Matroska entries start at their keyframes. An MP4 entry whose
// inpoint is its keyframe's time often seeks past that keyframe (45 of 80
// samples of one H.264 episode decoded nothing), so other containers keep
// sample-time inpoints and use the keyframes only to spot gaps.

// keyframeInpoints returns each sample's list inpoint and keyframe, both
// as container times, or nils when the runner has no ffprobe or the lookup
// fails, so the list falls back to sample times. A sample with no keyframe
// found at or before it keeps its own time as inpoint and NaN as keyframe.
func (a attemptRun) keyframeInpoints(req Request, inputStart float64) (inpoints, keyframes []float64) {
	if a.runner.FFprobePath == "" {
		return nil, nil
	}
	seconds := req.Samples.Seconds
	targets := make([]float64, len(seconds))
	var intervals strings.Builder
	for i, at := range seconds {
		targets[i] = math.Max(0, at+inputStart)
		if i > 0 {
			intervals.WriteByte(',')
		}
		intervals.WriteString(formatSampleSeconds(targets[i]) + "%+#1")
	}
	args := []string{
		"-v", errorLogLevel, "-select_streams", "V:0",
		"-read_intervals", intervals.String(),
		"-show_entries", "packet=pts_time,flags", "-of", "csv=p=0",
		req.Input,
	}
	var out bytes.Buffer
	if failure := a.execBinary(req, a.runner.FFprobePath, args, nil, &out); failure != nil {
		return nil, nil
	}
	keys := parseKeyframeTimes(out.Bytes())
	if len(keys) == 0 {
		return nil, nil
	}
	// A sample sharing its keyframe with the sample before keeps its own
	// time: the decoder drops a keyframe sent twice in a row as a duplicate,
	// while an entry from the sample time decodes it as before.
	inpoints, keyframes = make([]float64, len(targets)), make([]float64, len(targets))
	previous := math.NaN()
	for i, target := range targets {
		inpoints[i], keyframes[i] = target, math.NaN()
		n, _ := slices.BinarySearch(keys, target+sampleMatchSeconds)
		if n == 0 {
			continue
		}
		keyframes[i] = keys[n-1]
		if keys[n-1] != previous {
			inpoints[i] = keys[n-1]
			previous = keys[n-1]
		}
	}
	return inpoints, keyframes
}

// parseKeyframeTimes reads ffprobe's "pts_time,flags" packet lines and
// returns the keyframes' times, sorted and finite.
func parseKeyframeTimes(out []byte) []float64 {
	var keys []float64
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		raw, flags, ok := strings.Cut(strings.TrimSpace(scanner.Text()), ",")
		if !ok || !strings.HasPrefix(flags, "K") {
			continue
		}
		if at, err := strconv.ParseFloat(raw, 64); err == nil && finite(at) && at >= 0 {
			keys = append(keys, at)
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}
