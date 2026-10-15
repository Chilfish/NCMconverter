# 设计：桌面应用与双模式入口

> 关联：[../01-requirements/PRD.md](../01-requirements/PRD.md)｜[api-contract.md](api-contract.md)｜[../03-implementation/phased-plan.md](../03-implementation/phased-plan.md)
> 架构决策：[ADR-010](../../../planning/architecture.md)

## 1. 总体结构

桌面应用**不引入第二套转换逻辑**。它只是在既有编排外面加一层「谁能看到进度」的出口，并把界面接到这层出口上。

```
                ┌───────────────────────── cmd/ncmconverter ─────────────────────────┐
                │  main(): os.Args 有参数 → runCLI()（原有 urfave/cli 路径）           │
                │            无参数       → desktop.Run()（MyGo 应用）                │
                └───────────────┬──────────────────────────────┬───────────────────┘
                                │                              │
                     internal/app (编排，UI 无关)          internal/desktop (bound service)
                                │                              │
        ┌───────────────────────┼──────────────────────┐       │  mygo.Bind / NewEvent / Dialog
        ▼                       ▼                      ▼       ▼
   internal/ncm         internal/converter        internal/tag   desktop/ 前端 (TS)
   （容器解析）           （解密与元数据）          （标签写入）     └─ mygo generate → desktop/src/mygo.ts
```

依赖方向（延续 [ADR-002](../../../planning/architecture.md)）：

- `cmd/ncmconverter → {app, desktop}`
- `desktop → app`、`desktop → github.com/egoist/mygo`
- `app → {ncm, converter, tag}`、`tag → converter`
- **`app` 不导入 mygo，也不导入 `desktop`**：`internal/app` 必须保持 UI 无关，桌面通过回调接入。

## 2. 目录布局（改动后）

```
cmd/ncmconverter/
  main.go                  # 双模式入口
  console_windows.go       # //go:build windows：attachConsole()
  console_other.go         # //go:build !windows：no-op
internal/
  app/                     # + report.go（Outcome/FileResult）、Options.OnFile、导出 Scan
  desktop/                 # 新增：bound service + MyGo 应用装配
    service.go
    app.go
    service_test.go
  ncm/ converter/ tag/     # 不变
  ncmtest/                 # 不变（测试夹具）
desktop/                   # 新增：web 前端（Vite 根目录）
  index.html
  src/main.ts  src/style.css  src/mygo.ts（生成，提交）
mygo.config.ts             # 新增：MyGo 项目配置（仓库根）
package.json  tsconfig.json  vite.config.ts   # 新增：前端工具链（仓库根）
```

**为什么前端在 `desktop/` 而不是仓库根**：根目录已经有 `cmd/ internal/ docs/` 等语义明确的目录，把 `index.html`/`src/` 摊在根上会污染；Vite 的 `root` 指到 `desktop/` 即可，`frontendDist` 随之是 `desktop/dist`。

**为什么 MyGo 配置在仓库根**：`mygo.config.ts` 里 `main` 是「相对项目的 Go 包」，只有项目根在仓库根时它才是自然的 `./cmd/ncmconverter`；`mygo build` 的输出目录 `out: "dist"` 与既有 `.gitignore` 的 `dist/` 对齐。

## 3. 双模式入口

```go
// cmd/ncmconverter/main.go（结构示意）
func main() {
	// 有参数 = 命令行工具；无参数 = 桌面应用。
	// `mygo generate` 会设置 MYGO_GENERATE 跑同一份二进制（它写客户端后立即返回），
	// 那种情况必须走桌面分支，否则参数判断会把生成流程当成 CLI。
	if len(os.Args) > 1 && os.Getenv("MYGO_GENERATE") == "" {
		attachConsole() // Windows：GUI 子系统构建没有自己的控制台
		runCLI(os.Args)
		return
	}
	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}
```

- `runCLI` **原样搬运**现有 `main` 主体：`signal.NotifyContext`、`cli.VersionPrinter`、`slog` handler（含 `--quiet` 的 `LevelVar`）、`cli.Command`、`app.Run(ctx, app.Options{...})`、失败 `os.Exit(1)`。**不改**任何参数名、默认值、日志文案与退出码。
- `version/commit/date` 三个变量与 `-ldflags -X` 注入方式不动，保证 `--version` 输出不变。

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

### 4.1 新增 `internal/app/report.go`

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

`collect` → `Scan`（同一实现、同一行为，只改名 + 补文档注释）。`app_test.go` 两处调用点同步改名。导出它是因为界面需要在开跑前显示工作量。

### 4.4 `convert` 返回 `FileResult`

原签名 `(skipped bool, err error)` 丢掉目标路径，界面需要 `Dest`。改为：

```go
func convert(ctx context.Context, source string, opts Options) (FileResult, error)
```

`Dest` 在算出 `outputPath` 后立即填入；`Outcome` 在「跳过」与「写出成功」两个分支分别填 `OutcomeSkipped`/`OutcomeConverted`；失败时由 `Run` 覆盖为 `OutcomeFailed` 并带上 `Err`。

> 既有的 `slog` 调用（`converted container`、`skipping a file that already exists`、`conversion failed`、`converted files`）**全部保留**，CLI 输出不变。

### 4.5 `Run` 里回调的时机

```go
mu.Lock()
defer mu.Unlock()
switch {
case err != nil:
	slog.Error("conversion failed", "file", source, "err", err)
	result.Outcome, result.Err = OutcomeFailed, err
	failures = append(failures, fmt.Errorf("%s: %w", source, err))
case result.Outcome == OutcomeSkipped:
	skipped++
}
if opts.OnFile != nil {
	opts.OnFile(result)
}
```

- 回调**在已持有的互斥锁内**调用 → 天然串行，避免调用方为竞态发愁（`-race` 下安全）。
- 试运行分支逐文件回调 `OutcomePlanned`。
- **`Run` 的签名与返回错误不变**（`error`），避免影响 CLI 与既有测试；计数由调用方按回调自行累加。

## 5. `internal/desktop`

### 5.1 `service.go`：绑定给页面的服务

类型 `Converter`（绑定后前端叫 `Converter`）。**全部方法用指针接收者**：值接收者会复制内含的 `sync.Mutex`，`go vet` 会报 `passes lock by value`。

| 方法 | 作用 |
| --- | --- |
| `Info() Info` | 名称、版本（回退 `mygo.App.Version()`）、mygo 版本、平台/架构 |
| `PickFiles(ctx) ([]string, error)` | 系统对话框选多个 `.ncm` |
| `PickDirectory(ctx) (string, error)` | 选一个目录，取消返回 `""` |
| `PickOutput(ctx) (string, error)` | 选输出目录 |
| `Scan(inputs, depth) ([]string, error)` | 转调 `app.Scan`，供界面预览 |
| `Run(ctx, request, reports *mygo.Channel[Report]) (Result, error)` | 转换并逐文件推送到通道 |
| `Cancel()` | 停止当前运行 |
| `OpenPath(path) error` | `mygo.Shell.OpenPath`，用于「打开输出目录」 |

单飞与取消：

```go
func (c *Converter) begin(ctx context.Context) (context.Context, context.CancelFunc, error)
func (c *Converter) finish(cancel context.CancelFunc)
```

`begin` 在已运行时返回错误；`cancel` 供 `Cancel()` 使用。

`Run` 的错误归一（关键，决定界面体验）：

```go
summary, err := ...            // 通过 Options.OnFile 计数
if errors.Is(err, context.Canceled) { result.Canceled = true; return result, nil }
if result.Failed > 0 { return result, nil } // 每个失败都已在一条 Report 里，聚合错误对界面没有增量
return result, err                          // 输入不可读、threads<1 之类才是真错误
```

### 5.2 事件

```go
var Opened = mygo.NewEvent[[]string]("files:opened")
```

菜单里选完文件后广播，页面合并进队列。页面按钮走 `PickFiles` 的返回值，两条路径最终汇到页面同一个 `addInputs`。

### 5.3 `app.go`：应用装配

```go
func Run() error {
	mygo.App.SetName("NCMconverter")
	mygo.Bind(&Converter{})
	mygo.App.WhenReady(func() { mygo.App.SetMenu(newMenu()); openWindow() })
	mygo.App.OnWindowAllClosed(func() {})                 // macOS：关窗不退出
	mygo.App.OnActivate(func(visible bool) { if !visible { openWindow() } })
	return mygo.App.Run()
}

func openWindow() {
	mygo.NewWindow(mygo.WindowOptions{
		Title: "NCMconverter", Width: 860, Height: 760,
		MinWidth: 620, MinHeight: 540,
		BackgroundColor: "light-dark(#f6f7f9, #16181d)", // 与 src/style.css 的 --bg 一致
		StateKey: "main",                                // 记住窗口位置
		URL: "/",
	})
}
```

菜单用 `RoleAppMenu/RoleEditMenu/RoleViewMenu/RoleWindowMenu` 加一个「文件」子菜单；对话框会阻塞，所以在 `go func()` 里调。

> 参考实现：工作区 `I:\dev\mygo-app`（已在 Windows 上跑通窗口、拖拽、进度、取消）——可直接对照搬运前端与文案。

## 6. 前端与绑定

根配置（`mygo.config.ts`）：

```ts
export default defineConfig({
  name: "NCMconverter",
  identifier: "dev.chilfish.ncmconverter",
  version: "0.1.0",
  main: "./cmd/ncmconverter",
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "desktop/dist",
  bindings: "desktop/src/mygo.ts",
  out: "dist",
});
```

要点：

- `bindings` 指到 `desktop/src/mygo.ts`，由 `mygo generate` 生成（**提交进仓库**，CI 校验同步）。
- `vite.config.ts` 用 `root: "desktop"`，`build.outDir` 默认落在 `desktop/dist`。
- `tsconfig.json` 的 `include` 指向 `desktop/src`。
- 页面用 `mygo-runtime` 的 `Channel`（逐文件进度）、`onFileDrop`（拖拽入队），并从生成的客户端订阅 `events.filesOpened`。

前端调用的确切表面见 [api-contract.md](api-contract.md)。

## 7. 构建契约（关键约束）

| | `go build`（`make build`） | `mygo build`（`bun run build`） |
| --- | --- | --- |
| 前端 | **不嵌入** | 用 `-overlay` 把 `frontendDist` 注入主包（生成 `mygo_frontend_gen.go` + `mygo_frontend/`） |
| Windows 子系统 | 控制台 | `-H=windowsgui`（GUI） |
| 产物 | 裸二进制 | `.app`+dmg / exe+NSIS 安装器 / tar.gz+deb |
| 用途 | 快速编译门禁、CLI | 桌面开发与发布 |

因此：

- **发布产物必须由 `mygo build` 产出**（否则桌面打不开页面、Windows 还有黑框）。
- `make build` 保持为「快」的 Go 编译门禁，**不引入 bun**；其二进制桌面模式没有内嵌前端，文档里讲清。
- `mygo dev` 用 devUrl 加载页面，不需要嵌入，是日常开发路径。

## 8. 平台差异

| | macOS | Windows | Linux |
| --- | --- | --- | --- |
| 子系统 | 常规 | `windowsgui` + `AttachConsole` | 常规 |
| 桌面运行时依赖 | 系统 WebKit | WebView2 Runtime | **GTK + WebKitGTK**（仅桌面模式） |
| CLI 运行时依赖 | 无 | 无 | 无（MyGo 无 cgo，GTK/WebKit 是运行时按需 dlopen） |
| 打包 | `.app` + dmg（签名/公证需 macOS） | exe + NSIS 安装器 | tar.gz（`install.sh`）+ deb |
| 关窗行为 | 留在 Dock，点图标重开 | 退出 | 退出 |

**headless 安全性**：桌面模式之外不触碰任何 MyGo API，GTK/WebKitGTK 不会被加载，因此服务器/CI/Docker 上直接用 CLI 是安全的。

## 9. 依赖变化

- `go.mod`：新增 `github.com/egoist/mygo v0.5.1`（连带 `purego`、`x/image`、`x/net`、`go-text/typesetting` 等），`go` 指令被 `go mod tidy` 抬到 `1.27.1`。
- 根 `package.json`：`mygo-runtime`（运行时）、`mygo-cli`/`typescript`/`vite`（开发）。版本与 MyGo 官方模板对齐。

## 10. 与既有规范的关系

- **ADR-002（单向管线、`internal/` 私有）**：不破坏。`desktop` 是新的一层，`app` 依旧不认识 UI。
- **ADR-007（并发与失败隔离）**：不破坏。回调挂在既有的失败聚合点上。
- **ADR-008（GoReleaser）**：**被 ADR-010 取代**（发布改由 `mygo build` 驱动）。
- **ADR-009（门禁走 make）**：新增 `make desktop/generate/typecheck`，Go 门禁仍不依赖 bun。
