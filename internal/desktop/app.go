// Package desktop shows the converter in a MyGo window of native UI.
//
// The window draws itself from Go: there is no web page and no bindings, and
// the view calls internal/app, which stays unaware of the desktop, so both
// entry points share one conversion implementation.
package desktop

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/chilfish/ncmconverter/internal/app"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Run starts the desktop application and returns when it quits.
//
// It is called by the shared binary when it is started without arguments.
func Run() error {
	a := newApp()
	mygo.App.SetName("NCMconverter")
	mygo.App.WhenReady(func() {
		a.build = buildInfo()
		mygo.App.SetMenu(newMenu(a))
		a.openWindow()
	})
	// With a listener the app does not quit with its last window. macOS keeps
	// it in the Dock, where a click opens a window again; elsewhere there is no
	// Dock to come back from, so quit.
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.openWindow()
		}
	})
	return mygo.App.Run()
}

// queueEntry is one source in the queue, and how far the run has got with it.
type queueEntry struct {
	path string

	// scanned is set once the source has been expanded, so a source reads as
	// waiting rather than as holding nothing before that.
	scanned bool
	invalid bool
	total   int // containers this source owns in a run

	// The current run's tally for this source.
	started   bool
	done      int
	converted int
	skipped   int
	failed    int
}

// The states a queued source can be in, and the mark drawn before it.
const (
	kindPending = iota // waiting, or nothing to do yet
	kindActive         // being converted now
	kindDone           // converted, with nothing failed
	kindFailed         // converted, with failures
	kindEmpty          // holds no .ncm
	kindInvalid        // the path cannot be read
)

// status is what a source shows beside its path, and the kind of mark before it.
func (e *queueEntry) status(dryRun bool) (string, int) {
	switch {
	case e.invalid:
		return "路径不可用", kindInvalid
	case !e.scanned:
		return "待处理", kindPending
	case e.total == 0:
		return "无 .ncm", kindEmpty
	case e.done >= e.total:
		return e.resultText(dryRun), outcomeKind(e.failed)
	case e.started:
		return fmt.Sprintf("转换中 · %d/%d", e.done, e.total), kindActive
	default:
		return fmt.Sprintf("待处理 · %d 个", e.total), kindPending
	}
}

// resultText describes a source the run has finished.
func (e *queueEntry) resultText(dryRun bool) string {
	if dryRun {
		return fmt.Sprintf("%d 个将转换", e.total)
	}
	parts := []string{fmt.Sprintf("%d 已转换", e.converted)}
	if e.skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d 跳过", e.skipped))
	}
	if e.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d 失败", e.failed))
	}
	return strings.Join(parts, " · ")
}

// outcomeKind is the mark a source that finished shows.
func outcomeKind(failed int) int {
	if failed > 0 {
		return kindFailed
	}
	return kindDone
}

// App is what the window shows: the queue, the options and how the run is
// going.
//
// The view reads and writes it on the main thread only. The conversion runs on
// a goroutine of its own and hands its results back with Window.Update.
type App struct {
	win   *mygo.Window
	run   runner
	build string

	// The queue and the options, bound to the controls by pointer.
	queue        []*queueEntry
	output       string
	template     string
	depth        float64
	threads      float64
	tag          bool
	skipExisting bool
	dryRun       bool

	// The run in flight, or the last one.
	hasRun    bool
	running   bool
	cancelRun context.CancelFunc
	owners    map[string]int // a container to the queue entry that owns it
	progress  float64
	planned   int
	done      int
	counts    Result
	summary   string
	fault     string
	log       []Report
	firstDest string
	outputDir string
	stats     string
	showToast string
	scanToken int

	// The section the sidebar shows.
	section string

	// Widget state that lasts from frame to frame.
	queueList ui.ListState
	logList   ui.ListState
}

// newApp returns the state the window shows before anything ran.
func newApp() *App {
	return &App{
		threads: 10,
		tag:     true,
		summary: "尚未开始。",
		section: sectionQueue,
	}
}

// openWindow creates the main window.
//
// Its title bar is hidden, so the view draws it under the window controls, and
// it carries a material that the sidebar shows through.
func (a *App) openWindow() {
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "NCMconverter",
		Width:     900,
		Height:    640,
		MinWidth:  680,
		MinHeight: 460,
		StateKey:  "main",
		// The view draws the title bar, so the window controls sit over it,
		// inset over the sidebar on macOS.
		TitleBarStyle:  mygo.TitleBarHiddenInset,
		TitleBarHeight: titleBarHeight,
		// The material shows wherever the view draws no background: the
		// sidebar has none, the content pane has one.
		Vibrancy: mygo.VibrancySidebar,
		Content:  ui.View(a.view),
	})
	// Files dropped anywhere the view does not take them, which is anywhere
	// once the queue is no longer empty.
	a.win.OnFileDrop(func(event *mygo.FileDropEvent) { a.addPaths(event.Paths) })
}

// buildInfo describes the running build, shown in the header.
func buildInfo() string {
	version := mygo.App.Version()
	if version == "" {
		version = "dev"
	} else {
		version = "v" + version
	}
	return fmt.Sprintf("%s · %s/%s", version, runtime.GOOS, runtime.GOARCH)
}

// newMenu builds the application menu.
func newMenu(a *App) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "添加文件…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.pickFiles() }},
			{Label: "添加文件夹…", Accelerator: "CmdOrCtrl+Shift+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.pickDirectory() }},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Label: "转换", Submenu: []*mygo.MenuItem{
			{Label: "开始转换", Accelerator: "CmdOrCtrl+Return", Click: func(*mygo.MenuItem, *mygo.Window) { a.start() }},
			{Label: "取消", Click: func(*mygo.MenuItem, *mygo.Window) { a.cancel() }},
			mygo.Separator(),
			{Label: "打开输出目录", Click: func(*mygo.MenuItem, *mygo.Window) { a.openOutput() }},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	})
}

// update runs fn on the main thread and draws a new frame. Use it from
// goroutines; the view itself runs on the main thread and changes the state
// directly.
func (a *App) update(fn func()) {
	if a.win == nil {
		return
	}
	a.win.Update(fn)
}

// toast asks for a message the next frame shows as a toast.
func (a *App) toast(message string) {
	a.update(func() { a.showToast = message })
}

// pickFiles asks for containers and adds them to the queue.
//
// Dialogs block, and menu clicks and button clicks run on the main thread,
// which draws the view, so the dialog and the update happen on a goroutine of
// their own.
func (a *App) pickFiles() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:   a.win,
			Title:    "选择 .ncm 文件",
			Filters:  []mygo.FileFilter{{Name: "NCM 文件", Extensions: []string{"ncm"}}},
			Multiple: true,
		})
		if err != nil {
			a.toast("选择文件失败：" + err.Error())
			return
		}
		a.update(func() { a.addPaths(paths) })
	}()
}

// pickDirectory asks for a directory and adds it to the queue.
func (a *App) pickDirectory() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     "选择文件夹",
			Directory: true,
		})
		if err != nil {
			a.toast("选择文件夹失败：" + err.Error())
			return
		}
		a.update(func() { a.addPaths(paths) })
	}()
}

// pickOutput asks for the output directory and fills the field with it.
func (a *App) pickOutput() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    a.win,
			Title:     "选择输出目录",
			Directory: true,
		})
		if err != nil {
			a.toast("选择输出目录失败：" + err.Error())
			return
		}
		if len(paths) == 0 {
			return
		}
		directory := paths[0]
		a.update(func() { a.output = directory })
	}()
}

// openOutput opens the directory the run wrote to with the file manager.
func (a *App) openOutput() {
	if a.outputDir == "" {
		return
	}
	directory := a.outputDir
	go func() {
		if err := mygo.Shell.OpenPath(directory); err != nil {
			a.toast("打开目录失败：" + err.Error())
		}
	}()
}

// paths is the queue's inputs, in order.
func (a *App) paths() []string {
	paths := make([]string, len(a.queue))
	for i, entry := range a.queue {
		paths[i] = entry.path
	}
	return paths
}

// holds reports whether the queue already has the path.
func (a *App) holds(path string) bool {
	return slices.ContainsFunc(a.queue, func(entry *queueEntry) bool { return entry.path == path })
}

// addPaths adds the paths to the queue, without duplicates.
func (a *App) addPaths(paths []string) {
	changed := false
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || a.holds(path) {
			continue
		}
		a.queue = append(a.queue, &queueEntry{path: path})
		changed = true
	}
	if changed {
		a.refreshStats()
	}
}

// removeAt takes one input out of the queue.
func (a *App) removeAt(index int) {
	if index < 0 || index >= len(a.queue) {
		return
	}
	a.queue = slices.Delete(a.queue, index, index+1)
	a.refreshStats()
}

// clearQueue empties the queue.
func (a *App) clearQueue() {
	a.queue = nil
	a.owners = nil
	a.refreshStats()
}

// refreshStats expands each source to count the containers it would convert,
// off the main thread so a large directory does not stall the view. It records
// the containers of each source, which is how a run's reports are later
// attributed to the source they came from.
func (a *App) refreshStats() {
	a.scanToken++
	token := a.scanToken
	if len(a.queue) == 0 {
		a.stats = ""
		return
	}
	paths := a.paths()
	depth := int(math.Round(a.depth))
	a.stats = "正在统计…"
	go func() {
		owners, sizes := scanBySource(paths, depth)
		a.update(func() {
			// A slow scan must not overwrite the result of a newer one.
			if a.scanToken != token {
				return
			}
			a.owners = owners
			for i, entry := range a.queue {
				if i >= len(sizes) {
					break
				}
				entry.total, entry.invalid, entry.scanned = sizes[i].total, sizes[i].invalid, true
			}
			a.stats = a.statsLine()
		})
	}()
}

// statsLine describes the queue: how many sources, and how much work.
func (a *App) statsLine() string {
	containers := 0
	for _, entry := range a.queue {
		containers += entry.total
	}
	return fmt.Sprintf("%d 个来源 · %d 个 .ncm", len(a.queue), containers)
}

// start begins a run. It is called on the main thread, from the button, the
// menu or the shortcut, and returns at once.
func (a *App) start() {
	if a.running || len(a.queue) == 0 {
		return
	}
	request := a.request()
	paths := a.paths()
	ctx, cancel := context.WithCancel(context.Background())

	// Every source starts over, so the queue shows this run's progress.
	for _, entry := range a.queue {
		entry.started, entry.done = false, 0
		entry.converted, entry.skipped, entry.failed = 0, 0, 0
	}
	a.hasRun = true
	a.running = true
	a.cancelRun = cancel
	a.owners = nil
	a.progress = 0
	a.planned = 0
	a.done = 0
	a.counts = Result{}
	a.log = nil
	a.firstDest = ""
	a.outputDir = ""
	a.showToast = ""
	a.fault = ""
	a.summary = progressSummary(0, 0)

	go a.runRequest(ctx, request, paths)
}

// cancel stops the run in progress. A run that is not running ignores it.
func (a *App) cancel() {
	if a.cancelRun != nil {
		a.cancelRun()
	}
}

// request snapshots the queue and the options into what a run takes.
func (a *App) request() Request {
	return Request{
		Inputs:         a.paths(),
		Output:         strings.TrimSpace(a.output),
		OutputTemplate: strings.TrimSpace(a.template),
		Tag:            a.tag,
		Depth:          int(math.Round(a.depth)),
		Threads:        int(math.Round(a.threads)),
		DryRun:         a.dryRun,
		SkipExisting:   a.skipExisting,
	}
}

// runRequest converts off the main thread, handing every result back to the
// view as it happens and the summary at the end.
func (a *App) runRequest(ctx context.Context, request Request, paths []string) {
	// Expand the sources first, so each report can be attributed to the source
	// it came from and the progress bar has a denominator.
	owners, sizes := scanBySource(paths, request.Depth)
	a.update(func() {
		a.owners = owners
		a.planned = len(owners)
		for i, entry := range a.queue {
			if i >= len(sizes) {
				break
			}
			entry.total, entry.invalid, entry.scanned = sizes[i].total, sizes[i].invalid, true
		}
		a.summary = progressSummary(0, a.planned)
	})

	result, err := a.run.convert(ctx, request, func(report Report) {
		a.update(func() { a.record(report) })
	})
	a.update(func() { a.finish(result, err) })
}

// record adds one container's result to the log, the tally and the source it
// came from. It runs on the main thread, where the view reads the same state.
func (a *App) record(report Report) {
	a.log = append(a.log, report)
	a.done++
	a.counts.Total++
	switch report.Outcome {
	case string(app.OutcomeConverted):
		a.counts.Converted++
	case string(app.OutcomeSkipped):
		a.counts.Skipped++
	case string(app.OutcomeFailed):
		a.counts.Failed++
	}
	if report.Dest != "" && a.firstDest == "" {
		a.firstDest = report.Dest
	}
	if index, ok := a.owners[report.Source]; ok && index < len(a.queue) && a.queue[index].path == report.Source {
		entry := a.queue[index]
		entry.started = true
		entry.done++
		switch report.Outcome {
		case string(app.OutcomeConverted), string(app.OutcomePlanned):
			entry.converted++
		case string(app.OutcomeSkipped):
			entry.skipped++
		case string(app.OutcomeFailed):
			entry.failed++
		}
	}
	if a.planned > 0 {
		a.progress = float64(a.done) / float64(a.planned)
	}
	a.summary = progressSummary(a.done, a.planned)
}

// finish ends a run: the summary, and where "open the output directory" goes.
func (a *App) finish(result Result, err error) {
	a.running = false
	a.cancelRun = nil
	a.progress = 1
	a.counts = result
	if err != nil {
		a.fault = err.Error()
		a.summary = ""
		return
	}
	a.summary = summarize(result)
	switch {
	case strings.TrimSpace(a.output) != "":
		a.outputDir = strings.TrimSpace(a.output)
	case a.firstDest != "":
		// No output directory set, so the converted files went beside their
		// sources; open where the first one landed.
		a.outputDir = filepath.Dir(a.firstDest)
	}
}
