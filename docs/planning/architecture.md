# 架构决策记录（ADR）

> 记录关键架构决策及其理由。格式：日期 / 决策 / 背景 / 理由 / 后果。
> 系统总览见 [project-architecture.md](project-architecture.md)；决策中涉及的代码细节见 [../features/](../features/)。

## 技术栈总览

| 层       | 技术                                                             |
| -------- | ---------------------------------------------------------------- |
| 语言     | Go（版本以 [`../../go.mod`](../../go.mod) 为准）                  |
| CLI      | `github.com/urfave/cli/v3`                                       |
| 桌面     | `github.com/egoist/mygo` v0.5.5（纯 Go、无 cgo）+ `mygo/ui` 原生 UI（无 webview） |
| 并发     | `golang.org/x/sync/errgroup`                                     |
| 编码     | `golang.org/x/text`（GBK）                                       |
| mp3 标签 | `github.com/bogem/id3v2`                                         |
| flac 标签| `github.com/go-flac/go-flac/v2` + `flacvorbis/v2` + `flacpicture/v2` |
| 工具链   | `go tool mygo`（`go.mod` 的 tool 指令；无 Node/bun）              |
| 发布     | `mygo build`（桌面与 CLI 共用一个二进制；取代 GoReleaser/nfpm）    |
| 测试     | 标准 `go test`（含 `-race`；视图测试用 `ui.NewTester`）           |

---

## ADR-001: 自实现容器解析与解密（不套壳上游工具）

- **日期**: 2026-09（项目重写时确立）
- **决策**: 直接按字节布局解析 `.ncm` 并自行实现密钥派生与反混淆，而不是调用 `ncmdump` 等外部程序。
- **背景**: 格式的最初解析参考了 [yoki123/ncmdump](https://github.com/yoki123/ncmdump)，但需要并发转换、可编程输出命名与跨平台发布。
- **理由**: 外部程序无法提供库级错误处理、并发与输出控制；直接解析还能把格式布局沉淀成文档与测试。
- **后果**: 必须自己承担格式漂移的风险（布局、未校验字段都写进注释与测试）；解密的正确性靠逐字节回归锚点保证。

## ADR-002: 单向管线，`internal/` 全私有

- **日期**: 2026-09
- **决策**: 数据流固定为 `ncm.File → converter.Converter → tag`，所有实现包放在 `internal/` 下，只暴露 `cmd/ncmconverter` 一个入口。
- **理由**: 单向依赖便于推理与测试；`internal/` 保证没有外部导入者，格式细节可以随时调整。
- **后果**: 包之间不互相反向依赖（`app` 依赖 `ncm`/`converter`/`tag`，`tag` 依赖 `converter` 的类型与常量，没有环）。

## ADR-003: 测试用独立的合成容器构造器（`internal/ncmtest`）

- **日期**: 2026-09
- **决策**: 测试夹具由 `internal/ncmtest` **独立重新实现**容器格式来构造，而不是复用被测的 `ncm`/`converter` 代码。
- **背景**: 若夹具复用被测代码，读取端的一个 bug 会同时藏进「本该抓住它」的夹具里，测试照常通过。
- **理由**: 独立实现让读取端的错误无处可藏；内存中构造也免去对真实素材的依赖。
- **后果**: `ncmtest` 与生产代码有刻意的重复（AES、密钥盒、异或逻辑各写一份）；这是**特性不是缺陷**。详见 [../engineering/testing.md](../engineering/testing.md)。

## ADR-004: 标签锁定 ID3v2.4 + UTF-8

- **日期**: 2026-09
- **决策**: mp3 的标签固定写入 **ID3v2.4**，文本帧一律 UTF-8。
- **背景**: 容器里的音频常常自带一个 ID3v2.3 标签，而 v2.3 默认 ISO-8859-1 编码无法表示中文/日文标题与艺人。
- **理由**: 只有顶掉版本、强制 UTF-8，非拉丁文本才能无损写入并正确显示。
- **后果**: `mp3.New` 打开后立即 `SetVersion(4)`；非拉丁字段的回归由 mp3 测试锁定。详见 [../features/tagging/mp3-id3.md](../features/tagging/mp3-id3.md)。

## ADR-005: 「已有值优先」，不覆盖源文件标签

- **日期**: 2026-09
- **决策**: 每个 `Set*` 只在目标字段**尚不存在**时才写入。
- **理由**: 源音频可能已经带有更可信的标签；本工具是「补齐缺失」，不是「以元数据为准覆盖一切」。
- **后果**: flac 用 `setOnce`，mp3 用「帧是否已存在」判定；重复保存不改变已有值，由各自的测试锁定。

## ADR-006: 歌词走同名 `.lrc` 侧车

- **日期**: 2026-09
- **决策**: 歌词从与容器同名的兄弟 `.lrc` 读取，原样嵌入（mp3 → `USLT`，flac → `LYRICS`）；缺失不是错误。
- **背景**: 容器格式没有任何位置存放歌词。
- **理由**: 只查源目录、不递归、不复制到输出目录，规则简单可预期；BOM/GBK 兼容历史文件。
- **后果**: 缺少 `.lrc` 是常见情况，相关路径必须容忍为空。详见 [../features/lyrics/lyrics.md](../features/lyrics/lyrics.md)。

## ADR-007: `errgroup` 并发 + 单文件失败隔离 + 最终非零退出

- **日期**: 2026-09
- **决策**: 用 `errgroup` 的 `SetLimit` 控制并发；单个文件失败被记录并跳过而**不中断整批**；只要有失败，进程最终以非零状态退出。
- **背景**: 批量转换里一个坏容器不应让整批作废，但脚本化使用时又需要能感知失败。
- **理由**: 把「继续处理」和「报告失败」解耦——错误聚合到切片，`Wait` 只用来接收取消。
- **后果**: `Run` 返回的错误里含「N/M files failed」与 `errors.Join` 的明细；取消是唯一经由 `Wait` 冒泡的错误。详见 [../features/cli/cli-and-orchestration.md](../features/cli/cli-and-orchestration.md)。

## ADR-008: GoReleaser + nfpm 发布平台矩阵（**已被 [ADR-010](#adr-010-桌面应用与-cli-共用一个二进制mygo) 取代**）

> 2026-10-15：发布链路改由 `mygo build` 驱动（桌面产物必须设置 Windows GUI 子系统并打包安装器，
> 前端形态见 [ADR-011](#adr-011-桌面界面改用-mygo-原生-ui)），
> GoReleaser 与 nfpm 退役，`.goreleaser.yaml` 删除。本条保留作为历史决策与背景。

- **日期**: 2026-09
- **决策**: 用 GoReleaser 交叉编译 `linux`/`darwin`/`windows` × `amd64`/`arm64`，Windows 出 `zip`、其余出 `tar.gz`，Linux 另出 `deb`/`rpm`，并生成 `checksums.txt`。
- **背景**: Go 天然跨平台；发布链路最初没有任何产物，CI 也只在 Linux 上跑测试。
- **理由**: 一次 tag 产出全部平台与安装包，降低分发成本；版本信息经 `-ldflags` 注入。
- **后果**: CI 增加三平台测试矩阵与覆盖率上传；发布说明的过滤规则与 `CHANGELOG.md` 分工明确。详见 [../features/release/release-artifacts.md](../features/release/release-artifacts.md)。

## ADR-009: 门禁统一走 make 目标，CI 锁定 lint 版本

- **日期**: 2026-09
- **决策**: CI 的 test/coverage 步骤改为调用 `make vet`/`make test-race`/`make build`/`make coverage`，避免 workflow 与 Makefile 两处维护；`golangci-lint` 版本在 CI 中锁定（`v2.14.0`）。
- **背景**: lint 曾用 `version: latest`，上游一次发布就可能把绿色构建改红。
- **理由**: 单一事实来源（Makefile）降低漂移；锁定版本让红绿变化只由代码引起。
- **后果**: 本地 golangci-lint 需与 CI 相同版本；新增/调整门禁时先改 Makefile。lint job 仍走 action（它自己负责安装与锁定）。

## ADR-010: 桌面应用与 CLI 共用一个二进制（MyGo）

- **日期**: 2026-10-15
- **决策**: 用 [MyGo](https://github.com/egoist/mygo)（`github.com/egoist/mygo` v0.5.5，纯 Go、无 cgo）为项目加一个**桌面入口**，它与原 CLI **共用同一个可执行文件**：无参数启动即打开窗口，带参数即当前 CLI。桌面与 CLI 共用 `internal/app` 的编排，桌面通过新增的 `Options.OnFile` 回调获取逐文件进度；发布链路改由 `mygo build` 驱动（取代 [ADR-008](#adr-008-goreleaser--nfpm-发布平台矩阵已被-adr-010-取代)）。
- **背景**:
  - 项目此前只有批处理 CLI，普通用户门槛高；`backlog.md` 曾把 GUI 裁为「不接」，本次明确反转（定位扩展为「面向普通用户的转换器」，且不产生第二套转换逻辑）。
  - 用户要求「同一个执行文件，既能打开为 desktop，也能作为 shell 调用」。
  - MyGo 的 `mygo build` 给 Windows 加 `-H=windowsgui` 并打包各平台安装器；这些是普通 `go build` 做不到的。界面形态见 [ADR-011](#adr-011-桌面界面改用-mygo-原生-ui)。
- **理由**:
  - MyGo 是纯 Go 包，`os.Args` 分支发生在 `App.Run()` 之前，CLI 路径完全不触碰 MyGo API，因此**单二进制不会给 headless/脚本场景引入 GUI 运行时依赖**（GTK 3 是运行时按需加载）。
  - 复用 `internal/app` 保证「CLI 与桌面行为一字不差」；`app` 仍 UI 无关（不导入 mygo），依赖方向不破坏 [ADR-002](#adr-002-单向管线internal-全私有)。
  - 发布只用一种产物家族，避免「同名但能力不同」的二进制并存。
- **后果**:
  - Windows 需要 `AttachConsole` 垫片才能让 GUI 子系统构建在终端里输出（`cmd/ncmconverter/console_windows.go`，仅标准库 `syscall`）。
  - 二进制体积增加；Linux 的 deb 会带上 GTK 3 依赖（仅桌面模式需要）。作为单二进制的固有代价记录在 backlog。
  - 发布产物形状变化：macOS `.app`+dmg、Windows exe+NSIS 安装器、Linux tar.gz（`install.sh`）+deb；**不再产出 rpm 与 `checksums.txt`**（跟进项记入 backlog）。
  - `make build`（普通 `go build`）不再等同于发布产物：它不做 GUI 子系统与安装器打包，作为快速编译门禁保留；桌面开发与打包走 `go tool mygo dev` / `go tool mygo build`。
  - 需求、设计与测试方案见 [../requirements/desktop/](../requirements/desktop/)。

## ADR-011: 桌面界面改用 MyGo 原生 UI

- **日期**: 2026-10-16
- **决策**: 桌面的窗口界面用 MyGo 的 **原生 UI**（`github.com/egoist/mygo/ui`，Go 直接声明、GPU 绘制）实现，**取代** [ADR-010](#adr-010-桌面应用与-cli-共用一个二进制mygo) 当时选定的 web 前端方案（原 [design-decisions DD-3](../requirements/desktop/05-review/design-decisions.md)）。据此删除 `desktop/`（HTML/TS/CSS）、`mygo.config.ts`、`package.json`、`tsconfig.json`、`vite.config.ts`、`bun.lock`，改为 Go 视图（`internal/desktop/view.go`）与 `mygo.json`；MyGo CLI 以 `go.mod` 的 **tool 指令**引入，工具链变为 `go tool mygo`。
- **背景**:
  - ADR-010 的选择理由是「表单/列表/日志用 HTML 写最快」。实测下来，web 前端产出的界面**不像原生应用**（自绘的按钮/表单/进度条与系统控件观感不一致），UX 也差。
  - MyGo 自带完整的原生控件与主题体系：`ui.Form`/`Field`/`Fieldset`、`ui.List`、`ui.Sidebar`、`ui.Toolbar`、`ui.Badge`、`ui.Progress`/`Spinner`、`ui.Switch`，主题跟随系统外观/强调色/字号/对比度，并支持隐藏标题栏（`TitleBarHiddenInset` + `Context.TitleBar` + `DragWindow`）与窗口材质（`Vibrancy`，macOS 全量、Windows 11 Mica/Acrylic）。
  - 原生 UI 的视图能在**没有窗口**的情况下由 `ui.NewTester` 驱动，因此界面第一次变得可自动化测试。
- **理由**:
  - 观感与交互直接由系统控件与主题决定，跟随系统设置，不需要用 CSS 去模仿原生。
  - 依赖链更短、体积更小：**不再需要 WebView**——Linux 只需 GTK 3（不再需要 WebKitGTK），Windows 不再需要 WebView2 Runtime。
  - 不再需要 Node/bun/Vite/生成客户端这一整条工具链；配置从需要 JS 运行时求值的 `mygo.config.ts` 简化为纯数据 `mygo.json`。
- **后果**:
  - 界面从「HTML 布局 + CSS」变为 Go 视图代码；状态变化跨线程一律经 `Window.Update`，视图本身只在主线程读写状态。
  - 绑定层消失（不再有 `mygo.Bind`/`Channel`/`NewEvent`/生成的 TS 客户端），因此 CI 不再校验生成物同步，`make generate`/`make typecheck` 目标删除。
  - 新增视图测试（`ui.NewTester`，无窗口）；[engineering/testing.md](../engineering/testing.md) 的「桌面不写 UI 测试」结论被推翻。
  - Linux 桌面依赖由 GTK/WebKitGTK 收敛为 **GTK 3**，backlog 的「Linux 桌面运行时依赖策略」随之缓解（仍需 GTK）。
  - 需求、设计与测试方案同步更新：[PRD](../requirements/desktop/01-requirements/PRD.md)、[设计](../requirements/desktop/02-design/architecture.md)、[UI 设计](../requirements/desktop/02-design/ui-design.md)、[DD-3](../requirements/desktop/05-review/design-decisions.md)。
