// Package desktop binds the converter to a MyGo desktop application.
//
// The service here is the only bridge between the pages and conversion: it
// drives internal/app, which stays unaware of the desktop, so both entry
// points share one implementation.
package desktop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/chilfish/ncmconverter/internal/app"
	"github.com/egoist/mygo"
)

// Opened carries the files or directories chosen from the menu to the page.
// The page's own buttons use the return value of PickFiles and PickDirectory
// instead; both paths end in the same place.
var Opened = mygo.NewEvent[[]string]("files:opened")

// Info describes the build the page is talking to.
type Info struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	MyGo     string `json:"myGo"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

// Request is one run of the converter, mirroring app.Options.
type Request struct {
	Inputs         []string `json:"inputs"`
	Output         string   `json:"output"`
	OutputTemplate string   `json:"outputTemplate"`
	Tag            bool     `json:"tag"`
	Depth          int      `json:"depth"`
	Threads        int      `json:"threads"`
	DryRun         bool     `json:"dryRun"`
	SkipExisting   bool     `json:"skipExisting"`
}

// Report is how one container was handled, sent to the page as it happens.
type Report struct {
	Source  string `json:"source"`
	Dest    string `json:"dest"`
	Outcome string `json:"outcome"`
	Error   string `json:"error"`
}

// Result summarizes a run.
type Result struct {
	Total      int  `json:"total"`
	Converted  int  `json:"converted"`
	Skipped    int  `json:"skipped"`
	Failed     int  `json:"failed"`
	Canceled   bool `json:"canceled"`
	DryRun     bool `json:"dryRun"`
	DurationMs int  `json:"durationMs"`
}

// Converter is the service the page binds to.
//
// Every method takes a pointer receiver: the struct holds a mutex, and a value
// receiver would copy it, which go vet reports as passing a lock by value.
type Converter struct {
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

// Info returns the name, versions and platform of the running app.
func (c *Converter) Info() Info {
	version := mygo.App.Version()
	if version == "" {
		version = "dev"
	}
	return Info{
		Name:     mygo.App.Name(),
		Version:  version,
		MyGo:     mygo.Version,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
}

// PickFiles asks for .ncm files and returns them, or an empty slice when the
// user cancels.
func (c *Converter) PickFiles(ctx context.Context) ([]string, error) {
	return pickPaths(ctx, mygo.OpenDialogOptions{
		Title:    "选择 .ncm 文件",
		Filters:  []mygo.FileFilter{{Name: "NCM 文件", Extensions: []string{"ncm"}}},
		Multiple: true,
	})
}

// PickDirectory asks for a directory and returns it, or an empty string when
// the user cancels.
func (c *Converter) PickDirectory(ctx context.Context) (string, error) {
	paths, err := pickPaths(ctx, mygo.OpenDialogOptions{
		Title:     "选择文件夹",
		Directory: true,
	})
	if err != nil {
		return "", err
	}
	return firstPath(paths), nil
}

// PickOutput asks for the output directory and returns it, or an empty string
// when the user cancels.
func (c *Converter) PickOutput(ctx context.Context) (string, error) {
	paths, err := pickPaths(ctx, mygo.OpenDialogOptions{
		Title:     "选择输出目录",
		Directory: true,
	})
	if err != nil {
		return "", err
	}
	return firstPath(paths), nil
}

// Scan returns the .ncm files a run with the same inputs and depth would
// handle, so the page can show the work before it starts.
func (c *Converter) Scan(inputs []string, depth int) ([]string, error) {
	return app.Scan(inputs, depth)
}

// Run converts the requested inputs, sending one Report per container as it is
// handled, and returns the summary.
//
// A per-file failure is reported and does not fail the call: it is visible in
// the reports and counted in Result.Failed. Cancellation is not a failure
// either: it sets Result.Canceled. Only an unusable input or a bad thread count
// returns an error.
func (c *Converter) Run(ctx context.Context, request Request, reports *mygo.Channel[Report]) (Result, error) {
	ctx, cancel, err := c.begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer c.finish(cancel)

	start := time.Now()
	result := Result{DryRun: request.DryRun}

	opts := app.Options{
		Inputs:         request.Inputs,
		Output:         request.Output,
		OutputTemplate: request.OutputTemplate,
		Tag:            request.Tag,
		Depth:          request.Depth,
		Threads:        request.Threads,
		DryRun:         request.DryRun,
		SkipExisting:   request.SkipExisting,
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
			// A closed channel only means the page stopped listening, which
			// must not abort the run.
			_ = reports.Send(reportOf(file))
		},
	}

	runErr := app.Run(ctx, opts)
	result.DurationMs = int(time.Since(start).Milliseconds())

	switch {
	case errors.Is(runErr, context.Canceled):
		result.Canceled = true
		return result, nil
	case result.Failed > 0:
		// Each failure is already a report of its own, so an aggregate error
		// adds nothing the page can show.
		return result, nil
	default:
		return result, runErr
	}
}

// Cancel stops the run in progress. A run that is not running ignores it.
func (c *Converter) Cancel() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// OpenPath opens a file or directory with the default application, used for the
// "open the output directory" action.
func (c *Converter) OpenPath(path string) error {
	return mygo.Shell.OpenPath(path)
}

// begin starts a run, refusing a second one while the first is in flight, and
// returns a cancellable context for it.
func (c *Converter) begin(ctx context.Context) (context.Context, context.CancelFunc, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil, nil, errors.New("a conversion is already running")
	}
	ctx, cancel := context.WithCancel(ctx)
	c.running = true
	c.cancel = cancel
	return ctx, cancel, nil
}

// finish releases the single-flight guard a successful begin took.
func (c *Converter) finish(cancel context.CancelFunc) {
	cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	c.cancel = nil
}

// reportOf maps one conversion result to what the page sees.
func reportOf(file app.FileResult) Report {
	report := Report{
		Source:  file.Source,
		Dest:    file.Dest,
		Outcome: string(file.Outcome),
	}
	if file.Err != nil {
		report.Error = file.Err.Error()
	}
	return report
}

// pickPaths shows a file or directory dialog and returns the chosen paths.
func pickPaths(ctx context.Context, opts mygo.OpenDialogOptions) ([]string, error) {
	opts.Parent = mygo.CallerWindow(ctx)
	paths, err := mygo.Dialog.Open(opts)
	if err != nil {
		return nil, err
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, nil
}

// firstPath returns the first of paths, or an empty string when there is none.
func firstPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}
