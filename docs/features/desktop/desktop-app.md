# 桌面应用与双模式入口

> 子系统：`cmd/ncmconverter` + `internal/desktop` + `desktop/`｜ 关键文件：[`cmd/ncmconverter/main.go`](../../../cmd/ncmconverter/main.go)、[`internal/desktop/service.go`](../../../internal/desktop/service.go)、[`internal/desktop/app.go`](../../../internal/desktop/app.go)、[`desktop/src/main.ts`](../../../desktop/src/main.ts)
> 需求、设计与测试方案：[`requirements/desktop/`](../../requirements/desktop/)（PRD / 用户故事 / 设计 / API 契约 / 测试计划）；架构决策见 [ADR-010](../../planning/architecture.md)。

`ncmconverter` 是**一个可执行文件、两种形态**：无参数启动打开桌面窗口，带参数启动就是原来的命令行工具。两者共用 [`internal/app`](../cli/cli-and-orchestration.md) 的编排，桌面通过 `Options.OnFile` 回调获取逐文件进度——**不存在第二套转换逻辑**，CLI 的参数、默认值、日志与退出码一字未变。

## 1. 目录布局

```
cmd/ncmconverter/
  main.go                  # 双模式入口：分支 → runCLI() 或 desktop.Run()
  console_windows.go       # //go:build windows：attachConsole()
  console_other.go         # //go:build !windows：空实现
internal/
  app/                     # 编排（+ report.go；Options.OnFile；导出 Scan）
  desktop/                 # 新增：绑定服务与 MyGo 装配
    service.go             # Converter、DTO、Opened 事件、对话框助手
    app.go                 # Run、窗口、菜单
    service_test.go
  ncm/ converter/ tag/ ncmtest/   # 不变
desktop/                   # 新增：web 前端（Vite 的 root）
  index.html               # 队列 / 选项 / 转换 / 日志 / 页脚版本
  src/main.ts              # Channel<Report> 收进度、拖拽、事件订阅
  src/style.css            # 亮暗模式、面板/日志/进度条
  src/mygo.ts              # mygo generate 生成，提交进仓库
  dist/                    # vite build 产物（frontendDist；不提交）
mygo.config.ts             # MyGo 项目配置（仓库根）
package.json               # 前端脚本与依赖（仓库根）
tsconfig.json              # include: ["desktop/src"]（仓库根）
vite.config.ts             # root: "desktop"（仓库根）
resources/icon.png         # 打包图标
dist/                      # mygo build 产物，每个目标一个子目录（不提交）
```

前端放在 `desktop/` 而不是仓库根：根目录已有 `cmd/ internal/ docs/` 等语义明确的目录，Vite 的 `root` 指到 `desktop/` 即可，`frontendDist` 随之是 `desktop/dist`。MyGo 配置放在仓库根，因为 `mygo.config.ts` 的 `main` 是相对项目根的 Go 包路径（`./cmd/ncmconverter`），且 `out: "dist"` 与既有 `.gitignore` 的 `dist/` 对齐。

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
- **`MYGO_GENERATE` 例外**：`mygo generate` 会带着 `MYGO_GENERATE` 跑同一份二进制来生成 TypeScript 客户端并立即返回。那种情况必须走桌面分支，否则「有参数」的判断会把生成流程误当成 CLI 调用。
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

## 3. 绑定给页面的服务

[`internal/desktop/service.go`](../../../internal/desktop/service.go) 里的 `Converter` 是页面与转换之间的**唯一桥梁**（`mygo.Bind(&Converter{})` 之后前端就叫 `Converter`）。全部方法用**指针接收者**：结构体含 `sync.Mutex`，值接收者会复制锁，`go vet` 报 `passes lock by value`。

| 方法 | 作用 |
| --- | --- |
| `Info() Info` | 名称、版本（回退 `mygo.App.Version()`，再回退 `dev`）、MyGo 版本、平台/架构 |
| `PickFiles(ctx) ([]string, error)` | 系统对话框选多个 `.ncm`，取消返回 `[]` |
| `PickDirectory(ctx) (string, error)` | 选一个目录，取消返回 `""` |
| `PickOutput(ctx) (string, error)` | 选输出目录 |
| `Scan(inputs, depth) ([]string, error)` | 转调 `app.Scan`，供界面预览工作量 |
| `Run(ctx, request, reports *mygo.Channel[Report]) (Result, error)` | 转换并逐文件推送进度 |
| `Cancel()` | 停止当前运行 |
| `OpenPath(path) error` | `mygo.Shell.OpenPath`，「打开输出目录」用 |

DTO（`Info`/`Request`/`Report`/`Result`）字段都带 `json` tag，前端类型由它生成。`Request` 与 `app.Options` 一一对应；`Report` 是单个容器的处理结果（`outcome` 为 `planned|converted|skipped|failed`）。

两条关键语义（详见 [API 契约](../../requirements/desktop/02-design/api-contract.md)）：

- **单飞与取消**：`begin`/`finish` 成对控制 `running` 与 `cancel`；已有运行时再次 `Run` 返回错误，`Cancel()` 让 `run` 以 `canceled: true` 正常返回。
- **错误归一**：逐文件失败**不算调用错误**（每条 `Report` 已带原因，计进 `Result.Failed`）；取消也不算错（`Result.Canceled = true`）；只有输入不可读、`threads < 1` 之类才返回错误。

菜单里选完文件/文件夹后广播 `Opened` 事件（`mygo.NewEvent[[]string]("files:opened")`），页面合并进队列；页面自己的按钮直接用 `PickFiles`/`PickDirectory` 的返回值，两条路径最终汇到页面同一个 `addInputs`。对话框会阻塞，所以在 `go func()` 里调。

[`internal/desktop/app.go`](../../../internal/desktop/app.go) 负责装配：`SetName` → `Bind` → `WhenReady`（`SetMenu` + `openWindow`）→ `OnWindowAllClosed`（macOS 关窗不退出，其余平台退出）→ `OnActivate`（点 Dock 图标重开窗口）→ `App.Run()`。窗口标题 `NCMconverter`，宽高 860×760（最小 620×540），底色 `light-dark(#f6f7f9, #16181d)` 与 `style.css` 的 `--bg` 一致以免切换闪白，`StateKey: "main"` 记住位置。

## 4. 前端与生成的客户端

前端是 TypeScript + Vite 的单页应用，**只通过 `mygo generate` 生成的客户端调用 Go，不手写 `call("...")`**。

- [`desktop/index.html`](../../../desktop/index.html)：中文界面——队列（添加文件/文件夹/清空、拖拽提示）、选项（输出目录、命名模板、depth、threads、写标签/跳过已存在/试运行）、转换（进度条、汇总、日志）、页脚版本。
- [`desktop/src/main.ts`](../../../desktop/src/main.ts)：`Channel<Report>` 收逐文件进度并推进 `progress`；`onFileDrop` 接收拖拽入队；`events.filesOpened.on(...)` 接收菜单选中的路径；`Converter.scan` 在开跑前统计工作量、`Converter.run`/`cancel`/`openPath` 驱动转换与打开输出目录。运行期间队列与选项被禁用（单飞）。
- [`desktop/src/mygo.ts`](../../../desktop/src/mygo.ts)：**由 `bun run generate` 生成并提交**，文件头写着 `Code generated by mygo generate. DO NOT EDIT.`。它导出 `Converter`（`info`/`pickFiles`/`pickDirectory`/`pickOutput`/`scan`/`run`/`cancel`/`openPath`）、`Info`/`Request`/`Report`/`Result` 接口与 `events.filesOpened`。CI 在 Linux 上跑 `make generate` 后 `git diff --exit-code -- desktop/src/mygo.ts` 校验它与 Go 代码同步。

## 5. 构建与开发工作流

仓库根的 [`package.json`](../../../package.json) 把脚本转发给 `mygo` 与 `vite`：

| 命令 | 实际执行 | 用途 |
| --- | --- | --- |
| `bun run dev` | `mygo dev`（跑 `devCommand`=`vite`，加载 `devUrl`） | 日常开发，改前端即时热更新 |
| `bun run generate` | `mygo generate` | 改 Go 绑定后重生成 `desktop/src/mygo.ts` |
| `bun run build` | `mygo build`（跑 `buildCommand`=`tsc --noEmit && vite build`） | 打包发布产物 |
| `bun run dev:web` / `build:web` / `typecheck` | 直接 `vite` / `tsc + vite build` / `tsc --noEmit` | 只跑前端，不经 MyGo |

[`Makefile`](../../../Makefile) 把同样的动作包成目标，且**与 Go 门禁分离**（Go 目标不依赖 bun）：`make desktop`（`bun install --frozen-lockfile && bun run build`）、`make generate`、`make typecheck`。

`mygo dev` 用 `devUrl` 直接加载页面，不需要嵌入；`mygo build` 才做打包。

### 5.1 `make build` 与打包产物的差别

**普通 `go build` 不注入前端 overlay，产出的二进制桌面模式打不开页面**（Windows 上还会带黑框）。这是单二进制方案的固有约束，因此发布产物必须由 `mygo build` 产出：

| | `go build`（`make build`） | `mygo build`（`bun run build`） |
| --- | --- | --- |
| 前端 | **不嵌入** | 用 `-overlay` 把 `frontendDist`（`desktop/dist`）注入主包 |
| Windows 子系统 | 控制台 | `-H=windowsgui`（GUI） |
| 产物 | `bin/ncmconverter`（裸二进制） | `dist/<平台>/` 下的 `.app`+dmg / exe+NSIS 安装器 / tar.gz+deb |
| 用途 | 快速编译门禁、CLI 路径 | 桌面开发与发布 |

`make build` 保持为「快」的编译门禁，它的二进制用于 CLI（`bin/ncmconverter --help` 等）；桌面开发与打包一律走 `mygo dev` / `mygo build`。发布链路与产物清单见 [发布产物](../release/release-artifacts.md)。

## 6. 平台差异与运行时依赖

| | macOS | Windows | Linux |
| --- | --- | --- | --- |
| 子系统 | 常规 | `windowsgui` + `AttachConsole` | 常规 |
| 桌面运行时依赖 | 系统 WebKit | WebView2 Runtime | **GTK + WebKitGTK**（仅桌面模式） |
| CLI 运行时依赖 | 无 | 无 | 无（MyGo 无 cgo，GTK/WebKit 是运行时按需加载） |
| 打包 | `.app` + dmg（签名/公证需 macOS） | exe + NSIS 安装器 | tar.gz（含 `install.sh`）+ deb |
| 关窗行为 | 留在 Dock，点图标重开 | 退出 | 退出 |

**headless 安全性**：桌面分支之外不触碰任何 MyGo API，GTK/WebKitGTK 不会在 CLI 模式下被加载，因此在服务器/CI/Docker 上直接用 CLI 是安全的。Linux 的 deb 会声明依赖 GTK/WebKitGTK，CLI-only 用户会被牵连安装——这是单二进制的固有代价，记入 [backlog](../../planning/backlog.md)。

## 关键文件

- [`cmd/ncmconverter/main.go`](../../../cmd/ncmconverter/main.go) — 双模式分支、`runCLI`、版本注入变量
- [`cmd/ncmconverter/console_windows.go`](../../../cmd/ncmconverter/console_windows.go) / [`console_other.go`](../../../cmd/ncmconverter/console_other.go) — `attachConsole`
- [`internal/desktop/service.go`](../../../internal/desktop/service.go) — `Converter`、DTO、`Opened`、对话框助手
- [`internal/desktop/app.go`](../../../internal/desktop/app.go) — 应用装配、窗口、菜单
- [`desktop/src/main.ts`](../../../desktop/src/main.ts) — 页面逻辑；[`desktop/src/mygo.ts`](../../../desktop/src/mygo.ts) — 生成的客户端
- [`mygo.config.ts`](../../../mygo.config.ts) / [`vite.config.ts`](../../../vite.config.ts) / [`Makefile`](../../../Makefile) — 构建配置

## 相关测试 / 文档

- 测试：`internal/desktop/service_test.go`（服务行为，无窗口）、`internal/app/report_test.go`（回调语义）
- 需求与设计：[`requirements/desktop/`](../../requirements/desktop/)；决策：[ADR-010](../../planning/architecture.md)
- 发布：[`features/release/release-artifacts.md`](../release/release-artifacts.md)；CLI 语义：[`features/cli/cli-and-orchestration.md`](../cli/cli-and-orchestration.md)
