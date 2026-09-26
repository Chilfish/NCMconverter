# 架构决策记录（ADR）

> 记录关键架构决策及其理由。格式：日期 / 决策 / 背景 / 理由 / 后果。
> 系统总览见 [project-architecture.md](project-architecture.md)；决策中涉及的代码细节见 [../features/](../features/)。

## 技术栈总览

| 层       | 技术                                                             |
| -------- | ---------------------------------------------------------------- |
| 语言     | Go（版本以 [`../../go.mod`](../../go.mod) 为准）                  |
| CLI      | `github.com/urfave/cli/v3`                                       |
| 并发     | `golang.org/x/sync/errgroup`                                     |
| 编码     | `golang.org/x/text`（GBK）                                       |
| mp3 标签 | `github.com/bogem/id3v2`                                         |
| flac 标签| `github.com/go-flac/go-flac/v2` + `flacvorbis/v2` + `flacpicture/v2` |
| 发布     | GoReleaser（+ nfpm）                                             |
| 测试     | 标准 `go test`（含 `-race`）                                     |

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

## ADR-008: GoReleaser + nfpm 发布平台矩阵

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
