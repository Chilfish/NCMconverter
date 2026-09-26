# NCMconverter 文档索引

> **本文档是项目文档的唯一入口。** AI 和开发者在处理任何任务前，先阅读本文档了解文档全局布局，再根据任务类型按需读取对应的领域文档。

---

## 一、项目概述

NCMconverter — 把网易云音乐的 `.ncm` 容器转换成可播放的 mp3 或 flac，并保留其中的元数据、封面与歌词。

- **语言**：Go（版本以 [`go.mod`](../go.mod) 的 `go` 指令为准）｜ **CLI**：`urfave/cli/v3`
- **容器解析**：直接解析 `.ncm` 字节布局（最初参考 [yoki123/ncmdump](https://github.com/yoki123/ncmdump)）
- **音频解密**：AES-128-ECB + XOR 掩码 + 256 字节密钥盒反混淆
- **标签写入**：mp3 用 `bogem/id3v2`（ID3v2.4 + UTF-8）、flac 用 `go-flac/v2`（Vorbis comment + picture）
- **并发**：`golang.org/x/sync/errgroup`（`SetLimit` 限流）
- **发布**：GoReleaser 交叉编译 linux/darwin/windows × amd64/arm64，附 deb/rpm
- **验证**：`go test` 三层（单元 / 合成容器端到端 / 真实 fixture 端到端），合成容器由独立的 `internal/ncmtest` 构造

> ⚠️ **未校验字段**：容器 `metaEnd` 处的 4 字节并**不是** meta 段的 CRC32（实测与 `hash/crc32` 三种算法都对不上），因此**故意不校验**；`metaEnd+5` 处的 reserved 字段实测等于 cover 长度。详见 [`planning/project-architecture.md`](planning/project-architecture.md) §2 与 [`features/container/container-format.md`](features/container/container-format.md)。
> ⚠️ **真实素材版权**：`testdata/Ave Mujica - KINGS (Cover).ncm` 是商业歌曲的翻唱，仅作回归锚点，经 `.gitignore` 的 `!testdata/*` 例外提交进公开仓库。来源与处置见 [`../testdata/README.md`](../testdata/README.md)。

---

## 二、文档目录结构

```
docs/
├── INDEX.md                       # 本文档（唯一入口）
├── features/                      # 功能文档，按子系统组织
│   ├── container/                 #   .ncm 容器解析（container-format.md）
│   ├── converter/                 #   解密与反混淆（decryption.md）
│   ├── metadata/                  #   元数据模型与解码（metadata.md）
│   ├── tagging/                   #   标签写入（tag.md · mp3-id3.md · flac-vorbis.md）
│   ├── lyrics/                    #   歌词侧车（lyrics.md）
│   ├── cli/                       #   命令行与编排（cli-and-orchestration.md）
│   └── release/                   #   发布产物（release-artifacts.md）
├── planning/                      # 规划与决策
│   ├── architecture.md            #   架构决策记录（ADR-001~009）
│   ├── project-architecture.md    #   系统架构总览
│   └── backlog.md                 #   未决任务清单
├── engineering/                   # 工程规范
│   ├── code-style.md              #   Go 代码规范与门禁
│   ├── git-workflow.md            #   分支/提交/PR/发布流程
│   ├── testing.md                 #   测试策略与验证映射
│   └── release-checklist.md       #   发布前检查清单
├── postmortem/                    # 尸检报告（写码前必读，README + 12 篇）
├── reviews/                       # 代码审查记录（review-YYYY-MM-DD-<主题>.md）
├── development-log/               # 开发日志（按天记录 YYYY-MM-DD.md）
├── requirements/                  # 复杂功能需求文档（PRD / 用户故事）
└── archive/                       # 历史规划归档（TODO.md 等，不主动读取）
```

---

## 三、文档导航 — 什么时候读哪个文档

> **核心原则**：根据任务涉及的代码范围按需读取，不要一次性全读。

### 按功能领域

| 任务场景                              | 必读文档                                                                                                     | 说明                                                  |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------- |
| **修改容器字节布局 / 段偏移**         | [`features/container/container-format.md`](features/container/container-format.md)                           | 偏移表、链式偏移、magic 校验、未校验字段的来龙去脉    |
| **修改解密 / 反混淆 / 格式判定**      | [`features/converter/decryption.md`](features/converter/decryption.md)                                       | 三步解密、密钥盒、`0x8000` 块边界、声明优先规则       |
| **修改元数据模型 / Int64 解码**       | [`features/metadata/metadata.md`](features/metadata/metadata.md)                                             | `Meta`/`Album`/`Artist`、数字与字符串双形态           |
| **修改 mp3 标签写入**                 | [`features/tagging/mp3-id3.md`](features/tagging/mp3-id3.md)                                                 | ID3v2.4+UTF-8、`USLT`/`APIC`/`TPE1`、上游陷阱         |
| **修改 flac 标签写入**                | [`features/tagging/flac-vorbis.md`](features/tagging/flac-vorbis.md)                                         | Vorbis comment 复用、自定义 `LYRICS`、picture、句柄   |
| **修改标签门面 / 封面策略**           | [`features/tagging/tag.md`](features/tagging/tag.md)                                                         | `Tagger` 接口语义、封面三级策略、错误处理             |
| **修改歌词读取与嵌入**                | [`features/lyrics/lyrics.md`](features/lyrics/lyrics.md)                                                     | `.lrc` 侧车、BOM/GBK、原样嵌入、超大文件              |
| **修改 CLI 参数 / 编排 / 输出命名**   | [`features/cli/cli-and-orchestration.md`](features/cli/cli-and-orchestration.md)                             | flag 表、`--depth` 语义、并发与失败聚合、路径逃逸防护 |
| **修改构建 / 发布 / CI**              | [`features/release/release-artifacts.md`](features/release/release-artifacts.md) + `../Makefile`             | GoReleaser、平台矩阵、deb/rpm、版本注入               |
| **架构级变更前**                      | [`planning/architecture.md`](planning/architecture.md) + [`planning/project-architecture.md`](planning/project-architecture.md) | 先读 ADR 历史决策，再对照架构总览                     |
| **规划下一阶段任务**                  | [`planning/backlog.md`](planning/backlog.md)                                                                 | 未决任务清单                                          |
| **测试 / 验证体系**                   | [`engineering/testing.md`](engineering/testing.md)                                                           | 三层测试、`ncmtest` 哲学、行为→测试映射表             |
| **写码前防复现 / 新 Bug 模式**        | [`postmortem/README.md`](postmortem/README.md)                                                               | 高频雷区自查；新 Bug 模式写 postmortem                |
| **每次 commit / PR**                  | [`engineering/git-workflow.md`](engineering/git-workflow.md)                                                 | 分支模型、Conventional Commits、审查清单              |
| **里程碑发布前**                      | [`engineering/release-checklist.md`](engineering/release-checklist.md)                                       | 本地门禁 + 快照试产 + 发布前已知尾巴                  |
| **了解历史决策 / 已知问题**           | [`reviews/`](reviews/) + [`development-log/`](development-log/) + [`archive/`](archive/)                     | 审查记录、按天日志、已完成阶段存档                    |

### 按文档类型速查

| 需要什么                     | 去哪里                                                     |
| ---------------------------- | ---------------------------------------------------------- |
| 系统架构总览（模块/数据流）  | [`planning/project-architecture.md`](planning/project-architecture.md) |
| 架构决策记录（ADR）          | [`planning/architecture.md`](planning/architecture.md)     |
| 未决任务清单（backlog）      | [`planning/backlog.md`](planning/backlog.md)               |
| Go 代码规范                  | [`engineering/code-style.md`](engineering/code-style.md)   |
| Git/Commit/PR 流程           | [`engineering/git-workflow.md`](engineering/git-workflow.md) |
| 测试与验证                   | [`engineering/testing.md`](engineering/testing.md)         |
| 发布检查清单                 | [`engineering/release-checklist.md`](engineering/release-checklist.md) |
| 尸检报告索引                 | [`postmortem/README.md`](postmortem/README.md)             |
| 代码审查记录                 | [`reviews/README.md`](reviews/README.md)                   |
| 开发日志（按天）             | [`development-log/README.md`](development-log/README.md)   |
| 已完成阶段的历史规划         | [`archive/README.md`](archive/README.md)（历史查阅）       |

---

## 四、常用开发命令

```sh
make build          # 编译到 bin/ncmconverter（按平台补 .exe）
make run ARGS="--help"
make test           # go test ./...
make test-race      # go test -race ./...（CI 门禁）
make coverage       # 写入 coverage.out
make vet            # go vet ./...
make fmt            # gofmt -w .
make fmt-check      # 只检查，不修改
make lint           # 需要 golangci-lint v2（CI 锁定 v2.14.0）
make clean          # 只删构建产物，不动音频与容器
```

其余命令的定义见 [`../Makefile`](../Makefile)，CI 步骤见 [`../.github/workflows/ci.yml`](../.github/workflows/ci.yml)。

---

## 五、文档体系约定（流程与反馈闭环）

> 文档按「规划 → 工程 → 功能 → 反馈」组织，历史教训固化为规则，防止同类问题复现。

### 文档生命周期

```
需求/规划   docs/requirements/ · docs/planning/（architecture ADR · project-architecture · backlog）
工程规范    docs/engineering/（code-style · git-workflow · testing · release-checklist）
功能文档    docs/features/<子系统>/（container · converter · metadata · tagging · lyrics · cli · release）
反馈闭环    docs/postmortem/ · docs/reviews/ · docs/development-log/
存档        docs/archive/（已完成阶段的历史记录，不主动读取）
```

### 何时写入

| 事件         | 动作                                                                              |
| ------------ | --------------------------------------------------------------------------------- |
| 新 Bug 模式  | 写 `docs/postmortem/0NN-*.md`（模板 `TEMPLATE.md`），预防项同步 `engineering/`    |
| 每次代码审查 | 记录到 `docs/reviews/review-YYYY-MM-DD-<主题>.md`                                 |
| 每天收尾     | 记入 `docs/development-log/YYYY-MM-DD.md`                                         |
| 架构级决策   | 以 ADR 记录到 `docs/planning/architecture.md`                                     |
| 阶段计划完成 | 计划文档 `git mv` 到 `docs/archive/`                                              |
| 里程碑发布   | 走 `docs/engineering/release-checklist.md`                                        |

### 文档语言

所有 `docs/` 文档用**中文**；代码注释与提交信息用**英文**（根级 `README.md` / `CHANGELOG.md` / `CONTRIBUTING.md` 等面向用户文档用中文）。

### 验证门禁（commit / PR 前）

`make fmt-check` → `make vet` → `make test-race` → `make build`；CI 另跑 `make coverage` 与 `golangci-lint`。详见 [`engineering/git-workflow.md`](engineering/git-workflow.md)。

---

## 六、根目录文档

| 文档                                           | 说明                             |
| ---------------------------------------------- | -------------------------------- |
| [../README.md](../README.md)                   | 项目介绍、安装、使用、开发       |
| [../CHANGELOG.md](../CHANGELOG.md)             | 变更日志（Keep a Changelog）     |
| [../CONTRIBUTING.md](../CONTRIBUTING.md)       | 贡献指南（环境、测试、提交约定） |
| [../SECURITY.md](../SECURITY.md)               | 安全政策与漏洞报告渠道           |
| [../CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md) | 行为准则（Contributor Covenant） |
| [../LICENSE](../LICENSE)                       | MIT License                      |
