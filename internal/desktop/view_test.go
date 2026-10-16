package desktop

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// hasText reports whether the frame shows exactly want as one text. The tester
// matches whole texts, so a substring does not count.
func hasText(tt *ui.Tester, want string) bool {
	return slices.Contains(tt.Texts(), want)
}

// TestViewShowsTheSections locks the sidebar: every section is offered.
func TestViewShowsTheSections(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	for _, want := range []string{"转换队列", "选项", "日志"} {
		if !hasText(tt, want) {
			t.Errorf("the sidebar does not offer %q; texts = %q", want, tt.Texts())
		}
	}
}

// TestViewShowsTheDropHintWhenTheQueueIsEmpty locks the empty state: with
// nothing queued, the queue pane asks for files rather than showing a list.
func TestViewShowsTheDropHintWhenTheQueueIsEmpty(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	if !hasText(tt, "把 .ncm 文件或文件夹拖进窗口") {
		t.Errorf("the empty queue does not show the drop hint; texts = %q", tt.Texts())
	}
}

// TestViewShowsTheEmptyLog locks that the log pane says it is empty.
func TestViewShowsTheEmptyLog(t *testing.T) {
	a := newApp()
	a.section = sectionLog
	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	if !hasText(tt, "还没有记录。") {
		t.Errorf("the empty log does not say so; texts = %q", tt.Texts())
	}
}

// TestViewShowsTheQueue locks that a queued source is listed, and the drop hint
// is gone once there is one.
func TestViewShowsTheQueue(t *testing.T) {
	a := newApp()
	a.addPaths([]string{"/music/a.ncm"})

	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	if !hasText(tt, "/music/a.ncm") {
		t.Errorf("the queue does not show the added path; texts = %q", tt.Texts())
	}
	if hasText(tt, "把 .ncm 文件或文件夹拖进窗口") {
		t.Error("the drop hint is still shown with a non-empty queue")
	}
}

// TestViewAddsAPathOnlyOnce locks that dropping a path twice queues it once.
func TestViewAddsAPathOnlyOnce(t *testing.T) {
	a := newApp()
	a.addPaths([]string{"/music/a.ncm", "/music/a.ncm"})
	if len(a.queue) != 1 {
		t.Errorf("queue = %v, want one entry", a.queue)
	}
}

// TestViewShowsWhereEachSourceIs locks that a queued source shows the stage it
// has reached, so the flow of a run is readable from the list.
func TestViewShowsWhereEachSourceIs(t *testing.T) {
	a := newApp()
	a.addPaths([]string{"/music/a.ncm", "/music/b.ncm"})
	a.queue[0].scanned, a.queue[0].total = true, 3
	a.queue[0].started, a.queue[0].done = true, 1
	a.queue[1].scanned, a.queue[1].total = true, 3

	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	for _, want := range []string{"转换中 · 1/3", "待处理 · 3 个"} {
		if !hasText(tt, want) {
			t.Errorf("the queue does not show %q; texts = %q", want, tt.Texts())
		}
	}
}

// TestQueueEntryStatus locks what each stage of a source reads as.
func TestQueueEntryStatus(t *testing.T) {
	tests := []struct {
		name   string
		entry  queueEntry
		dryRun bool
		want   string
		kind   int
	}{
		{"not scanned yet", queueEntry{}, false, "待处理", kindPending},
		{"nothing to do", queueEntry{scanned: true}, false, "无 .ncm", kindEmpty},
		{"waiting with work", queueEntry{scanned: true, total: 4}, false, "待处理 · 4 个", kindPending},
		{"in flight", queueEntry{scanned: true, total: 4, started: true, done: 2}, false, "转换中 · 2/4", kindActive},
		{
			"converted",
			queueEntry{scanned: true, total: 4, started: true, done: 4, converted: 3, skipped: 1},
			false, "3 已转换 · 1 跳过", kindDone,
		},
		{
			"converted with a failure",
			queueEntry{scanned: true, total: 2, started: true, done: 2, converted: 1, failed: 1},
			false, "1 已转换 · 1 失败", kindFailed,
		},
		{
			"a dry run",
			queueEntry{scanned: true, total: 2, started: true, done: 2, converted: 2},
			true, "2 个将转换", kindDone,
		},
		{"an unreadable path", queueEntry{invalid: true}, false, "路径不可用", kindInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			label, kind := test.entry.status(test.dryRun)
			if label != test.want || kind != test.kind {
				t.Errorf("status() = (%q, %d), want (%q, %d)", label, kind, test.want, test.kind)
			}
		})
	}
}

// TestViewShowsTheRunOutcomes locks the log of a finished run.
func TestViewShowsTheRunOutcomes(t *testing.T) {
	a := newApp()
	a.section = sectionLog
	a.log = []Report{
		{Source: "/music/a.ncm", Dest: "/music/a.mp3", Outcome: "converted"},
		{Source: "/music/b.ncm", Outcome: "failed", Err: "not an ncm container"},
	}

	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	for _, want := range []string{
		"已转换",
		"失败",
		"/music/a.ncm → /music/a.mp3",
		"not an ncm container",
	} {
		if !hasText(tt, want) {
			t.Errorf("the log does not show %q; texts = %q", want, tt.Texts())
		}
	}
}

// TestViewShowsTheRunSummary locks the summary card: the counts of a finished
// run, and where its results are.
func TestViewShowsTheRunSummary(t *testing.T) {
	a := newApp()
	a.hasRun = true
	a.counts = Result{Total: 3, Converted: 1, Skipped: 1, Failed: 1, DurationMs: 1200}
	a.summary = summarize(a.counts)

	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	for _, want := range []string{"已转换 1", "已跳过 1", "失败 1", "共 3 个 · 1.2s"} {
		if !hasText(tt, want) {
			t.Errorf("the summary card does not show %q; texts = %q", want, tt.Texts())
		}
	}
}

// TestViewShowsTheRunStateFromEverySection locks that the state of the run is
// visible from a section other than the queue.
func TestViewShowsTheRunStateFromEverySection(t *testing.T) {
	a := newApp()
	a.section = sectionOptions
	a.hasRun = true
	a.counts = Result{Total: 2, Converted: 2, DurationMs: 500}

	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	if !hasText(tt, "已完成") {
		t.Errorf("the run state is not shown outside the queue; texts = %q", tt.Texts())
	}
}

// TestViewShowsAFailedRun locks that a run that could not start says why.
func TestViewShowsAFailedRun(t *testing.T) {
	a := newApp()
	a.hasRun = true
	a.fault = "inspect input: no such file or directory"

	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()

	for _, want := range []string{"运行失败", "inspect input: no such file or directory"} {
		if !hasText(tt, want) {
			t.Errorf("the failed run does not show %q; texts = %q", want, tt.Texts())
		}
	}
}

// TestSummarize locks the wording beside the counts.
func TestSummarize(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   string
	}{
		{
			name:   "a plain run",
			result: Result{Total: 3, Converted: 1, Skipped: 1, Failed: 1, DurationMs: 2500},
			want:   "共 3 个 · 2.5s",
		},
		{
			name:   "a dry run",
			result: Result{Total: 2, DurationMs: 10, DryRun: true},
			want:   "共 2 个 · 0.0s · 试运行",
		},
		{
			name:   "a cancelled run",
			result: Result{Total: 1, Converted: 1, DurationMs: 500, Canceled: true},
			want:   "共 1 个 · 0.5s · 已取消",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := summarize(test.result); got != test.want {
				t.Errorf("summarize() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestProgressSummary locks what a run in flight reads as.
func TestProgressSummary(t *testing.T) {
	if got, want := progressSummary(3, 40), "转换中… 3 / 40"; got != want {
		t.Errorf("progressSummary() = %q, want %q", got, want)
	}
}

// TestOutcomeLabels locks the Chinese wording of every outcome.
func TestOutcomeLabels(t *testing.T) {
	tests := map[string]string{
		"planned":   "将转换",
		"converted": "已转换",
		"skipped":   "已跳过",
		"failed":    "失败",
	}
	for outcome, want := range tests {
		if got := outcomeLabel(outcome); got != want {
			t.Errorf("outcomeLabel(%q) = %q, want %q", outcome, got, want)
		}
	}
}

// TestSectionTitles locks the wording of the sections.
func TestSectionTitles(t *testing.T) {
	tests := map[string]string{
		sectionQueue:   "转换队列",
		sectionOptions: "选项",
		sectionLog:     "日志",
	}
	for section, want := range tests {
		if got := sectionTitle(section); got != want {
			t.Errorf("sectionTitle(%q) = %q, want %q", section, got, want)
		}
	}
}
