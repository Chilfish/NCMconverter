# 构建与发布产物

> 子系统：构建/发布｜ 关键文件：[`Makefile`](../../../Makefile)、[`.goreleaser.yaml`](../../../.goreleaser.yaml)、[`.github/workflows/`](../../../.github/workflows/)

## 1. 本地构建（Makefile）

命名与 CMD 约定：可执行文件名与 `cmd/` 下的包目录同名（`ncmconverter`）；Windows 下自动补 `.exe`。

```make
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(CMD)
```

`LDFLAGS` 通过 `-X` 注入三个变量（`main.go` 里声明为默认 `dev`/`none`/`unknown`）：

| 变量      | 取值来源                                         |
| --------- | ------------------------------------------------ |
| `version` | `git describe --tags --always --dirty`，回退 `dev` |
| `commit`  | `git rev-parse HEAD`，回退 `none`                |
| `date`    | `git log -1 --format=%cI`，回退 `unknown`         |

`--version` 由 `main.go` 的 `cli.VersionPrinter` 打印成 `… (commit …, built …)`，方便定位正在运行的二进制（见 [postmortem 005 相关的版本可见性动因]，以及 `CHANGELOG.md`）。

其余目标：`test`、`test-race`、`coverage`（`-covermode=atomic -coverprofile=coverage.out`）、`vet`、`fmt`、`fmt-check`、`lint`（`golangci-lint run`）、`clean`（只删 `bin/` 与 `coverage.out`，**绝不删音频或容器**）。CI 复用这些目标，避免 workflow 与 Makefile 两处维护。

## 2. 发布产物（GoReleaser）

`.goreleaser.yaml`（`version: 2`）：

- **平台**：`linux`/`darwin`/`windows` × `amd64`/`arm64`，`CGO_ENABLED=0`，`-trimpath`。
- **ldflags**：`-s -w` 加三个 `-X`（与 Makefile 一致，取 `{{.Version}}`/`{{.Commit}}`/`{{.Date}}`）。
- **归档**：默认 `tar.gz`；**Windows 用 `zip`**（`format_overrides`）。归档附 `LICENSE` 与 `README.md`。
- **安装包（nfpm）**：Linux `amd64`/`arm64` 生成 `deb` 与 `rpm`，二进制装到 `/usr/bin`，文档装到 `/usr/share/doc/ncmconverter/`。
- **校验和**：`checksums.txt`。
- **快照版本**：`{{ incpatch .Version }}-next`。
- **发布说明**：正文是按 conventional-commit 类型分组的提交列表（Features / Bug fixes / Others），footer 回链 `CHANGELOG.md`；**过滤掉** `chore`/`ci`/`docs`/`test`/`build`/`refactor` 与任何 `(release)` 提交（包括带 scope 的形式）。

> 用户可读的变更说明以 `CHANGELOG.md` 为准，Release 正文只是提交清单。

## 3. CI / 发布工作流

- **`.github/workflows/ci.yml`**（push `main` + PR）：
  - `test` job：三平台矩阵（`ubuntu`/`macos`/`windows`，`fail-fast: false`），Go 版本取自 `go.mod`；Windows 用 `choco install make` 补装；Linux 跑 `make fmt-check`，全部跑 `make vet`、`make test-race`、`make build`。
  - `coverage` job：`make coverage` 后上传 `coverage.out` artifact。
  - `lint` job：走 `golangci-lint-action`，**锁定 `version: v2.14.0`**（新版本不能自行改红绿），本地跑的 golangci-lint 需同步该版本。
- **`.github/workflows/release.yml`**（推 `v*` tag）：`fetch-depth: 0` 取全历史 → `goreleaser release --clean`，`GITHUB_TOKEN` 来自 secrets。
- **`.github/dependabot.yml`**：`gomod` 与 `github-actions` 各自每周，且都用 `groups` 合并成一个 PR。

## 4. 发布流程

见 [`engineering/git-workflow.md`](../../engineering/git-workflow.md) 与 [`engineering/release-checklist.md`](../../engineering/release-checklist.md)：更新 `CHANGELOG.md` → 本地过门禁 → 快照试产 → 打 `v*` tag → GitHub Actions 发布。

## 相关文档

- [`engineering/release-checklist.md`](../../engineering/release-checklist.md)
- [`engineering/git-workflow.md`](../../engineering/git-workflow.md)
- [`../CHANGELOG.md`](../../../CHANGELOG.md)
