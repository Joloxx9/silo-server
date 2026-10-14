package mediasample

import (
	"fmt"
	"slices"
	"strings"
)

// A sampled list runs every entry through one decoder, which jumps can
// confuse: HEVC drops open-GOP keyframes as duplicates (see samples), and
// H.264 discards keyframes over an "illegal short term buffer state". The
// samples a list leaves empty are fetched again, each from an input of its
// own: every ffmpeg input has its own decoder, with nothing left in it from
// another sample. A damaged stretch stays empty and still counts as filled. One run takes up to
// maxRefetchInputs inputs, each seeking to the sample's keyframe and passing
// on its first frame, tagged with the sample, and a concat filter joins
// them into the same raw frames and log lines a list run gives.

// A list that left most samples empty, as an H.264 stream whose keyframes
// all reference skipped pictures does, is cheaper read as one window than
// fetched a sample at a time, so the refetch takes at most maxRefetchShare
// of the samples, or minRefetchCells if that is more.
const (
	maxRefetchShare = 0.5
	minRefetchCells = 16
)

// maxRefetchKeyframeGap is how far before its sample a keyframe may lie and
// still be fetched for it. Keyframes lie seconds apart; a sample whose last
// keyframe is further back sits in a stretch of the file with no pictures,
// as a damaged or cut file has, and stays filled so the run can tell.
const maxRefetchKeyframeGap = 30.0

// maxRefetchInputs bounds the inputs, and so the open demuxers and
// decoders, of one refetch run.
const maxRefetchInputs = 16

// refetchSeconds bounds how much of the input each refetch input reads
// after its keyframe.
const refetchSeconds = 1

// missingCells returns the cells no frame landed in. Only a deferred
// assembler still holds every sheet to look at.
func (a *sheetAssembler) missingCells() []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.deferEncode || a.err != nil {
		return nil
	}
	var missing []int
	for cell := range a.times {
		if !a.cellSet(cell) {
			missing = append(missing, cell)
		}
	}
	if len(missing) > max(minRefetchCells, int(maxRefetchShare*float64(len(a.times)))) {
		return nil
	}
	return missing
}

// stillMissing returns the cells of cells no frame has landed in yet.
func (a *sheetAssembler) stillMissing(cells []int) []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(cells), a.cellSet)
}

func (a *sheetAssembler) cellSet(cell int) bool {
	canvas := a.open[cell/a.perSheet]
	return canvas != nil && canvas.set[cell%a.perSheet]
}

// beginRun readies the assembler for another ffmpeg run, whose frame
// counter starts at zero again.
func (a *sheetAssembler) beginRun() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.logged, a.segment, a.current = 0, 0, nil
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
// assembler. inpoints are the list entries' container times; cells marked
// in stale, whose keyframe lies too far back, are not fetched. A run that
// fails is tried once more for the cells it left empty, as a busy host can
// fail one; cells still empty after that stay empty. Only the attempt's
// timeout or the caller's cancellation stops the refetch with a failure.
func (a attemptRun) refetch(req Request, assembler *sheetAssembler, inpoints []float64, stale []bool) *AttemptError {
	chain, ok := sheetChain(a.sheetsGraph)
	if !ok {
		return nil
	}
	missing := slices.DeleteFunc(assembler.missingCells(), func(cell int) bool { return stale != nil && stale[cell] })
	for batch := range slices.Chunk(missing, maxRefetchInputs) {
		for try := 0; try < 2 && len(batch) > 0; try++ {
			failure := a.refetchRun(req, assembler, chain, inpoints, batch)
			if failure == nil {
				break
			}
			if failure.Reason == ReasonTimeout || failure.Reason == ReasonCanceled {
				return failure
			}
			batch = assembler.stillMissing(batch)
		}
	}
	return nil
}

// refetchRun fetches cells in one ffmpeg run, dropping what a failed run
// left half joined.
func (a attemptRun) refetchRun(req Request, assembler *sheetAssembler, chain string, inpoints []float64, cells []int) *AttemptError {
	args, err := buildRefetchArgs(req, a.attempt, a.hw, chain, assembler.times, inpoints, cells)
	if err != nil {
		return &AttemptError{Reason: ReasonArgs, Err: err}
	}
	assembler.beginRun()
	prior := assembler.err
	failure := a.exec(req, args, nil, assembler, assembler.line)
	if failure != nil && failure.Reason != ReasonTimeout && failure.Reason != ReasonCanceled {
		assembler.abandonRun(prior)
	}
	return failure
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
