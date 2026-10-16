package desktop

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chilfish/ncmconverter/internal/app"
)

// Request is one run of the converter, mirroring app.Options.
type Request struct {
	Inputs         []string
	Output         string
	OutputTemplate string
	Tag            bool
	Depth          int
	Threads        int
	DryRun         bool
	SkipExisting   bool
}

// Report is how one container was handled, shown in the log as it happens.
type Report struct {
	Source  string
	Dest    string
	Outcome string
	Err     string
}

// Result summarizes a run.
type Result struct {
	Total      int
	Converted  int
	Skipped    int
	Failed     int
	Canceled   bool
	DryRun     bool
	DurationMs int
}

// runner drives internal/app for the window: it keeps a run single-flight and
// normalizes the errors the view shows. It holds no window, so it can be
// driven from a test.
type runner struct {
	mu      sync.Mutex
	running bool
	freed   context.CancelFunc
}

// convert runs request, calling onFile once per container as it is handled.
//
// A per-file failure is visible in the reports and counted in Result.Failed,
// not returned: only an unusable input or a bad thread count is an error.
// Cancellation is not a failure either: it sets Result.Canceled.
func (r *runner) convert(ctx context.Context, request Request, onFile func(Report)) (Result, error) {
	ctx, release, err := r.begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer r.finish(release)
	return runConvert(ctx, request, onFile)
}

// begin starts a run, refusing a second one while the first is in flight, and
// returns a cancellable context for it.
func (r *runner) begin(ctx context.Context) (context.Context, context.CancelFunc, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil, nil, errors.New("a conversion is already running")
	}
	ctx, release := context.WithCancel(ctx)
	r.running = true
	r.freed = release
	return ctx, release, nil
}

// finish releases the single-flight guard a successful begin took.
func (r *runner) finish(release context.CancelFunc) {
	release()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.freed = nil
}

// runConvert is the window-free core: it maps each app.FileResult to a Report,
// counts the outcomes and normalizes the error.
func runConvert(ctx context.Context, request Request, onFile func(Report)) (Result, error) {
	start := time.Now()
	result := Result{DryRun: request.DryRun}

	options := app.Options{
		Inputs:         request.Inputs,
		Output:         request.Output,
		OutputTemplate: request.OutputTemplate,
		Tag:            request.Tag,
		Depth:          request.Depth,
		Threads:        request.Threads,
		DryRun:         request.DryRun,
		SkipExisting:   request.SkipExisting,
		// The callback runs inside the mutex app.Run holds, so its calls are
		// serialized and the tally needs no lock of its own.
		OnFile: func(file app.FileResult) {
			result.Total++
			switch file.Outcome {
			case app.OutcomeConverted:
				result.Converted++
			case app.OutcomeSkipped:
				result.Skipped++
			case app.OutcomeFailed:
				result.Failed++
			}
			if onFile != nil {
				onFile(reportOf(file))
			}
		},
	}

	runErr := app.Run(ctx, options)
	result.DurationMs = int(time.Since(start).Milliseconds())

	switch {
	case errors.Is(runErr, context.Canceled):
		result.Canceled = true
		return result, nil
	case result.Failed > 0:
		// Each failure is already a report of its own, so an aggregate error
		// adds nothing the window can show.
		return result, nil
	default:
		return result, runErr
	}
}

// scan returns the containers a run with these inputs and depth would handle.
func scan(inputs []string, depth int) ([]string, error) {
	return app.Scan(inputs, depth)
}

// sourceSize is one source's share of the work a run would do.
type sourceSize struct {
	total   int
	invalid bool
}

// scanBySource expands every source on its own, so a report can be attributed
// to the source it came from, and gives each source the containers no earlier
// source claimed, the way a run deduplicates overlapping inputs.
//
// A source that cannot be read is marked rather than failing the whole scan, so
// one stale path does not blank the queue's counts; the run itself still
// reports it.
func scanBySource(paths []string, depth int) (map[string]int, []sourceSize) {
	owners := make(map[string]int)
	sizes := make([]sourceSize, len(paths))
	for i, path := range paths {
		found, err := app.Scan([]string{path}, depth)
		if err != nil {
			sizes[i].invalid = true
			continue
		}
		for _, container := range found {
			if _, claimed := owners[container]; claimed {
				continue
			}
			owners[container] = i
			sizes[i].total++
		}
	}
	return owners, sizes
}

// reportOf maps one conversion result to what the log shows.
func reportOf(file app.FileResult) Report {
	report := Report{
		Source:  file.Source,
		Dest:    file.Dest,
		Outcome: string(file.Outcome),
	}
	if file.Err != nil {
		report.Err = file.Err.Error()
	}
	return report
}
