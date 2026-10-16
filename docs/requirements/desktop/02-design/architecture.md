# 设计：桌面应用与双模式入口

> 关联：[../01-requirements/PRD.md](../01-requirements/PRD.md)｜[ui-design.md](ui-design.md)｜[../03-implementation/phased-plan.md](../03-implementation/phased-plan.md)
> 架构决策：[ADR-010](../../../planning/architecture.md)（单二进制双模式）、[ADR-011](../../../planning/architecture.md)（原生 UI）

## 1. 总体结构

桌面应用**不引入第二套转换逻辑**。它只是在既有编排外面加一层「谁能看到进度」的出口，并把一个用 Go 声明的原生界面接到这层出口上。

```
                ┌───────────────────────── cmd/ncmconverter ─────────────────────────┐
                │  main(): os.Args 有参数 → runCLI()（原有 urfave/cli 路径）           │
                │            无参数       → desktop.Run()（MyGo 应用）                │
                └───────────────┬──────────────────────────────┬───────────────────┘
                                │                              │
                     internal/app (编排，UI 无关)          internal/desktop (原生 UI)
                                │                              │
        ┌───────────────────────┼──────────────────────┐       │  mygo.NewWindow / NewMenu / Dialog
        ▼                       ▼                      ▼       ▼
   internal/ncm         internal/converter        internal/tag   runner.go（无窗口内核）
   （容器解析）           （解密与元数据）          （标签写入）    view.go（逐帧声明界面）
                                                                app.go（状态、装配、菜单）
```

依赖方向（延续 [ADR-002](../../../planning/architecture.md)）：

- `cmd/ncmconverter → {app, desktop}`
- `desktop → app`、`desktop → github.com/egoist/mygo`、`desktop → github.com/egoist/mygo/ui`
- `app → {ncm, converter, tag}`、`tag → converter`
- **`app` 不导入 mygo，也不导入 `desktop`**：`internal/app` 必须保持 UI 无关，桌面通过回调接入。

## 2. 目录布局（改动后）

```
cmd/ncmconverter/
  main.go                  # 双模式入口
  console_windows.go       # //go:build windows：attachConsole()
  console_other.go         # //go:build !windows：no-op
internal/
  app/                     # 编排（report.go：Outcome/FileResult、Options.OnFile、导出 Scan）
  desktop/                 # 原生 UI 桌面应用
    app.go                 # App 状态、窗口、菜单、对话框、运行编排
    view.go                # 视图与辅助函数（ui.* 逐帧声明）
    runner.go              # 无窗口的转换内核：Request/Report/Result、单飞、错误归一
    runner_test.go         # 内核测试（internal/ncmtest 夹具）
    view_test.go           # 视图测试（ui.NewTester，无需窗口）
  ncm/ converter/ tag/ ncmtest/   # 不变
mygo.json                  # MyGo 项目配置（纯数据，无 JS 运行时）
resources/icon.png         # 打包图标
dist/                      # mygo build 产物，每个目标一个子目录（不提交）
```

**为什么前端资产全部消失**：界面由 `package ui` 在 Go 里声明，没有 HTML/CSS/TS，也就没有 Vite 的 `root`、`frontendDist` 与生成客户端。`mygo.json` 取代了 `mygo.config.ts`——后者需要 JS 运行时（Bun/Node 22.6+）求值，而原生 UI 项目不该为此背一个 Node 依赖。

**为什么 MyGo 配置仍在仓库根**：`mygo.json` 的 `main` 是「相对项目根的 Go 包路径」，只有项目根在仓库根时它才是自然的 `./cmd/ncmconverter`；`out: "dist"` 与既有 `.gitignore` 的 `dist/` 对齐。

## 3. 双模式入口

```go
// cmd/ncmconverter/main.go（结构示意）
func main() {
	// 有参数 = 命令行工具；无参数 = 桌面应用。
	// MyGo 工具链会带 MYGO_GENERATE 跑同一份二进制（写完客户端立即返回），
	// 那种情况必须走桌面分支，否则参数判断会把生成流程当成 CLI。
	if len(os.Args) > 1 && os.Getenv("MYGO_GENERATE") == "" {
		attachConsole() // Windows：GUI 子系统构建没有自己的控制台
		runCLI()
		return
	}
	if err := desktop.Run(); err != nil {
		slog.Error("ncmconverter failed", "err", err)
		os.Exit(1)
	}
}
```

- `runCLI` **原样搬运**现有 `main` 主体：`signal.NotifyContext`、`cli.VersionPrinter`、`slog` handler（含 `--quiet` 的 `LevelVar`）、`cli.Command`、`app.Run(ctx, app.Options{...})`、失败 `os.Exit(1)`。**不改**任何参数名、默认值、日志文案与退出码。
- `version/commit/date` 三个变量与 `-ldflags -X` 注入方式不动，保证 `--version` 输出不变。
- `MYGO_GENERATE` 守卫保留：它是「工具链跑同一份二进制」的入口标记，与界面形态无关。

### 3.1 Windows 控制台垫片

`mygo build` 在 Windows 上会加 `-H=windowsgui`（`cmd/mygo/build.go`），产出**没有自己控制台**的 GUI 程序：从 Explorer 双击不弹黑框，但从终端调用时 stdout/stderr 不接父控制台，CLI 会「哑掉」。垫片在 CLI 分支前调用：

```go
//go:build windows

package main

// attachConsole attaches the process to the console it was launched from, so
// a windowsgui build still prints when run from a terminal. It does nothing
// when there is no console to attach to (a double click, a service).
func attachConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0) // -1, DWORD(-1)
	if ok, _, _ := attach.Call(attachParentProcess); ok == 0 {
		return
	}
	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = out, out
	}
	if in, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = in
	}
}
```

要点：

- 只用标准库 `syscall`（与 MyGo 一致地避开 cgo），**不新增依赖**。
- 必须在建立 `slog` handler **之前**调用，因为 handler 在创建时就捕获了 `os.Stderr`。
- Linux/macOS 无此问题，`console_other.go` 提供空实现。

## 4. `internal/app`：让第二个调用方接入（行为不变）

现状：`Run` 只把每文件结果写进 `slog`，桌面拿不到。改动是**附加式**的：

### 4.1 `internal/app/report.go`

```go
type Outcome string
const (
	OutcomePlanned   Outcome = "planned"   // 试运行：将转换
	OutcomeConverted Outcome = "converted" // 已写出
	OutcomeSkipped   Outcome = "skipped"   // 目标已存在
	OutcomeFailed    Outcome = "failed"    // 失败
)

type FileResult struct {
	Source  string
	Dest    string   // 未写盘时为空
	Outcome Outcome
	Err     error
}
```

### 4.2 `Options` 增加回调

```go
// OnFile, when set, is called once for every container the run handles...
// Calls are serialized.
OnFile func(FileResult)
```

### 4.3 导出遍历

`collect` → `Scan`（同一实现、同一行为，只改名 + 补文档注释）。导出它是因为界面需要在开跑前显示工作量，并把容器归属回各自的来源（见 §6）。

### 4.4 `convert` 返回 `FileResult`

原签名 `(skipped bool, err error)` 丢掉目标路径，界面需要 `Dest`。改为 `func convert(ctx, source string, opts Options) (FileResult, error)`；`Dest` 在算出 `outputPath` 后立即填入。

> 既有的 `slog` 调用**全部保留**，CLI 输出不变。`Run` 的签名与返回错误不变。

### 4.5 `Run` 里回调的时机

回调**在已持有的互斥锁内**调用 → 天然串行，调用方无需自己加锁（`-race` 下安全）。试运行分支逐文件回调 `OutcomePlanned`。

## 5. `internal/desktop`：原生应用

三个文件，职责清晰：

| 文件 | 内容 |
| --- | --- |
| `runner.go` | `Request`/`Report`/`Result`、`runner.convert`（单飞 `begin/finish` + 错误归一）、`runConvert`、`scan`、`scanBySource`、`reportOf`。**不引用 `*mygo.Window`**，因此可无窗口测试 |
| `app.go` | `Run()`（应用装配）、`queueEntry`、`App`（状态）、`openWindow`、`newMenu`、对话框、运行编排（`start`/`cancel`/`runRequest`/`record`/`finish`） |
| `view.go` | `view` 与各分区（`sidebar`/`pane`/`titleBar`/`queueBody`/`statusCard`/`queueRows`/`optionsBody`/`logBody`/`logRow`）、纯函数（`summarize`/`progressSummary`/`outcomeLabel`/`runState`）、内联 SVG 图标 |

### 5.1 应用装配

```go
func Run() error {
	a := newApp()
	mygo.App.SetName("NCMconverter")
	mygo.App.WhenReady(func() {
		a.build = buildInfo()
		mygo.App.SetMenu(newMenu(a))
		a.openWindow()          // Content: ui.View(a.view)
	})
	mygo.App.OnWindowAllClosed(func() { if runtime.GOOS != "darwin" { mygo.App.Quit() } })
	mygo.App.OnActivate(func(hasVisibleWindows bool) { if !hasVisibleWindows { a.openWindow() } })
	return mygo.App.Run()
}
```

窗口选项与材质见 [ui-design §2](ui-design.md#2-窗口与外壳)；菜单用 `RoleAppMenu/RoleEditMenu/RoleViewMenu/RoleWindowMenu` 加「文件」「转换」两个子菜单；对话框会阻塞，所以在 `go func()` 里调。

### 5.2 线程模型

视图跑在**主线程**，直接读写 `App` 状态；转换跑在**自己的 goroutine**，每次结果都经 `Window.Update` 回写：

```go
// start()（主线程）：置 running、清空本次计数、起 goroutine
go a.runRequest(ctx, request, paths)

// runRequest（后台）：
owners, sizes := scanBySource(paths, request.Depth)      // 归属 + 计划数
a.update(func() { a.owners, a.planned = owners, len(owners); /* 写回每个来源的总数 */ })
result, err := a.run.convert(ctx, request, func(r Report) {
	a.update(func() { a.record(r) })                     // 每个容器一次，串行
})
a.update(func() { a.finish(result, err) })
```

`OnFile` 在 `internal/app` 已持有的互斥锁内被调用，天然串行。对话框与 `mygo.Shell.OpenPath` 同理：在 goroutine 里做，完成后 `Window.Update`。

### 5.3 错误归一（决定界面体验）

```go
summary, err := ...            // 通过 OnFile 计数
if errors.Is(err, context.Canceled) { result.Canceled = true; return result, nil }
if result.Failed > 0 { return result, nil } // 每个失败都已在一条 Report 里，聚合错误对界面没有增量
return result, err                          // 输入不可读、threads<1 之类才是真错误
```

真错误由 `finish` 记进 `App.fault`，以 `Danger` 显示在状态卡内，并在标题栏给出「运行失败」。

### 5.4 文件拖放

两条路径都汇到同一个 `addPaths`：视图内的拖放区（`Element.DroppedFiles`，队列为空时显示）与窗口级 `Window.OnFileDrop`（队列非空时元素不接管，故落到窗口）。

## 6. 状态流转与来源归属

界面要能回答「每个来源跑到哪一步了」。为此 `scanBySource` **逐个来源**单独展开：

```go
func scanBySource(paths []string, depth int) (map[string]int, []sourceSize) {
	owners := make(map[string]int)   // 容器 → 来源下标
	...
	for i, path := range paths {
		found, err := app.Scan([]string{path}, depth)
		if err != nil { sizes[i].invalid = true; continue }  // 只标记，不清零整行
		for _, container := range found {
			if _, claimed := owners[container]; claimed { continue } // 先到先得，与 Scan 一致
			owners[container] = i
			sizes[i].total++
		}
	}
	return owners, sizes
}
```

- 与 `app.Scan(inputs, depth)` 的去重**顺序与语义一致**（按输入顺序、先到先得），因此一次运行里每条 `Report.Source` 唯一对应一个队列项。
- 用于两处：加入队列时的统计（`refreshStats`，异步、带 `scanToken` 防旧结果覆盖），与运行开始时算计划数与归属（`runRequest`）。
- 打不开的来源标为「路径不可用」，**不把整行统计清零**；运行本身仍报告该输入的错误。

阶段文案、标记与运行状态卡的细节见 [ui-design §4](ui-design.md#4-转换队列的状态流转)。

## 7. 构建契约（关键约束）

| | `go build`（`make build`） | `go tool mygo build`（`make desktop`） |
| --- | --- | --- |
| Windows 子系统 | 控制台 | `-H=windowsgui`（GUI） |
| 打包 | 裸二进制 | `.app`+dmg / exe+NSIS 安装器 / tar.gz+deb |
| 用途 | 快速编译门禁、CLI 路径 | 桌面开发与发布 |

**没有前端要内嵌**（界面是 Go 代码），因此二者的差别只在 GUI 子系统与打包，而不在资源。`make build` 保持为「快」的 Go 编译门禁，不引入任何 Node 依赖；桌面开发与打包一律走 `go tool mygo dev` / `go tool mygo build`。

## 8. 平台差异

| | macOS | Windows | Linux |
| --- | --- | --- | --- |
| 子系统 | 常规 | `windowsgui` + `AttachConsole` | 常规 |
| 桌面渲染 | 原生（系统框架） | 原生（Direct3D 11） | 原生（OpenGL） |
| 桌面运行时依赖 | 系统框架 | 无（**不需要 WebView2**） | **GTK 3**（不需要 WebKitGTK） |
| CLI 运行时依赖 | 无 | 无 | 无（MyGo 无 cgo，GTK 3 运行时按需加载） |
| 窗口材质 | 全部 `Vibrancy` | Windows 11 Mica/Acrylic | 无（侧栏退化为不透明背景） |
| 打包 | `.app` + dmg（签名/公证需 macOS） | exe + NSIS 安装器 | tar.gz（`install.sh`）+ deb |
| 关窗行为 | 留在 Dock，点图标重开 | 退出 | 退出 |

**headless 安全性**：桌面分支之外不触碰任何 MyGo API，GTK 3 不会被加载，因此服务器/CI/Docker 上直接用 CLI 是安全的。

## 9. 依赖与工具链

- `go.mod`：`github.com/egoist/mygo v0.5.5`（纯 Go、无 cgo；连带 `go-text/typesetting`、`x/image` 等），**并新增 tool 指令** `tool github.com/egoist/mygo/cmd/mygo`——MyGo CLI 以模块工具引入，用 `go tool mygo` 调用，版本随 `go.mod` 锁定。
- **不再有任何 Node/bun 依赖**：没有 `package.json`、没有 Vite、没有生成客户端。
- `Makefile`：`dev`（`go tool mygo dev`）与 `desktop`（`go tool mygo build`）两个目标；Go 门禁不依赖二者。

## 10. 与既有规范的关系

- **ADR-002（单向管线、`internal/` 私有）**：不破坏。`desktop` 是新的一层，`app` 依旧不认识 UI。
- **ADR-007（并发与失败隔离）**：不破坏。回调挂在既有的失败聚合点上。
- **ADR-008（GoReleaser）**：被 ADR-010 取代（发布改由 `mygo build` 驱动）。
- **ADR-009（门禁走 make）**：Go 门禁仍不依赖 Node；桌面目标走 `go tool mygo`。
- **[DD-3](../05-review/design-decisions.md)（界面用 web 前端）**：被 [ADR-011](../../../planning/architecture.md) 取代（原生 UI），理由是观感与 UX，以及不再需要 WebView 运行时。
