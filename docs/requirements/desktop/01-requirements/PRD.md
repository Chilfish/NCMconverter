# PRD：NCMconverter 桌面应用

> 状态：**已定稿，待实现**（2026-10-15）
> 关联：[user-stories.md](user-stories.md)、[../02-design/architecture.md](../02-design/architecture.md)、[../03-implementation/phased-plan.md](../03-implementation/phased-plan.md)
> 决策记录：[ADR-010](../../../planning/architecture.md)、[../05-review/design-decisions.md](../05-review/design-decisions.md)

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
- **不做原生 UI**：界面走 web 前端（HTML/TS），不使用 MyGo 的 `package ui`。原因是现有页面交互（列表、表单、日志）用 web 技术实现成本最低，且开发迭代快。
- **不做系统托盘 / 常驻**：窗口关掉即退出（macOS 保留 Dock 行为）。
- **不改 CLI 的语义**：参数名、默认值、日志文案、退出码一律不变。
- **不新增转换能力**：不联网抓歌词、不转码到其它格式（沿用既有裁决）。

## 4. 用户与场景

| 用户 | 场景 | 诉求 |
| --- | --- | --- |
| 普通用户 | 下载了几十首 `.ncm`，想转成 mp3 丢进播放器 | 拖进来、点一下、看得到进度和结果 |
| 音乐整理者 | 想按「艺人 - 标题」命名、跳过已转过的 | 界面里能填模板、勾选跳过 |
| 脚本/CI 用户 | `ncmconverter -d 2 -o out ~/Music/ncm` | 行为一字不变，退出码可判定 |
| 开发者 | 想改界面 | `bun run dev` 热更新；类型由 Go 代码生成 |

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

### FR-5 平台与外观

- FR-5.1 亮/暗随系统；窗口底色与页面 `--bg` 一致，避免切换时闪白。
- FR-5.2 中文界面；文案与 CLI 的 `--help` 概念一致。
- FR-5.3 窗口尺寸、最小尺寸合理；记忆上次位置（`StateKey`）。
- FR-5.4 菜单：应用菜单、文件（添加文件/添加文件夹/关闭）、编辑、视图、窗口；快捷键 `CmdOrCtrl+O`、`CmdOrCtrl+Shift+O`。
- FR-5.5 macOS 关闭全部窗口后应用保留在 Dock，点 Dock 图标重新开窗。

## 6. 非功能需求

| 编号 | 需求 | 说明 |
| --- | --- | --- |
| NFR-1 | **headless 可用** | CLI 模式不得在无显示/无 GTK/WebKitGTK 的环境下失败或变慢；二进制加大是唯一代价 |
| NFR-2 | **无 cgo** | 保持 `CGO_ENABLED=0`，任意平台可交叉编译 |
| NFR-3 | **类型安全** | 前端调用的方法/事件/载荷由 `mygo generate` 从 Go 代码生成，两侧编译期同源 |
| NFR-4 | **可测** | 新增逻辑有 `go test`（含 `-race`）覆盖；不引入需要窗口的脆弱测试 |
| NFR-5 | **门禁不变** | `make fmt-check/vet/test-race/build`、覆盖率、lint 全绿 |
| NFR-6 | **打包可用** | `mygo build` 在三平台产出可安装产物；tag 触发发布 |
| NFR-7 | **体积** | 桌面产物与 CLI 同为「几 MB 级」，不引入 Node/Electron 运行时 |

## 7. 验收标准

实现完成的判定（可运行、可复现）：

| 编号 | 验收项 | 方式 |
| --- | --- | --- |
| AC-1 | 无参数启动打开窗口，页面可用 | 人工冒烟 + `mygo dev` |
| AC-2 | `bin/ncmconverter --version` 输出含 commit/date | Windows 实机（含从终端调用） |
| AC-3 | `--dry-run -d 2 <目录>` 输出与现状逐字一致 | 人工比对 + 既有测试 |
| AC-4 | 界面完成一次真实转换，产物可播放、带标签与歌词 | 人工冒烟（真实 `.ncm`） |
| AC-5 | 取消后不再写新文件，汇总显示「已取消」 | 人工冒烟 |
| AC-6 | 失败文件在界面上可见且不中断整批 | 人工冒烟（掺一个坏容器） |
| AC-7 | `make fmt-check vet test-race build` 全绿 | CI 与本地 |
| AC-8 | `bun run generate` 后 `git diff --exit-code -- desktop/src/mygo.ts` 无差异 | CI |
| AC-9 | `mygo build` 产出安装包 | CI（三平台矩阵） |
| AC-10 | 发布 workflow 把产物挂到 tag 的 Release | 打一个预发布 tag 验证 |

## 8. 约束与依赖

- **Go**：版本以 `go.mod` 为准；引入 MyGo 后会被抬到 `1.27.2`（MyGo v0.5.5 要求）。
- **MyGo `github.com/egoist/mygo` v0.5.5**：桌面框架；纯 Go、无 cgo。
- **前端**：TypeScript + Vite，包管理用 bun（与 MyGo 官方模板一致）。
- **CLI**：`github.com/urfave/cli/v3`（不变）。
- **平台**：macOS / Windows / Linux。
  - macOS：dmg + `.app`；签名与公证需要 macOS runner 与证书。
  - Windows：exe + NSIS 安装器；`mygo build` 会加 `-H=windowsgui`。
  - Linux：tar.gz（含 `install.sh`）+ deb；**运行时需要 GTK 与 WebKitGTK**（仅桌面模式）。

## 9. 风险

| 风险 | 影响 | 应对 |
| --- | --- | --- |
| Windows GUI 子系统导致 CLI 无输出 | CLI 在 Windows 上「哑掉」 | `AttachConsole` 垫片（FR-1.3），并列为验收项 AC-2 |
| 前端嵌入依赖 `mygo build` 的 overlay 契约 | 普通 `go build` 的产物桌面模式打不开页面 | 文档明确「打包/开发用 `mygo build`」；发布链路只用 `mygo build` |
| Linux 桌面包把 GTK/WebKitGTK 变成安装依赖 | CLI-only 用户被牵连装 GUI 库 | 在 README/features 说明；如不可接受，退回「两个二进制」方案（见 design-decisions 备选） |
| MyGo 上游 API 变动 | 编译失败 | 版本固定 v0.5.5；升级按 `mygo-maintenance` 流程走 |
| 拖拽路径在部分平台拿不到 | 拖拽不可用 | 保留对话框入口为主路径 |
| 发布链路从 GoReleaser 切到 `mygo build` | 产物形状变化（无 rpm/checksums） | 记入 CHANGELOG 与 backlog 跟进 |

## 10. 里程碑

| 阶段 | 内容 | 出口条件 |
| --- | --- | --- |
| M1 复用层 | `internal/app` 暴露 `Scan` 与 `OnFile` | 行为不变的测试全绿 |
| M2 桌面应用 | `internal/desktop` + 双模式入口 + 前端 | 本地 `mygo dev` 可完成一次转换 |
| M3 工程化 | Makefile / CI / 发布 / 文档 | CI 全绿，`mygo build` 出包 |
| M4 交付 | 分支 + 两个提交 + PR | 检查清单勾满 |

详见 [phased-plan.md](../03-implementation/phased-plan.md)。
