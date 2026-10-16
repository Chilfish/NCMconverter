# 构建与发布产物

> 子系统：构建/发布｜ 关键文件：[`Makefile`](../../../Makefile)、[`mygo.json`](../../../mygo.json)、[`.github/workflows/`](../../../.github/workflows/)

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

桌面另有 `make dev`（`go tool mygo dev`）与 `make desktop`（`go tool mygo build`）：两者把 MyGo CLI 作为本模块的 Go tool 运行（`go.mod` 的 `tool` 指令），**不需要 bun，与上面的 Go 门禁刻意分离**。

## 2. 发布产物（`mygo build`）

桌面应用与 CLI 共用一个二进制，产物必须**给 Windows 设置 GUI 子系统并打包各平台安装器**（普通 `go build` 做不到，差别见 [构建契约](../desktop/desktop-app.md#51-make-build-与打包产物的差别)）。因此发布链路由 `mygo build` 驱动，取代了此前的 GoReleaser + nfpm（[ADR-010](../../planning/architecture.md)、[DD-4](../../requirements/desktop/05-review/design-decisions.md)）；`.goreleaser.yaml` 已删除。

配置在 [`mygo.json`](../../../mygo.json)（纯数据：`name`/`identifier`/`version`/`main`/`out`），读取它**不需要 JS 运行时**。**没有前端要构建或内嵌**：应用自己用 MyGo 原生 UI（`github.com/egoist/mygo/ui`）在 GPU 上绘制界面（[ADR-011](../../planning/architecture.md)）；`mygo build` 编译应用并按目标打包。

平台矩阵与产物（[`.github/workflows/release.yml`](../../../.github/workflows/release.yml) 的 `build` job）：

| 目标 | runner | 平台 | 产物 |
| --- | --- | --- | --- |
| macOS | `macos-latest` | `darwin/universal` | `.app` + `dmg` |
| Windows | `ubuntu-latest` | `windows/amd64,windows/arm64` | `exe` + NSIS 安装器（`*Setup*.exe`） |
| Linux | `ubuntu-latest` | `linux/amd64,linux/arm64` | `tar.gz`（含 `install.sh`）+ `deb` |

- MyGo 无 cgo，任意机器都能交叉编译任意平台；只有 macOS 的 `.app`、签名与磁盘镜像需要 Mac，Linux 目标上的 NSIS 由 `apt-get install nsis` 提供。
- **签名**：[`mygo.json`](../../../mygo.json) 未配置签名字段（CI 从不注入旧 `mygo.config.ts` 读取的那些环境变量，故有效行为不变）：macOS 走 ad-hoc（`-sign -`）签名、Windows 不签名，构建不被阻塞。将来的发布再补 `macos`/`windows` 段或 `-sign`。
- **不再产出的东西**：`rpm` 与 `checksums.txt`。GoReleaser/nfpm 退役后两者一并消失，恢复办法（发布 job 里补 `sha256sum` 与 rpm 步骤）记入 [backlog](../../planning/backlog.md)。
- 产物落在 `dist/<平台>/` 下（`out: "dist"`），由发布 job 用 `gh release upload` 挂到该 tag 的 Release。

> Linux 的 `deb` 只声明依赖 **GTK 3**（原生 UI 不再需要 WebKitGTK）；CLI 用户运行不受影响，但会被牵连安装 GTK。用户可读的变更说明仍以 [`CHANGELOG.md`](../../../CHANGELOG.md) 为准。

## 3. CI / 发布工作流

- **`.github/workflows/ci.yml`**（push `main` + PR）：
  - `test` job：三平台矩阵（`ubuntu`/`macos`/`windows`，`fail-fast: false`），Go 版本取自 `go.mod`；Windows 用 `choco install make` 补装；Linux 跑 `make fmt-check`，全部跑 `make vet`、`make ui-vet`（`go tool mygo vet`，另查原生 UI 的 build 生命周期）、`make test-race`、`make build`。
  - `coverage` job：`make coverage` 后上传 `coverage.out` artifact。
  - `desktop` job：三平台矩阵，`checkout` + `setup-go`（Windows 另加 `choco install make`）后跑 `make desktop`（`go tool mygo build`）证明每个平台都能出包；**无 `setup-bun`、无 `bun install`、无 `make typecheck`、无生成物 diff 校验**。
  - `lint` job：走 `golangci-lint-action`，**锁定 `version: v2.14.0`**（新版本不能自行改红绿），本地跑的 golangci-lint 需同步该版本。
- **`.github/workflows/release.yml`**（推 `v*` tag）：先由 `draft` job 用 `gh release create --draft --verify-tag` 建一个草稿 Release，再由 `build` job 的三平台矩阵各自 `go tool mygo build -platform …`（无 `setup-bun`、无 `bun install`），最后用 `gh release upload --clobber` 把 `dist/*/*.{dmg,Setup*.exe,deb,tar.gz}` 挂上去；`GITHUB_TOKEN` 来自 workflow 内的 `github.token`。重跑失败的 job 会把文件补进同一个草稿。
- **`.github/dependabot.yml`**：`gomod` 与 `github-actions` 各自每周，且都用 `groups` 合并成一个 PR。

## 4. 发布流程

见 [`engineering/git-workflow.md`](../../engineering/git-workflow.md) 与 [`engineering/release-checklist.md`](../../engineering/release-checklist.md)：更新 `CHANGELOG.md` → 本地过门禁 → `mygo build` 试产（`go tool mygo build`）→ 打 `v*` tag → GitHub Actions 发布。

## 相关文档

- [`engineering/release-checklist.md`](../../engineering/release-checklist.md)
- [`engineering/git-workflow.md`](../../engineering/git-workflow.md)
- [`features/desktop/desktop-app.md`](../desktop/desktop-app.md) —— 构建契约与平台差异
- [`CHANGELOG.md`](../../../CHANGELOG.md)
