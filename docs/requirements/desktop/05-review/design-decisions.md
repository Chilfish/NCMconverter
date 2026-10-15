# 设计决策记录：桌面应用

> 关联：[PRD](../01-requirements/PRD.md)｜[设计](../02-design/architecture.md)
> 形式：决策 / 背景 / 备选 / 结论 / 后果。架构级决策另见 [ADR-010](../../../planning/architecture.md)。

## DD-1 一个二进制，还是两个？

**决策**：**一个**二进制。无参数 → 桌面；有参数 → CLI。

**背景**：用户明确要求「同一个执行文件，既能打开为 desktop，也能作为 shell 调用」。

**备选**

| 方案 | 优点 | 缺点 |
| --- | --- | --- |
| A. 单二进制双模式（采纳） | 用户的诉求；一份代码一个产物；无需额外分发 | Windows 需要 `AttachConsole` 垫片；CLI 用户多背体积与 Linux GUI 依赖 |
| B. 两个二进制（`ncmconverter` + `ncmconverter-desktop`） | CLI 保持极简；依赖干净 | 同名不同质的两类产物，用户困惑；发布链路要维护两套 |
| C. 只做桌面，CLI 退役 | 最简单 | 破坏脚本用户，违反「不改 CLI 语义」 |

**结论**：A。可行性来自 MyGo 是纯 Go 包——`os.Args` 分支发生在 `App.Run()` 之前，CLI 路径不触碰任何 MyGo API。

**后果**：Windows 需要 `console_windows.go`；Linux deb 会带上 GTK/WebKitGTK 依赖；二进制变大。三项均记入 [PRD §9](../01-requirements/PRD.md) 与 backlog。

## DD-2 进度从哪里来：解析日志，还是加回调？

**决策**：在 `internal/app` 加 **`Options.OnFile` 回调**（并导出 `Scan`）。

**背景**：`app.Run` 只把每文件结果写进 `slog`，界面拿不到结构化进度。

**备选**

| 方案 | 优点 | 缺点 |
| --- | --- | --- |
| A. 自定义 `slog.Handler` 捕获并解析日志（原型里试过） | 零改动 `internal/app` | 与日志文案强耦合（文案改了就静默失准）；拿不到「计划总数」；错误分类要靠字符串匹配 |
| B. 桌面自己重写 `convert`/`outputPath`/`lyrics` | 不改 `internal/app` | **两套转换逻辑必然漂移**，与「日志/退出码/边界行为一致」的承诺冲突 |
| C. `Options.OnFile` 回调（采纳） | 单点、纯 Go、类型安全；CLI 行为不变；能拿到 `Dest` | `internal/app` 需要一处附加式改动 |

**结论**：C。回调**在既有互斥锁内**调用，天然串行；`Run` 的签名与返回错误不变，CLI 与既有测试不受影响。

**后果**：`app` 仍 UI 无关（不引入 mygo）；桌面侧把 `FileResult` 映射成 `Report` 推给页面。

## DD-3 界面用 web 前端，还是 MyGo 原生 UI？

**决策**：**web 前端**（TypeScript + Vite），不用 MyGo 的 `package ui`。

**备选**

| 方案 | 优点 | 缺点 |
| --- | --- | --- |
| A. web 前端（采纳） | 表单/列表/日志这类界面用 HTML 写最快；已有可用实现；热更新开发体验好 | 需要 WebView 运行时（Linux 的 WebKitGTK）；打包体积略大 |
| B. 原生 UI | 无 WebView 依赖、启动更快、内存更小 | 手写列表/表单/进度条成本高；无现成实现；迭代慢 |

**结论**：A。本应用的界面是典型的「表单 + 列表 + 日志」，web 技术优势明显；`desktop/` 与 Go 代码的契约由 `mygo generate` 保证类型安全。

**后果**：Linux 桌面模式需要 GTK/WebKitGTK（CLI 模式不需要）。

## DD-4 发布链路：`mygo build` 还是 GoReleaser？

**决策**：发布改由 **`mygo build`** 驱动（矩阵 job），`GoReleaser` + `nfpm` 退役。

**背景**：`mygo build` 用 `go build -overlay` 把前端注入主包，并设置 Windows GUI 子系统；普通 `go build`（GoReleaser 走的路径）产出的二进制**桌面打不开页面**。若保留 GoReleaser，就会出现「同名的 CLI 产物」与「桌面产物」并存。

**备选**

| 方案 | 优点 | 缺点 |
| --- | --- | --- |
| A. `mygo build` 驱动发布（采纳） | 单一产物家族、产物形状贴合 GUI 应用（dmg/安装器/deb/`install.sh`） | 失去 rpm 与 `checksums.txt` |
| B. GoReleaser 保留 + 另加 `mygo build` job | 改动最小 | 两族同名产物，用户无法分辨 |
| C. GoReleaser 产出双模式二进制 | 保住 nfpm/rpm | 要在本仓库复刻 mygo 的 overlay 契约（耦合上游内部实现），维护负担高 |

**结论**：A。rpm 与 checksums 作为跟进项记入 backlog（发布 job 里补 `sha256sum` 与 rpm 都很直接）。

**后果**：`ADR-008` 被 `ADR-010` 取代；`docs/features/release/release-artifacts.md` 与 release checklist 需同步。

## DD-5 复用层改动怎么提？（过程决策）

**决策**：`internal/app` 的改动（导出 `Scan` + `OnFile` 回调）与桌面应用**并入同一个 PR**，但保持两个独立提交（先 `refactor:` 后 `feat:`）。

**背景**：`git-workflow.md` 要求「顺带的重构拆成单独 PR」。

**理由**：该改动与桌面强耦合、规模很小（一个新文件 + 几处签名），单独开 PR 会产生「先合一个没有调用方的回调」的空窗。已在 PR 描述中说明。

**后果**：审阅时按两个提交分别看；`refactor` 提交单独跑门禁也必须全绿（行为不变）。

## DD-6 打开被仓库否决过的方向（GUI）

**决策**：撤销 `backlog.md` 中「GUI / 常驻服务 → 删除（不接）」的裁决。

**理由**：

1. 定位从「批处理 CLI」扩展为「面向普通用户的转换器」，与既有的「不联网、不转码」裁决不冲突。
2. 桌面外壳**不产生第二套转换逻辑**（DD-2），维护成本可控。
3. 单二进制方案不牺牲脚本用户（DD-1）；MyGo 无 cgo，不引入 Node/Electron 运行时。

**保留的部分**：不做常驻/托盘/后台监听（仍不接），不做文件关联与自更新（转入 backlog 而非删除）。

## DD-7 明确不做（本轮）

| 项 | 裁决 | 理由 |
| --- | --- | --- |
| `.ncm` 文件关联（双击打开） | 延后（backlog） | 需要在页面就绪前缓冲启动参数，收益小于拖拽；先把主线做稳 |
| 自更新（`mygo.Updater`） | 延后（backlog） | 需要密钥体系与更新托管；与本次「单二进制双模式」正交 |
| 托盘常驻 / 目录监听自动转换 | 不接 | 与「批处理/整理」定位不符 |
| 界面内播放音频 | 不接 | 超出定位 |
