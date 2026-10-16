# PRD：NCMconverter 桌面应用

> 状态：**已实现**（2026-10-15 定稿；2026-10-16 界面改为 MyGo 原生 UI，见 [ADR-011](../../../planning/architecture.md)）
> 关联：[user-stories.md](user-stories.md)、[../02-design/architecture.md](../02-design/architecture.md)、[../03-implementation/phased-plan.md](../03-implementation/phased-plan.md)
> 决策记录：[ADR-010](../../../planning/architecture.md)、[ADR-011](../../../planning/architecture.md)、[../05-review/design-decisions.md](../05-review/design-decisions.md)

## 1. 背景

NCMconverter 目前只有 `cmd/ncmconverter` 一个批处理 CLI：能并发转换、能保留元数据/封面/歌词，但门槛在命令行——普通用户要记参数、看日志、自己去输出目录找文件。仓库现有素材（`testdata/` 的真实样本、`internal/ncmtest` 的合成容器）已经足够支撑一个带进度的图形界面。

同时，`docs/planning/backlog.md` 的「不做清单」里有一条旧裁决：

| 条目 | 裁决 | 理由 |
| --- | --- | --- |
| GUI / 常驻服务 | 删除（不接） | 定位是批处理 CLI |

本需求**明确反转**该裁决（见 §7 与 [design-decisions](../05-review/design-decisions.md)），理由：

1. 定位从「批处理工具」扩展为「面向普通用户的转换器」，CLI 继续存在但不再是唯一入口。
2. 桌面外壳可以**复用同一套编排**（`internal/app`），不产生第二套转换逻辑，维护成本可控。
3. 技术选型上 MyGo 是纯 Go、无 cgo、单文件几 MB，不引入 Node/Electron 运行时，也不会让 CLI 侧背上 GUI 依赖。

## 2. 目标

**一句话**：让同一个可执行文件既是原来的命令行工具，又是一个带进度的桌面应用；用户双击即用，脚本里调用行为不变。

**具体目标**

- G1 **单二进制双模式**：`ncmconverter`（无参数）打开桌面窗口；`ncmconverter [flags] <files/dirs>` 行为与现在完全一致。
- G2 **零逻辑分叉**：桌面与 CLI 共用 `internal/app` 的编排、`internal/{ncm,converter,tag}` 的解析/解密/标签，只有「谁来看进度」不同。
- G3 **逐文件进度**：桌面能实时显示每个容器的结果（转换/跳过/失败），并可取消。
- G4 **完整功能对齐**：CLI 的全部开关在界面里都能设：输出目录、命名模板、目录层级、并发数、写标签、跳过已存在、试运行。
- G5 **可发布**：CI 能编译并打包三平台产物；tag 触发发布，产物挂到 GitHub Release。
- G6 **不破坏既有门禁**：`make fmt-check vet test-race build`、覆盖率与 `golangci-lint` 继续全绿。

## 3. 非目标

- **不做文件关联**：不注册 `.ncm` 双击打开（拖拽入队已覆盖主要场景）。留作 backlog。
- **不做自更新**：不接入 MyGo 的 `updates`（签名更新链路需要密钥体系）。留作 backlog。
- **界面走原生 UI（原「不做原生 UI」条目已反转）**：界面采用 MyGo **原生 UI**（`github.com/egoist/mygo/ui`，Go 声明、GPU 绘制），**不再**走 web 前端，记为 [ADR-011](../../../planning/architecture.md)，取代旧的 [DD-3](../05-review/design-decisions.md)。仍不在 scope 内的是：文件关联、自更新、托盘常驻、CLI 语义变更、新增转换能力——见本节其余条目。
- **不做系统托盘 / 常驻**：窗口关掉即退出（macOS 保留 Dock 行为）。
- **不改 CLI 的语义**：参数名、默认值、日志文案、退出码一律不变。
- **不新增转换能力**：不联网抓歌词、不转码到其它格式（沿用既有裁决）。

## 4. 用户与场景

| 用户 | 场景 | 诉求 |
| --- | --- | --- |
| 普通用户 | 下载了几十首 `.ncm`，想转成 mp3 丢进播放器 | 拖进来、点一下、看得到进度和结果 |
| 音乐整理者 | 想按「艺人 - 标题」命名、跳过已转过的 | 界面里能填模板、勾选跳过 |
| 脚本/CI 用户 | `ncmconverter -d 2 -o out ~/Music/ncm` | 行为一字不变，退出码可判定 |
| 开发者 | 想改界面 | `go tool mygo dev` 改 Go 即重建重启；界面与逻辑同为 Go |

## 5. 功能需求

### FR-1 双模式入口

- FR-1.1 进程启动时若 `os.Args` 有参数（且 `MYGO_GENERATE` 未设置）→ 走 CLI；否则走桌面。
- FR-1.2 `--help` / `--version` 归 CLI；`--version` 输出仍含 commit 与构建时间。
- FR-1.3 Windows 上 GUI 子系统构建从终端调用时，CLI 的 stdout/stderr 必须能接上父控制台。
- FR-1.4 CLI 分支的日志级别、`--quiet`、退出码语义不变。

### FR-2 转换队列

- FR-2.1 通过系统对话框添加 `.ncm` 文件（可多选，过滤器限定 `.ncm`）。
- FR-2.2 通过系统对话框添加目录。
- FR-2.3 支持把文件/目录**拖拽**进窗口加入队列。
- FR-2.4 队列项可逐个移除，可一键清空。
- FR-2.5 队列变化时实时统计「N 个来源 · M 个 .ncm」（复用与运行完全一致的遍历语义）。
- FR-2.6 **每个来源显示它已达到的阶段**，让一次运行的流程在队列里直接可读；每个来源前有一枚标记：等待为圆环、转换中为进度指示、完成为对勾、失败或路径不可用为警示；配色取自主题（运行 `Accent`、完成 `Success`、失败/不可用 `Danger`、等待 `TextMuted`）。
- FR-2.7 来源阶段措辞：`待处理`（尚未扫描）、`待处理 · N 个`（已扫描，含 N 个容器）、`转换中 · d/t`（进行中，完成 d / 共 t）、结束 `N 已转换 · M 跳过 · K 失败`（只列非零项）、试运行结束 `N 个将转换`、`无 .ncm`（不含容器）、`路径不可用`（路径读不到）。来源各自展开归属容器（`scanBySource`，与 `app.Scan` 对重叠输入的「首个来源胜出」去重一致），因此一份报告的 `Source` 唯一映射回一个队列项；读不到的来源标记为 `路径不可用`，而不是把整个计数清零（运行本身仍会报告它）。

### FR-3 选项

- FR-3.1 输出目录（选择目录；留空表示与源文件同目录，界面明确提示）。
- FR-3.2 命名模板（占位符 `{name}/{title}/{artist}/{album}/{id}/{format}`，占位符列表在界面可查）。
- FR-3.3 目录层级 `depth`（≥0 整数）。
- FR-3.4 并发数 `threads`（≥1 整数）。
- FR-3.5 写标签（默认开）、跳过已存在（默认关）、试运行（默认关）。
- FR-3.6 运行期间选项与队列操作禁用，避免与进行中的批次冲突。

### FR-4 运行与进度

- FR-4.1 运行按钮启动转换；同一次运行中禁止再次启动（单飞）。
- FR-4.2 每个容器处理完立即产生一条记录：来源 → 目标、结果（转换/跳过/失败）、失败原因。
- FR-4.3 进度条按「已处理 / 计划数」推进；试运行时计划数即为将列出的文件数。
- FR-4.4 取消按钮立即停止：不再领取新文件，已写出的结果保留。
- FR-4.5 结束后给出汇总：总数、转换、跳过、失败、耗时。
- FR-4.6 失败不中断整批（沿用 [ADR-007]），且必须在界面上可见（计数 + 每行原因）。
- FR-4.7 提供「打开输出目录」（有显式输出目录用它，否则用首个产物的所在目录）。
- FR-4.8 **运行状态卡**：一次运行开始后，队列列表上方出现一张状态卡并在结束后保留——含进度条（`转换进度`；在计划数已知前为不确定态）、着色的计数徽标（`已转换 N` / `已跳过 N` / `失败 N`；为零的徽标用 `TextMuted` 低调呈现，非零的按 `Success`/`Warning`/`Danger` 着色）、汇总文案（`共 N 个 · 1.2s`，另有 `试运行` / `已取消` 标记）、`打开输出目录` 按钮（试运行时隐藏），以及运行无法启动时以 `Danger` 显示的运行级错误。
- FR-4.9 **运行状态处处可见**：标题栏带一枚运行状态胶囊，从任意分区都能看到——运行中为进度指示 + `d / t`；结束后为 `已完成`（Success）/ `完成 · N 个失败`（Danger）/ `已取消`（Warning）/ `试运行完成`（弱化）/ `运行失败`（Danger）。标题栏同时承载 `取消`（仅运行中）与主操作 `开始转换`（运行中或队列为空时禁用）。

### FR-5 平台与外观

- FR-5.1 亮/暗、强调色、字号、对比度、减弱动效跟随系统；主题经 `Context.SetTheme` 微调（`Radius = 8`），窗口底色与主题一致，避免切换时闪白。
- FR-5.2 中文界面；文案与 CLI 的 `--help` 概念一致。
- FR-5.3 窗口尺寸、最小尺寸合理；记忆上次位置（`StateKey`）。
- FR-5.4 菜单：应用菜单、文件（添加文件/添加文件夹/关闭）、编辑、视图、窗口、转换（开始转换/取消/打开输出目录）；快捷键 `CmdOrCtrl+O`、`CmdOrCtrl+Shift+O`、`CmdOrCtrl+Return`（开始转换）。
- FR-5.5 macOS 关闭全部窗口后应用保留在 Dock，点 Dock 图标重新开窗。
- FR-5.6 窗口为 MyGo **原生 UI**：半透明**侧栏**（窗口材质 `VibrancySidebar`；无材质的平台如 Linux 改画表面背景）分**转换队列 / 选项 / 日志**三区，各有图标与计数徽标；内容区不透明并自绘**内嵌标题栏**（可拖拽、避开窗口控件）。
- FR-5.7 操作出错（对话框失败、打开输出目录失败等）以 `ui.Context.Toast` 提示，不写成面板里的常驻文字。

## 6. 非功能需求

| 编号 | 需求 | 说明 |
| --- | --- | --- |
| NFR-1 | **headless 可用** | CLI 模式不得在无显示/无 GTK 3 的环境下失败或变慢；二进制加大是唯一代价 |
| NFR-2 | **无 cgo** | 保持 `CGO_ENABLED=0`，任意平台可交叉编译 |
| NFR-3 | **界面即 Go，无生成客户端** | 界面用 MyGo **原生 UI** 在 Go 里声明（无 web 前端、无 `mygo generate` 生成的客户端）；视图可在无窗口下由 `ui.NewTester` 单元测试 |
| NFR-4 | **可测** | 新增逻辑有 `go test`（含 `-race`）覆盖；不引入需要窗口的脆弱测试 |
| NFR-5 | **门禁不变** | `make fmt-check/vet/test-race/build`、覆盖率、lint 全绿 |
| NFR-6 | **打包可用** | `go tool mygo build` 在三平台产出可安装产物；tag 触发发布 |
| NFR-7 | **体积** | 桌面产物与 CLI 同为「几 MB 级」，不引入 Node/Electron 运行时 |

## 7. 验收标准

实现完成的判定（可运行、可复现）：

| 编号 | 验收项 | 方式 |
| --- | --- | --- |
| AC-1 | 无参数启动打开窗口，界面可用 | 人工冒烟 + `go tool mygo dev` |
| AC-2 | `bin/ncmconverter --version` 输出含 commit/date | Windows 实机（含从终端调用） |
| AC-3 | `--dry-run -d 2 <目录>` 输出与现状逐字一致 | 人工比对 + 既有测试 |
| AC-4 | 界面完成一次真实转换，产物可播放、带标签与歌词 | 人工冒烟（真实 `.ncm`） |
| AC-5 | 取消后不再写新文件，汇总显示「已取消」 | 人工冒烟 |
| AC-6 | 失败文件在界面上可见且不中断整批 | 人工冒烟（掺一个坏容器） |
| AC-7 | `make fmt-check vet test-race build` 全绿 | CI 与本地 |
| AC-8 | 转换内核与视图的单元测试都在**无窗口**下通过（视图由 `ui.NewTester` 驱动） | `go test ./internal/desktop` + CI |
| AC-9 | `go tool mygo build` 产出安装包 | CI（三平台矩阵） |
| AC-10 | 发布 workflow 把产物挂到 tag 的 Release | 打一个预发布 tag 验证 |

## 8. 约束与依赖

- **Go**：版本以 `go.mod` 为准（`1.27.2`，MyGo v0.5.5 要求）；MyGo CLI 由 `go.mod` 的 **tool 指令**（`tool github.com/egoist/mygo/cmd/mygo`）引入，运行方式为 `go tool mygo`，**不需要 Node/bun**。
- **MyGo `github.com/egoist/mygo` v0.5.5**：桌面框架；纯 Go、无 cgo；界面用其**原生 UI**（`github.com/egoist/mygo/ui`）。
- **配置**：`mygo.json`（纯数据，读取它不需要 JS 运行时；没有前端要构建或内嵌）。
- **CLI**：`github.com/urfave/cli/v3`（不变）。
- **平台**：macOS / Windows / Linux。
  - macOS：dmg + `.app`；签名与公证需要 macOS runner 与证书。
  - Windows：exe + NSIS 安装器；`go tool mygo build` 会加 `-H=windowsgui`。
  - Linux：tar.gz（含 `install.sh`）+ deb；**运行时只需要 GTK 3**（原生 UI 不再需要 WebKitGTK；仅桌面模式）。

## 9. 风险

| 风险 | 影响 | 应对 |
| --- | --- | --- |
| Windows GUI 子系统导致 CLI 无输出 | CLI 在 Windows 上「哑掉」 | `AttachConsole` 垫片（FR-1.3），并列为验收项 AC-2 |
| Linux 桌面包把 GTK 3 变成安装依赖 | CLI-only 用户被牵连装 GUI 库 | 在 README/features 说明；原生 UI 已去掉 WebKitGTK，顾虑减轻；如仍不可接受，退回「两个二进制」方案（见 design-decisions 备选） |
| MyGo 上游 API 变动 | 编译失败 | 版本固定 v0.5.5；升级按 `mygo-maintenance` 流程走 |
| 拖拽路径在部分平台拿不到 | 拖拽不可用 | 保留对话框入口为主路径 |
| 发布链路从 GoReleaser 切到 `go tool mygo build` | 产物形状变化（无 rpm/checksums） | 记入 CHANGELOG 与 backlog 跟进 |

## 10. 里程碑

| 阶段 | 内容 | 出口条件 |
| --- | --- | --- |
| M1 复用层 | `internal/app` 暴露 `Scan` 与 `OnFile` | 行为不变的测试全绿 |
| M2 桌面应用 | `internal/desktop` + 双模式入口 + 原生 UI 视图 | 本地 `go tool mygo dev` 可完成一次转换 |
| M3 工程化 | Makefile / CI / 发布 / 文档 | CI 全绿，`go tool mygo build` 出包 |
| M4 交付 | 分支 + 两个提交 + PR | 检查清单勾满 |

详见 [phased-plan.md](../03-implementation/phased-plan.md)。
