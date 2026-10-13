package mediasample

import (
	"fmt"
	"slices"
	"strings"
)

// A sampled list that dropped keyframes as duplicates (see samples) leaves
// the samples after each dropped keyframe empty. Each of them is fetched
// again from an input of its own: every ffmpeg input has its own decoder, so
// no earlier keyframe is left in it to collide with. One run takes up to
// maxRefetchInputs inputs, each seeking to the sample's keyframe and passing
// on its first frame, tagged with the sample, and a concat filter joins
// them into the same raw frames and log lines a list run gives.

// maxRefetchInputs bounds the inputs, and so the open demuxers and
// decoders, of one refetch run.
const maxRefetchInputs = 32

// refetchSeconds bounds how much of the input each refetch input reads
// after its keyframe.
const refetchSeconds = 1

// missingCells returns the cells no frame landed in. Only a deferred
// assembler still holds every sheet to look at.
func (a *sheetAssembler) missingCells() []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.deferEncode || !a.droppedKeyframes || a.err != nil {
		return nil
	}
	var missing []int
	for cell := range a.times {
		canvas := a.open[cell/a.perSheet]
		if canvas == nil || !canvas.set[cell%a.perSheet] {
			missing = append(missing, cell)
		}
	}
	return missing
}

// beginRun readies the assembler for another ffmpeg run, whose frame
// counter starts at zero again.
func (a *sheetAssembler) beginRun() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logged, a.current = 0, nil
}

// abandonRun drops what a failed run left half joined, keeping the frames
// it placed, and clears an error the run itself caused.
func (a *sheetAssembler) abandonRun(prior error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, frame := range a.queued {
		a.recycle(frame)
	}
	a.recycle(a.partial)
	a.partial, a.queued, a.queuedBytes, a.metas, a.current = nil, nil, 0, nil, nil
	a.err = prior
}

// refetch fetches the missing cells of a sampled sheets run into the
// assembler. inpoints are the list entries' container times. A run that
// fails leaves its samples empty; only the attempt's timeout or the
// caller's cancellation stops the refetch with a failure.
func (a attemptRun) refetch(req Request, assembler *sheetAssembler, inpoints []float64) *AttemptError {
	chain, ok := sheetChain(a.sheetsGraph)
	if !ok {
		return nil
	}
	missing := assembler.missingCells()
	for batch := range slices.Chunk(missing, maxRefetchInputs) {
		args, err := buildRefetchArgs(req, a.attempt, a.hw, chain, assembler.times, inpoints, batch)
		if err != nil {
			return nil
		}
		assembler.beginRun()
		prior := assembler.err
		if failure := a.exec(req, args, nil, assembler, assembler.line); failure != nil {
			if failure.Reason == ReasonTimeout || failure.Reason == ReasonCanceled {
				return failure
			}
			assembler.abandonRun(prior)
		}
	}
	return nil
}

// buildRefetchArgs returns the ffmpeg arguments fetching cells, each from
// its own input seeking to its inpoint.
func buildRefetchArgs(req Request, attempt Attempt, hw hardwareDecode, chain string, times, inpoints []float64, cells []int) ([]string, error) {
	var global, perInput []string
	if attempt.Hardware {
		decode, err := hardwareDecodeArgs(hw, false)
		if err != nil {
			return nil, err
		}
		// Device setup is global; the hwaccel options apply per input.
		split := slices.Index(decode, hwaccelOption)
		if split < 0 {
			split = len(decode)
		}
		global, perInput = decode[:split], decode[split:]
	}
	args := quietArgs(logLevel)
	if req.Threads > 0 {
		threads := fmt.Sprint(req.Threads)
		args = append(args, "-threads", threads, "-filter_threads", threads)
	}
	args = append(args, global...)
	var graph, joined strings.Builder
	for i, cell := range cells {
		args = append(args, perInput...)
		args = append(args, "-skip_frame:v", "nokey", "-noaccurate_seek",
			"-ss", formatSampleSeconds(inpoints[cell]), "-t", fmt.Sprint(refetchSeconds), "-i", req.Input)
		fmt.Fprintf(&graph, "[%d:V:0]trim=end_frame=1,%s,metadata=mode=add:key=%s:value=%s[s%d];",
			i, chain, sampleMetadataKey, formatSampleSeconds(times[cell]), i)
		fmt.Fprintf(&joined, "[s%d]", i)
	}
	fmt.Fprintf(&graph, "%sconcat=n=%d:v=1:a=0,%s@%s=print[out]", joined.String(), len(cells), filterMetadata, sheetMetadataID)
	return append(args,
		"-filter_complex", graph.String(), "-map", "[out]",
		"-fps_mode", "passthrough", pixelFormatOption, sheetPixelFormat,
		"-f", "rawvideo", "pipe:1",
	), nil
}
