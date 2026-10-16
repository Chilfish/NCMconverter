# 桌面应用与双模式入口

> 子系统：`cmd/ncmconverter` + `internal/desktop`｜ 关键文件：[`cmd/ncmconverter/main.go`](../../../cmd/ncmconverter/main.go)、[`internal/desktop/app.go`](../../../internal/desktop/app.go)、[`internal/desktop/view.go`](../../../internal/desktop/view.go)、[`internal/desktop/runner.go`](../../../internal/desktop/runner.go)
> 需求、设计与测试方案：[`requirements/desktop/`](../../requirements/desktop/)（PRD / 用户故事 / 设计 / UI 设计 / API 契约 / 测试计划）；架构决策见 [ADR-010](../../planning/architecture.md)、[ADR-011](../../planning/architecture.md)。

`ncmconverter` 是**一个可执行文件、两种形态**：无参数启动打开桌面窗口，带参数启动就是原来的命令行工具。两者共用 [`internal/app`](../cli/cli-and-orchestration.md) 的编排，桌面通过 `Options.OnFile` 回调获取逐文件进度——**不存在第二套转换逻辑**，CLI 的参数、默认值、日志与退出码一字未变。

窗口是 MyGo 的**原生 UI**（`github.com/egoist/mygo/ui`）：界面由 Go 声明、MyGo 直接在 GPU 上绘制。**没有网页、没有 webview**，也没有绑定层与生成的客户端（`mygo.Bind`/`mygo.Channel`/`mygo.NewEvent` 与 TypeScript 客户端都已不存在）——这是 [ADR-011](../../planning/architecture.md) 对 [DD-3](../../requirements/desktop/05-review/design-decisions.md) 的取代。

## 1. 目录布局

```
cmd/ncmconverter/        # 双模式入口：runCLI() 或 desktop.Run()；console_{windows,other}.go
internal/app/            # 编排（不变：Options.OnFile、Scan、convert）
internal/desktop/
  app.go                 # App 状态、窗口、菜单、对话框、运行编排
  view.go                # 原生 UI 视图与辅助函数
  runner.go              # 无窗口的转换内核（Request/Report/Result、单飞、错误归一）
  runner_test.go         # 转换内核测试（internal/ncmtest 夹具）
  view_test.go           # 视图测试（ui.NewTester，无需窗口）
mygo.json                # MyGo 配置（纯数据，无需 JS 运行时）
resources/icon.png       # 打包图标
dist/                    # mygo build 产物（不提交）
```

相比上一版，`desktop/`（`index.html`、`src/main.ts`、`src/style.css`、`src/mygo.ts`）、仓库根的 `package.json`、`tsconfig.json`、`vite.config.ts`、`bun.lock`、`mygo.config.ts` 与旧的绑定服务 `internal/desktop/service.go` 都已删除。`mygo.json` 仍在仓库根，因为它的 `main` 是相对项目根的 Go 包路径（`./cmd/ncmconverter`），且 `out: "dist"` 与既有 `.gitignore` 的 `dist/` 对齐。

## 2. 双模式入口

[`cmd/ncmconverter/main.go`](../../../cmd/ncmconverter/main.go) 的 `main()` 只做一件事：看 `os.Args` 决定走哪条路径。

```go
func main() {
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

- **有参数即 CLI**：`os.Args` 长度大于 1 时进入 `runCLI()`，它是改动前 `main` 主体的原样搬运（`signal.NotifyContext`、`cli.VersionPrinter`、`slog` handler、`cli.Command`、`app.Run`、失败 `os.Exit(1)`），参数名、默认值、日志文案与退出码都不变。
- **`MYGO_GENERATE` 例外**：MyGo 工具链会带着 `MYGO_GENERATE` 跑同一份二进制并立即返回；那种情况必须走桌面分支，否则「有参数」的判断会把它误当成 CLI 调用。
- **无参数即桌面**：其余情况调 `desktop.Run()`，失败时记一条错误并以非零码退出。

### 2.1 Windows 控制台垫片

`mygo build` 在 Windows 上给二进制加 `-H=windowsgui`，产出**没有自己控制台**的 GUI 程序：从资源管理器双击不弹黑框，但从终端调用时 stdout/stderr 不接父控制台，CLI 会「哑掉」。垫片在 `runCLI()` **之前**调用：

```go
//go:build windows

func attachConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	attach := kernel32.NewProc("AttachConsole")
	const attachParentProcess = ^uintptr(0) // DWORD(-1)
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

- 只用标准库 `syscall`，**不引入 cgo 或新依赖**。
- 必须在建立 `slog` handler **之前**调用，因为 handler 在创建时就捕获了 `os.Stderr`。
- `AttachConsole` 失败（双击、服务）时静默返回。Linux/macOS 的二进制保留启动时的控制台，[`console_other.go`](../../../cmd/ncmconverter/console_other.go) 提供 `//go:build !windows` 的空实现。

## 3. `internal/desktop`：原生应用

`internal/desktop` 就是一个普通的 Go 应用，只是把窗口交给 MyGo 绘制——没有页面、没有绑定服务，也没有生成的客户端：

| 文件 | 作用 |
| --- | --- |
| [`app.go`](../../../internal/desktop/app.go) | `App` 状态、窗口、菜单、对话框与运行编排 |
| [`view.go`](../../../internal/desktop/view.go) | 原生 UI 视图与辅助函数（`Content: ui.View(a.view)` 每帧重绘） |
| [`runner.go`](../../../internal/desktop/runner.go) | **无窗口**的转换内核：`Request`/`Report`/`Result`、单飞、错误归一 |
| [`runner_test.go`](../../../internal/desktop/runner_test.go) | 转换内核测试（`internal/ncmtest` 夹具） |
| [`view_test.go`](../../../internal/desktop/view_test.go) | 视图测试（`ui.NewTester`，无需窗口） |

**窗口**：`mygo.NewWindow` 的参数如下，窗口自带的标题栏被隐藏，交由视图在 `TitleBarHeight`（52）里自绘（见 §4）；`VibrancySidebar` 的材质透出侧栏。

| 选项 | 值 |
| --- | --- |
| `Title` | `NCMconverter` |
| `Width` / `Height` | 900 × 640 |
| `MinWidth` / `MinHeight` | 680 × 460 |
| `StateKey` | `main`（记住窗口尺寸与位置） |
| `TitleBarStyle` / `TitleBarHeight` | `mygo.TitleBarHiddenInset` / 52 |
| `Vibrancy` | `mygo.VibrancySidebar` |
| `Content` | `ui.View(a.view)` |

**拖到窗口任意位置**的文件由 `Window.OnFileDrop` 接收并入队。

**App 状态**：`App` 持有队列（`[]*queueEntry`）、各项选项、当前/最近一次运行（进度、计数、汇总、错误、日志）与所选分区。转换在内核的 goroutine 上跑，跨线程一律经 `Window.Update` 回主线程；视图只在主线程读写状态。

**菜单与对话框**：应用菜单 / 文件（添加文件… `CmdOrCtrl+O`、添加文件夹… `CmdOrCtrl+Shift+O`、Close）/ 转换（开始转换 `CmdOrCtrl+Return`、取消、打开输出目录）/ 编辑 / 视图 / 窗口；macOS 关闭全部窗口后留在 Dock，点图标重开。对话框用 `mygo.Dialog.Open` 选文件 / 文件夹 / 输出目录，会阻塞，所以在 `go func()` 里调，选完再 `Window.Update`；动作出错时用 `ui.Context.Toast` 提示。

**转换内核**：[`runner.go`](../../../internal/desktop/runner.go) 的 `runner` 不持有窗口，因此可被测试直接驱动。两条关键语义与旧版一致：

- **单飞与取消**：`runner.convert` 的 `begin`/`finish` 成对控制 `running`；运行中再次 `Run` 返回错误，`Cancel()` 让运行以 `Result.Canceled = true` 正常返回。
- **错误归一**：逐文件失败**不算调用错误**（每条 `Report` 已带原因，计进 `Result.Failed`）；取消也不算错；只有输入不可读、`threads < 1` 之类才返回错误。

**来源归属（状态流）**：每次运行先把每个来源**单独**展开（`scanBySource`），镜像 `app.Scan` 的「先到先得」去重，从而把每个容器归属到它的来源，并给进度条一个分母。逐条 `Report` 回到主线程后，除追加日志与计数外，还累加到对应来源的阶段计数上——这正是 §4 逐来源状态的来源。

## 4. 原生界面

视图（[`view.go`](../../../internal/desktop/view.go)）每帧从状态重绘，版面是**半透明侧栏 + 不透明内容区**：

- **侧栏**：来自窗口的 `VibrancySidebar` 材质；平台没有材质（Linux）时侧栏改画 `Surface` 底色。三个分区各带一个图标，队列与日志另带计数徽标——**转换队列**、**选项**、**日志**。
- **标题栏**：内容区自绘的内嵌标题栏（用 `Element.DragWindow` 拖动，经 `Context.TitleBar` 避开窗口控件），放着分区名、运行状态与动作（运行中显示**取消**，以及主按钮**开始转换**）。标题栏**在每个分区都显示运行状态**：运行中是 spinner + `d / t`，结束后是 已完成 / 完成 · N 个失败 / 已取消 / 试运行完成 / 运行失败。
- **队列面板**：动作行（添加文件… / 添加文件夹… / 清空）、运行状态卡片、来源列表与统计行。
- **选项面板**：原生 `ui.Form` 放进 `ui.Fieldset`，字段为 输出目录 / 命名模板 / 目录层级 depth / 并发数 threads / 写入标签与封面 / 跳过已存在的文件 / 试运行；运行中整体禁用。
- **日志面板**：`ui.List` 跟着末尾滚动，每条带结果徽标与「来源 → 目标」。
- **主题**：用 `Context.SetTheme` 把 `Radius` 调成 8，其余跟随桌面（外观、强调色、字号、对比度、减少动态效果）。
- 动作的错误以 `ui.Context.Toast` 弹出。

**逐来源状态**：队列里每个来源显示它此刻的阶段，前面带一个标记——**环**（等待）、**旋转**（转换中）、**对勾**（完成）、**感叹**（失败或不可读）：

| 文案 | 含义 |
| --- | --- |
| `待处理` | 尚未展开 |
| `待处理 · N 个` | 已展开，等待开跑 |
| `转换中 · d/t` | 正在转换该来源的第 d/t 个 |
| `N 已转换 · M 跳过 · K 失败` | 该来源已跑完（有失败时标记为感叹） |
| `N 个将转换` | 试运行结果 |
| `无 .ncm` | 没有可转换的容器 |
| `路径不可用` | 路径不可读 |

**运行状态卡片**：一旦开跑就显示、结束后保留，包含进度条、着色的计数药丸 **已转换 / 已跳过 / 失败**（为零时安静着色，非零时分别用 Success/Warning/Danger 着色）、汇总（`共 N 个 · 1.2s`，另有 试运行 / 已取消）、**打开输出目录** 按钮，以及运行级错误（Danger 文案）。

## 5. 构建与开发工作流

MyGo CLI 通过 [`go.mod`](../../../go.mod) 的 **tool 指令**（`tool github.com/egoist/mygo/cmd/mygo`）引入，因此以 `go tool mygo` 运行，**不需要 Node 或 bun**。[`mygo.json`](../../../mygo.json) 是纯数据配置（`name`/`identifier`/`version`/`main`/`out`），读取它不需要 JS 运行时（不像旧版需要求值的 `mygo.config.ts`）。

| 命令 | 实际执行 | 用途 |
| --- | --- | --- |
| `make dev` | `go tool mygo dev` | 日常开发，改 Go 代码自动重启 |
| `make desktop` | `go tool mygo build` | 打包当前平台的桌面产物 |

[`Makefile`](../../../Makefile) 里旧的 `generate` / `typecheck` 目标已删除——不再有绑定层或生成的客户端需要同步。

`mygo build` 会把 `name`/`identifier`/`version` 嵌进产物，在 Windows 上设置 `-H=windowsgui`，并把 macOS `.app`+dmg、Windows exe + NSIS 安装器、Linux tar.gz（含 `install.sh`）+ deb 打进 `dist/<os-arch>/`。

### 5.1 `make build` 与打包产物的差别

`make build` 是普通 `go build`，作为**快速编译门禁**与 CLI 路径保留；它**不带 GUI 子系统、也不打包安装器**，因此桌面开发与打包一律走 `go tool mygo dev` / `go tool mygo build`：

| | `go build`（`make build`） | `mygo build`（`make desktop`） |
| --- | --- | --- |
| Windows 子系统 | 控制台 | `-H=windowsgui`（GUI） |
| 打包 | `bin/ncmconverter`（裸二进制） | `dist/<平台>/` 下的 `.app`+dmg / exe+NSIS 安装器 / tar.gz+deb |
| 用途 | 快速编译门禁、CLI 路径 | 桌面开发与发布 |

`make build` 保持为「快」的编译门禁，它的二进制用于 CLI（`bin/ncmconverter --help` 等）；桌面开发与打包一律走 `mygo dev` / `mygo build`。发布链路与产物清单见 [发布产物](../release/release-artifacts.md)。

## 6. 平台差异与运行时依赖

| | macOS | Windows | Linux |
| --- | --- | --- | --- |
| 子系统 | 常规 | `windowsgui` + `AttachConsole` | 常规 |
| 桌面运行时依赖 | 系统框架（原生绘制） | 无（**不需要 WebView2 Runtime**） | **仅 GTK 3**（不需要 WebKitGTK） |
| CLI 运行时依赖 | 无 | 无 | 无（MyGo 无 cgo，GTK 是运行时按需加载） |
| 打包 | `.app` + dmg（签名/公证需 macOS） | exe + NSIS 安装器 | tar.gz（含 `install.sh`）+ deb |
| 关窗行为 | 留在 Dock，点图标重开 | 退出 | 退出 |

**headless 安全性**：桌面分支之外不触碰任何 MyGo API，GTK 不会在 CLI 模式下被加载，因此在服务器/CI/Docker 上直接用 CLI 是安全的。Linux 的 deb 会声明 GTK 3 依赖，CLI-only 用户会被牵连安装——这是单二进制的固有代价，记入 [backlog](../../planning/backlog.md)。

## 关键文件

- [`cmd/ncmconverter/main.go`](../../../cmd/ncmconverter/main.go) — 双模式分支、`runCLI`、版本注入变量
- [`cmd/ncmconverter/console_windows.go`](../../../cmd/ncmconverter/console_windows.go) / [`console_other.go`](../../../cmd/ncmconverter/console_other.go) — `attachConsole`
- [`internal/desktop/app.go`](../../../internal/desktop/app.go) — `App` 状态、窗口、菜单、对话框、运行编排
- [`internal/desktop/view.go`](../../../internal/desktop/view.go) — 原生 UI 视图与辅助函数
- [`internal/desktop/runner.go`](../../../internal/desktop/runner.go) — 无窗口的转换内核（`Request`/`Report`/`Result`、单飞、错误归一）
- [`mygo.json`](../../../mygo.json) / [`Makefile`](../../../Makefile) — 构建配置

## 相关测试 / 文档

- 测试：`internal/desktop/runner_test.go`（转换内核，无窗口，`internal/ncmtest` 夹具）、`internal/desktop/view_test.go`（视图测试，`ui.NewTester`，无窗口）、`internal/app/report_test.go`（回调语义）
- 需求与设计：[`requirements/desktop/`](../../requirements/desktop/)；UI 设计见 [`requirements/desktop/02-design/ui-design.md`](../../requirements/desktop/02-design/ui-design.md)；决策：[ADR-010](../../planning/architecture.md)、[ADR-011](../../planning/architecture.md)
- 发布：[`features/release/release-artifacts.md`](../release/release-artifacts.md)；CLI 语义：[`features/cli/cli-and-orchestration.md`](../cli/cli-and-orchestration.md)
