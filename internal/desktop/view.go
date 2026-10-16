package desktop

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chilfish/ncmconverter/internal/app"
	"github.com/egoist/mygo/ui"
)

// The window's geometry: a sidebar beside the content, under an inset title
// bar that the view draws itself.
const (
	titleBarHeight = 52
	sidebarWidth   = 216
	contentMaxW    = 680
)

// The sections the sidebar chooses between.
const (
	sectionQueue   = "queue"
	sectionOptions = "options"
	sectionLog     = "log"
)

// The icons, parsed from inline SVGs so the app ships no asset files. Their
// shapes are stroked in the text color, as the bundled icon sets are, so an
// icon takes the color of what it is in.
var (
	iconQueue   = icon(`<path d="M3 4.5h10M3 8h10M3 11.5h6"/>`)
	iconOptions = icon(`<path d="M3.5 5h9M3.5 11h9"/><circle cx="6" cy="5" r="1.7"/><circle cx="10" cy="11" r="1.7"/>`)
	iconLog     = icon(`<rect x="2.5" y="3" width="11" height="10" rx="2"/><path d="M5 6.5h6M5 9.5h4"/>`)
	iconEmpty   = icon(`<path d="M2.5 9.5 4.6 4h6.8l2.1 5.5v3h-11z"/><path d="M2.5 9.5h3l.8 1.6h3.4l.8-1.6h3"/>`)

	// The marks a queued source shows: waiting, done, and something to look at.
	iconWaiting = icon(`<circle cx="8" cy="8" r="5.6"/>`)
	iconDone    = icon(`<circle cx="8" cy="8" r="6"/><path d="M5.5 8.2 7.2 9.9l3.3-3.6"/>`)
	iconAlert   = icon(`<circle cx="8" cy="8" r="6"/><path d="M8 5.2v3.6M8 11h.01"/>`)
)

// icon parses one inline SVG.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" ` +
			`stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">` +
			shapes + `</svg>`))
}

// view builds the window from the state, for every frame.
//
// It is the window's Content: it runs on the main thread, and only reads and
// writes the state it is handed.
func (a *App) view(c *ui.Context) {
	a.tune(c)

	// A message an action asked for, shown as a toast in the frame after it.
	if a.showToast != "" {
		c.Toast(a.showToast)
		a.showToast = ""
	}
	// The shortcut the menu also offers.
	if !a.running && c.Shortcut(ui.Cmd, ui.KeyEnter) {
		a.start()
	}

	t := c.Theme()
	// The window's material shows wherever the view draws no background, so
	// the sidebar is left bare and the content pane is not. Where there is no
	// material the sidebar draws a background of its own instead.
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, t, vibrant)
		a.pane(c, t)
	})
}

// tune makes the default theme rounder, closer to the desktop's own apps.
// Everything else keeps following the system: the appearance, the accent
// color, the text size and the contrast.
func (a *App) tune(c *ui.Context) {
	t := *c.Theme()
	t.Radius = 8
	c.SetTheme(&t)
}

// sidebar is the source list choosing the section, over the window's material.
func (a *App) sidebar(c *ui.Context, t *ui.Theme, vibrant bool) {
	side := ui.Column(c).Width(sidebarWidth).Shrink(0)
	if !vibrant {
		side.Background(t.Surface)
	}
	side.Children(func() {
		// The band the window controls take, which also drags the window.
		ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()
		list := ui.Sidebar(c, &a.section, func() {
			ui.SidebarItem(c, sectionQueue, iconQueue, "转换队列").Children(func() {
				if n := len(a.queue); n > 0 {
					ui.Badge(c, strconv.Itoa(n))
				}
			})
			ui.SidebarItem(c, sectionOptions, iconOptions, "选项")
			ui.SidebarItem(c, sectionLog, iconLog, "日志").Children(func() {
				if n := len(a.log); n > 0 {
					ui.Badge(c, strconv.Itoa(n))
				}
			})
		}).Grow(1).Label("导航")
		if vibrant {
			list.Background(ui.Transparent)
		}
	})
}

// pane is the section chosen, under the title bar holding its actions.
func (a *App) pane(c *ui.Context, t *ui.Theme) {
	pane := ui.Column(c).Grow(1).MinWidth(0).Background(t.Background)
	pane.Children(func() {
		a.titleBar(c, t)
		ui.Divider(c)
		switch a.section {
		case sectionOptions:
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(t.Space(5), t.Space(6)).Gap(t.Space(4)).MaxWidth(contentMaxW).Children(func() {
					a.optionsBody(c, t)
				})
			})
		case sectionLog:
			a.logBody(c, t)
		default:
			a.queueBody(c, t)
		}
	})
}

// titleBar names the section, shows where the run is, and holds its actions.
// It drags the window by its empty room, and keeps clear of the window
// controls.
func (a *App) titleBar(c *ui.Context, t *ui.Theme) {
	bar := c.TitleBar()
	ui.Row(c).Height(titleBarHeight).Shrink(0).AlignItems(ui.Center).
		Padding(0, bar.Right+t.Space(4), 0, bar.Left+t.Space(4)).
		Gap(t.Space(2)).DragWindow().Children(func() {
		ui.Text(c, sectionTitle(a.section)).FontSize(15).Bold().SingleLine().Grow(1)
		if a.hasRun {
			// The state of the run stays visible from every section.
			if a.running {
				ui.Spinner(c).Label("转换中")
				ui.Textf(c, "%d / %d", a.done, a.planned).FontSize(12).TextColor(t.TextMuted)
			} else {
				label, color := runState(t, a.counts, a.fault != "")
				pill := ui.Badge(c, label)
				pill.Background(color.Alpha(0.15)).TextColor(color)
			}
		}
		if a.running {
			if ui.Button(c, "取消").Clicked() {
				a.cancel()
			}
		}
		ui.PrimaryButton(c, "开始转换").Disabled(a.running || len(a.queue) == 0).OnClick(a.start)
	})
}

// queueBody is the inputs to convert, and how the run is going with them.
func (a *App) queueBody(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Padding(t.Space(4), t.Space(5)).Gap(t.Space(3)).Children(func() {
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "添加文件…").Disabled(a.running).Clicked() {
				a.pickFiles()
			}
			if ui.Button(c, "添加文件夹…").Disabled(a.running).Clicked() {
				a.pickDirectory()
			}
			ui.Spacer(c)
			if ui.Button(c, "清空").Disabled(a.running || len(a.queue) == 0).Clicked() {
				a.clearQueue()
			}
		})
		if a.hasRun {
			a.statusCard(c, t)
		}
		if len(a.queue) == 0 {
			a.dropZone(c, t).Grow(1)
		} else {
			a.queueRows(c, t).Grow(1)
		}
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			if a.stats != "" {
				ui.Text(c, a.stats).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			if !a.hasRun {
				ui.Text(c, a.summary).FontSize(12).SingleLine().TextColor(t.TextMuted)
			}
		})
	})
}

// statusCard is the run at a glance: how far along, what came of it, and where
// the results are. It stays up after the run, so the outcome is not lost when
// the log scrolls.
func (a *App) statusCard(c *ui.Context, t *ui.Theme) {
	card := ui.Column(c).Padding(t.Space(3)).Gap(t.Space(2)).Radius(t.Radius).
		Background(t.Surface).Border(1, t.Border)
	card.Children(func() {
		if a.running {
			ui.Progress(c, a.progressValue()).Label("转换进度")
		}
		ui.Row(c).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
			a.pill(c, t, "已转换", a.counts.Converted, t.Success)
			a.pill(c, t, "已跳过", a.counts.Skipped, t.Warning)
			a.pill(c, t, "失败", a.counts.Failed, t.Danger)
			ui.Spacer(c)
			if a.summary != "" {
				ui.Text(c, a.summary).FontSize(12).SingleLine().TextColor(t.TextMuted)
			}
			if !a.running && !a.counts.DryRun {
				if ui.Button(c, "打开输出目录").Disabled(a.outputDir == "").Clicked() {
					a.openOutput()
				}
			}
		})
		if a.fault != "" {
			ui.Text(c, a.fault).FontSize(12).Selectable().TextColor(t.Danger)
		}
	})
}

// pill is one outcome of the run, tinted when it happened and quiet when it did
// not, so the counts that matter stand out.
func (a *App) pill(c *ui.Context, t *ui.Theme, label string, n int, color ui.Color) {
	el := ui.Badge(c, fmt.Sprintf("%s %d", label, n))
	if n == 0 {
		el.Background(t.TextMuted.Alpha(0.12)).TextColor(t.TextMuted)
		return
	}
	el.Background(color.Alpha(0.15)).TextColor(color)
}

// dropZone is the empty-queue hint, and takes files dragged onto it.
func (a *App) dropZone(c *ui.Context, t *ui.Theme) ui.Element {
	zone := ui.Column(c).Center().Gap(t.Space(2)).
		Radius(t.Radius).BorderWidth(1).BorderColor(t.Border).BorderStyle(ui.BorderDashed)
	if zone.FileDragOver() {
		zone.BorderColor(t.Accent).Background(t.Accent.Alpha(0.06))
	}
	zone.Children(func() {
		ui.Icon(c, iconEmpty).Size(28, 28).TextColor(t.TextMuted)
		ui.Text(c, "把 .ncm 文件或文件夹拖进窗口").TextColor(t.TextMuted)
		ui.Text(c, "或点上面的按钮添加").FontSize(12).TextColor(t.TextMuted)
	})
	if files := zone.DroppedFiles(); files != nil {
		a.addPaths(files)
	}
	return zone
}

// queueRows lists the queued sources, each with where it is in the run.
func (a *App) queueRows(c *ui.Context, t *ui.Theme) ui.Element {
	a.queueList.Key = func(row int) any { return a.queue[row].path }
	return ui.List(c, &a.queueList, len(a.queue), func(row int) {
		entry := a.queue[row]
		label, kind := entry.status(a.dryRun)
		color := statusColor(t, kind)
		rowEl := ui.Row(c).Padding(t.Space(1.5), t.Space(2.5)).Gap(t.Space(2)).
			AlignItems(ui.Center).Radius(6)
		if rowEl.Hovered() {
			rowEl.Background(t.SurfaceHover)
		}
		rowEl.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
		rowEl.ContextMenu(func(m *ui.Menu) {
			if m.Item("移除").Disabled(a.running).Chosen() {
				a.removeAt(row)
			}
		})
		rowEl.Children(func() {
			ui.Row(c).Width(16).Center().Children(func() {
				if kind == kindActive {
					ui.Spinner(c).Label("转换中")
				} else {
					ui.Icon(c, statusIcon(kind)).Size(15, 15).TextColor(color)
				}
			})
			ui.Text(c, entry.path).SingleLine().Grow(1).Tooltip(entry.path)
			ui.Text(c, label).FontSize(12).SingleLine().TextColor(color)
			ui.Button(c, "移除").Disabled(a.running).OnClick(func() { a.removeAt(row) })
		})
	})
}

// optionsBody is the options of a run, as a native form.
func (a *App) optionsBody(c *ui.Context, t *ui.Theme) {
	// Disabled while a run is in flight, so they cannot change under it.
	ui.Fieldset(c, "转换选项", func() {
		ui.Form(c, func() {
			ui.Field(c, "输出目录", func() {
				ui.Row(c).Gap(t.Space(1.5)).Children(func() {
					ui.TextInput(c, &a.output).Placeholder("留空则与源文件同目录").Grow(1)
					if ui.Button(c, "选择…").Clicked() {
						a.pickOutput()
					}
				})
			})
			ui.Field(c, "命名模板", func() {
				ui.TextInput(c, &a.template).Placeholder("{artist} - {title}")
			}).Description("{name} {title} {artist} {album} {id} {format}")
			ui.Field(c, "目录层级 depth", func() {
				if ui.NumberInput(c, &a.depth, 0, 99, 1).Changed() {
					a.refreshStats()
				}
			})
			ui.Field(c, "并发数 threads", func() {
				ui.NumberInput(c, &a.threads, 1, 64, 1)
			})
			ui.Field(c, "写入标签与封面", func() { ui.Switch(c, &a.tag) })
			ui.Field(c, "跳过已存在的文件", func() { ui.Switch(c, &a.skipExisting) })
			ui.Field(c, "试运行（只列出，不写入）", func() { ui.Switch(c, &a.dryRun) })
		})
	}).Disabled(a.running)
}

// logBody is what each container did, following its end as the run goes.
func (a *App) logBody(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Padding(t.Space(4), t.Space(5)).Gap(t.Space(2)).Children(func() {
		if len(a.log) == 0 {
			ui.Column(c).Grow(1).Center().Gap(t.Space(2)).Children(func() {
				ui.Icon(c, iconLog).Size(28, 28).TextColor(t.TextMuted)
				ui.Text(c, "还没有记录。").TextColor(t.TextMuted)
			})
			return
		}
		a.logList.FollowEnd = true
		ui.List(c.Key("log"), &a.logList, len(a.log), func(row int) { a.logRow(c, row) }).Grow(1)
		ui.Textf(c, "%d 条", len(a.log)).FontSize(12).TextColor(t.TextMuted)
	})
}

// logRow is one line of the log: the outcome in a tinted pill, and where the
// file went.
func (a *App) logRow(c *ui.Context, row int) {
	report := a.log[row]
	t := c.Theme()
	color := outcomeColor(t, report.Outcome)
	ui.Row(c).Padding(t.Space(1.5), t.Space(2)).Gap(t.Space(2)).AlignItems(ui.Center).Children(func() {
		ui.Badge(c, outcomeLabel(report.Outcome)).Background(color.Alpha(0.15)).TextColor(color)
		ui.Column(c).Grow(1).Gap(t.Space(0.5)).Children(func() {
			text := report.Source
			if report.Dest != "" {
				text = report.Source + " → " + report.Dest
			}
			ui.Text(c, text).FontSize(12).SingleLine().Selectable().Tooltip(text)
			if report.Err != "" {
				ui.Text(c, report.Err).FontSize(12).TextColor(t.Danger)
			}
		})
	})
}

// sectionTitle is what the title bar and the sidebar call a section.
func sectionTitle(section string) string {
	switch section {
	case sectionOptions:
		return "选项"
	case sectionLog:
		return "日志"
	default:
		return "转换队列"
	}
}

// statusIcon is the mark before a source that is not being converted now.
func statusIcon(kind int) *ui.SVG {
	switch kind {
	case kindDone:
		return iconDone
	case kindFailed, kindInvalid:
		return iconAlert
	case kindEmpty:
		return iconEmpty
	default:
		return iconWaiting
	}
}

// statusColor colors the mark and the label of a queued source.
func statusColor(t *ui.Theme, kind int) ui.Color {
	switch kind {
	case kindActive:
		return t.Accent
	case kindDone:
		return t.Success
	case kindFailed, kindInvalid:
		return t.Danger
	default:
		return t.TextMuted
	}
}

// runState names a run that ended, and colors it.
func runState(t *ui.Theme, result Result, fault bool) (string, ui.Color) {
	switch {
	case fault:
		return "运行失败", t.Danger
	case result.Canceled:
		return "已取消", t.Warning
	case result.Failed > 0:
		return fmt.Sprintf("完成 · %d 个失败", result.Failed), t.Danger
	case result.DryRun:
		return "试运行完成", t.TextMuted
	default:
		return "已完成", t.Success
	}
}

// progressValue is what the progress bar fills to: a negative value shows
// activity of unknown length, before the work to do is known.
func (a *App) progressValue() float64 {
	if a.planned <= 0 {
		return -1
	}
	return min(a.progress, 1)
}

// outcomeLabel is how each outcome of a run reads on screen.
func outcomeLabel(outcome string) string {
	switch outcome {
	case string(app.OutcomePlanned):
		return "将转换"
	case string(app.OutcomeConverted):
		return "已转换"
	case string(app.OutcomeSkipped):
		return "已跳过"
	case string(app.OutcomeFailed):
		return "失败"
	default:
		return outcome
	}
}

// outcomeColor colors an outcome's badge.
func outcomeColor(t *ui.Theme, outcome string) ui.Color {
	switch outcome {
	case string(app.OutcomeConverted):
		return t.Success
	case string(app.OutcomeSkipped):
		return t.Warning
	case string(app.OutcomeFailed):
		return t.Danger
	default:
		return t.TextMuted
	}
}

// summarize describes a finished run, beside the counts the card already shows.
func summarize(result Result) string {
	parts := []string{
		fmt.Sprintf("共 %d 个", result.Total),
		fmt.Sprintf("%.1fs", float64(result.DurationMs)/1000),
	}
	if result.DryRun {
		parts = append(parts, "试运行")
	}
	if result.Canceled {
		parts = append(parts, "已取消")
	}
	return strings.Join(parts, " · ")
}

// progressSummary describes a run in flight.
func progressSummary(done, planned int) string {
	return fmt.Sprintf("转换中… %d / %d", done, planned)
}
