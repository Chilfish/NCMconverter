# 分阶段实施计划

> 关联：[PRD](../01-requirements/PRD.md)｜[设计](../02-design/architecture.md)｜[API 契约](../02-design/api-contract.md)｜[测试计划](../04-testing/test-plan.md)
> 前置参考实现：工作区 `I:\dev\mygo-app`（**已在 Windows 跑通**：窗口、拖拽入队、逐文件进度、取消、打开输出目录、中文界面）。本文标注「可搬运」的文件可直接对照搬运，其余按上游结构重写。

## 0. 环境准备

```sh
go version            # 需 1.27+（引入 mygo 后会要求 1.27.1，以 go.mod 为准）
bun --version         # 前端与 mygo CLI 的运行时
```

依赖：`github.com/egoist/mygo v0.5.1`（纯 Go，无 cgo）；前端 `mygo-runtime`、`mygo-cli`、`typescript`、`vite`（版本与 MyGo 官方模板一致）。

**开工前先读**（按 [`../../INDEX.md`](../../INDEX.md) 的导航）：

- [`../../engineering/code-style.md`](../../engineering/code-style.md)、[`../../engineering/testing.md`](../../engineering/testing.md)、[`../../engineering/git-workflow.md`](../../engineering/git-workflow.md)
- [`../../postmortem/README.md`](../../postmortem/README.md)（高频雷区自查）
- 本目录的 [编码规范](coding-standards.md) 与 [设计](../02-design/architecture.md)

---

## 阶段 1 — 复用层（提交 ①，`refactor:`，行为不变）

**目标**：让 `internal/app` 能被第二个调用方使用，且 CLI 行为、日志、退出码、测试全不变。

| # | 文件 | 改动 |
| --- | --- | --- |
| 1.1 | `internal/app/report.go`（新） | `Outcome` 常量（planned/converted/skipped/failed）与 `FileResult{Source,Dest,Outcome,Err}`；导出标识符写文档注释 |
| 1.2 | `internal/app/app.go` | `Options` 增加 `OnFile func(FileResult)`；`collect` → 导出 `Scan`（同一实现，补文档注释）；`Run` 在**已持锁的临界区内**调用 `OnFile`（失败/跳过/成功三分支），试运行分支逐文件回调 `OutcomePlanned`；`Run` 签名不变 |
| 1.3 | `internal/app/convert.go` | `convert` 返回 `(FileResult, error)`：算出 `outputPath` 后填 `Dest`，跳过分支填 `OutcomeSkipped`，写出成功填 `OutcomeConverted`；**保留全部 `slog` 调用** |
| 1.4 | `internal/app/app_test.go` | 两处 `collect(` → `Scan(` |
| 1.5 | `internal/app/report_test.go`（新） | 覆盖 `OnFile`：试运行 → `OutcomePlanned`；成功 → `OutcomeConverted` 且 `Dest` 正确；`SkipExisting` → `OutcomeSkipped`；坏容器 → `OutcomeFailed` 且 `Err` 非空。复用 `integration_test.go` 的 `container`/`mp3Container`/`frames`/`readFile` 夹具（夹具走 `internal/ncmtest`，见 [ADR-003]） |

**出口**：`make fmt-check vet test-race build` 全绿，且**未改动任何用户可见行为**。

> 约定说明：`git-workflow.md` 说「顺带的重构拆成单独 PR」。本功能的复用层改动与桌面强耦合，按用户决定并入同一个 PR，但在 PR 描述里说明，并保持「先 refactor、后 feat」两个独立提交。

---

## 阶段 2 — 桌面应用（提交 ②，`feat:`）

### 2.1 `internal/desktop`（新包）

| # | 文件 | 内容 |
| --- | --- | --- |
| 2.1.1 | `service.go` | `Info/Request/Report/Result` DTO（带 json tag）+ `Converter` 服务（`Info`、`PickFiles`、`PickDirectory`、`PickOutput`、`Scan`、`Run`、`Cancel`、`OpenPath`）+ `Opened` 事件 + `pickPaths`/`firstPath` 助手。**指针接收者** |
| 2.1.2 | `app.go` | `Run() error`：`SetName` → `Bind(&Converter{})` → `WhenReady`（`SetMenu(newMenu())` + `openWindow()`）→ `OnWindowAllClosed`/`OnActivate` → `App.Run()`；窗口与菜单按 [设计 §5.3](../02-design/architecture.md) |
| 2.1.3 | `service_test.go` | 见 [测试计划](../04-testing/test-plan.md)：用 `ncmtest` 造容器，零值 `&mygo.Channel[Report]{}` 直接调 `Run`，断言计数/落盘/`Scan`/`Cancel` 归一 |

**注意**：`internal/desktop` 会导入 mygo，因此 `cmd/ncmconverter` 也链接 mygo——这是单二进制双模式的必然结果，不是缺陷。

### 2.2 双模式入口

| # | 文件 | 内容 |
| --- | --- | --- |
| 2.2.1 | `cmd/ncmconverter/main.go` | `main()` 只做分支（有参数且非 `MYGO_GENERATE` → CLI；否则桌面）；现有主体原样搬进 `runCLI(args []string)`，**参数、默认值、日志、退出码、`version/commit/date` 注入均不变** |
| 2.2.2 | `cmd/ncmconverter/console_windows.go`（新） | `attachConsole()`：`syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole").Call(-1)`，成功后重开 `CONOUT$`/`CONIN$`；失败静默返回 |
| 2.2.3 | `cmd/ncmconverter/console_other.go`（新） | `//go:build !windows`，空实现 |

### 2.3 前端（仓库根配置 + `desktop/`）

可直接搬运自 `I:\dev\mygo-app`，并做两处对齐：把页面调用的方法名保持为 `Converter.*`（与上游服务名一致），`events.filesOpened` 的订阅保持一致。

| # | 文件 | 内容 |
| --- | --- | --- |
| 2.3.1 | `mygo.config.ts`（新） | 见 [设计 §6](../02-design/architecture.md)：`main: "./cmd/ncmconverter"`、`frontendDist: "desktop/dist"`、`bindings: "desktop/src/mygo.ts"`、`out: "dist"` |
| 2.3.2 | `package.json`（新） | scripts：`dev`/`build`/`generate`（mygo）、`dev:web`/`build:web`/`typecheck`（vite + tsc）；依赖同上 |
| 2.3.3 | `tsconfig.json`（新） | `include: ["desktop/src"]` |
| 2.3.4 | `vite.config.ts`（新） | `root: "desktop"`，端口 5173 / `strictPort`，忽略 `.mygo/`、`dist/` |
| 2.3.5 | `desktop/index.html`（新，可搬运） | 队列 / 选项 / 转换（进度+汇总）/ 日志 / 页脚版本 |
| 2.3.6 | `desktop/src/main.ts`（新，可搬运） | `Channel<Report>` 收进度、`onFileDrop` 拖拽、`events.filesOpened`、运行/取消/打开输出目录 |
| 2.3.7 | `desktop/src/style.css`（新，可搬运） | 亮暗模式、面板/日志/进度条样式 |
| 2.3.8 | `desktop/src/mygo.ts` | **由 `bun run generate` 生成并提交**，不手写 |

### 2.4 构建辅助

| # | 文件 | 改动 |
| --- | --- | --- |
| 2.4.1 | `.gitignore` | 增加 `node_modules/`、`.mygo/`、`cmd/ncmconverter/mygo_windows_*.syso`（`dist/` 已在） |
| 2.4.2 | `Makefile` | 新增 `desktop`（`bun install --frozen-lockfile && bun run build`）、`generate`、`typecheck`；**现有 Go 目标保持无 bun 依赖** |

**出口**：`bun run dev` 能完成一次真实转换；`bin/ncmconverter --dry-run -d 1 <目录>` 输出与改动前一致。

---

## 阶段 3 — CI 与发布

### 3.1 `.github/workflows/ci.yml`（新增 job，不动现有 job）

```yaml
  desktop:
    name: Desktop (${{ matrix.os }})
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with: { go-version-file: go.mod }
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - run: make typecheck
      - name: Bindings are in sync
        if: runner.os == 'Linux'
        run: |
          make generate
          git diff --exit-code -- desktop/src/mygo.ts
      - run: make desktop        # mygo build：证明三平台都能打包
```

### 3.2 `.github/workflows/release.yml`（替换 GoReleaser）

- 删除 GoReleaser job；**删除 `.goreleaser.yaml`**（ADR-008 被取代）。
- 新 matrix：macOS（`darwin/universal` + dmg）、Windows（exe + NSIS 安装器）、Linux（`linux/amd64,linux/arm64` → tar.gz + deb）。
- 每个 job：`bun install --frozen-lockfile` → `bun run build` → 用 `gh release upload`（先 `gh release create --draft`）把产物挂到该 tag。
- 签名：`mygo.config.ts` 的 `macos.signingIdentity` / `macos.notarize` / `windows.certificate` 从 secrets 注入；缺失时 ad-hoc（`-sign -`）不阻塞。
- 说明：不再产出 rpm 与 `checksums.txt`（见 [backlog](../../../planning/backlog.md) 跟进项）。

### 3.3 依赖

- `go.mod` 新增 `github.com/egoist/mygo v0.5.1`；`go mod tidy` 会把 `go` 指令抬到 `1.27.1`。

---

## 阶段 4 — 文档（与代码同 PR）

| # | 文档 | 改动 |
| --- | --- | --- |
| 4.1 | `docs/planning/architecture.md` | 新增 **ADR-010**（桌面与 CLI 共用二进制）；在 ADR-008 标注「被 ADR-010 取代」；技术栈总览加 MyGo / Vite / bun |
| 4.2 | `docs/features/desktop/desktop-app.md`（新） | 领域文档：目录布局、双模式入口与 `AttachConsole`、`Converter.*` 表面、前端与生成物、`mygo dev`/`mygo build` 工作流、`make build`（无内嵌前端）与打包产物的差别、Linux 运行时依赖 |
| 4.3 | `docs/features/cli/cli-and-orchestration.md` | 补「无参数即桌面」的入口语义；`app.Scan`/`Options.OnFile` 的说明 |
| 4.4 | `docs/features/release/release-artifacts.md` | GoReleaser/nfpm 段落替换为 `mygo build` 矩阵与产物清单（含「无 rpm/checksums」的说明） |
| 4.5 | `docs/INDEX.md` | 功能导航表加 desktop 行；`docs/requirements/` 导航加本目录 |
| 4.6 | `docs/requirements/README.md` | 「现有功能级需求文档」表加 `desktop` |
| 4.7 | `docs/engineering/code-style.md` | 包组织表加 `internal/desktop`、`desktop/`；写明「`app` 不导入 mygo」 |
| 4.8 | `docs/engineering/testing.md` | 行为→测试映射补 `internal/app` 的 `OnFile` 与 `internal/desktop` 的服务测试 |
| 4.9 | `docs/engineering/release-checklist.md` | 门禁与试产换 `mygo build`；产物名与平台要求 |
| 4.10 | `docs/planning/backlog.md` | **反转**「GUI / 常驻服务（删除/不接）」裁决；新增跟进项：rpm/checksums 回归、`.ncm` 文件关联、自更新、Linux 依赖策略 |
| 4.11 | `README.md` | 增加「桌面应用」一节：双模式用法、`bun run dev`、打包产物、平台要求（Linux 需 GTK/WebKitGTK） |
| 4.12 | `CHANGELOG.md` | Unreleased：Added 桌面应用/单二进制双模式；Changed 发布链路改 `mygo build`（说明 rpm/checksums 不再产出） |
| 4.13 | `docs/development-log/2026-10-15.md`（新）+ `development-log/README.md` 时间线 | 按当天记录 |

---

## 阶段 5 — 验证、提交与 PR

### 5.1 本地门禁

```sh
make fmt-check vet test-race build
go test ./internal/... -race

bun install
bun run typecheck
bun run generate && git diff --exit-code -- desktop/src/mygo.ts
bun run build                    # mygo build：产出 dist/ 下的安装包
```

### 5.2 双模式冒烟（Windows 实测含控制台附加）

```sh
dist/<产物>/ncmconverter.exe --version                    # 应打印版本 + commit + 构建时间
dist/<产物>/ncmconverter.exe --dry-run -d 2 <样本目录>      # 与改动前逐字一致
dist/<产物>/ncmconverter.exe                              # 无参数：打开窗口
bin/ncmconverter.exe --help                               # make build 的产物，CLI 路径
```

界面冒烟：添加文件/文件夹、拖拽目录、跑一次真实转换（看进度与汇总）、取消、打开输出目录、试运行不写盘、掺坏容器看失败计数。

### 5.3 提交与 PR

```sh
git switch -c feat/desktop
git add internal/app
git commit   # refactor: report every converted file through a callback
git add -A
git commit   # feat: add a desktop app sharing the ncmconverter binary
git push -u origin feat/desktop
gh pr create --fill --title "feat: add a desktop app sharing the ncmconverter binary"
```

PR 描述按 [模板](../../../../.github/PULL_REQUEST_TEMPLATE.md)：做了什么 / 关联 issue / 检查清单；并说明「refactor 与 feat 并入一个 PR」的原因（已获授权）。

---

## 6. 交付物清单（自查）

- [ ] `internal/app`：`report.go`、`Options.OnFile`、`Scan`、`convert` 返回 `FileResult`、`OnFile` 测试
- [ ] `internal/desktop`：`service.go`、`app.go`、`service_test.go`
- [ ] `cmd/ncmconverter`：双模式 `main.go` + `console_{windows,other}.go`
- [ ] 根配置：`mygo.config.ts`、`package.json`、`tsconfig.json`、`vite.config.ts`、`.gitignore`
- [ ] `desktop/`：`index.html`、`src/{main.ts,style.css,mygo.ts}`
- [ ] 构建：`Makefile` 目标、CI desktop job、发布 workflow、删除 `.goreleaser.yaml`
- [ ] 文档：阶段 4 的 13 项
- [ ] 门禁全绿 + 双模式冒烟通过

## 7. 已知取舍与后续（同步进 backlog）

| 取舍 | 说明 |
| --- | --- |
| 单二进制体积 | CLI 用户多背几 MB，Linux deb 会依赖 GTK/WebKitGTK（仅桌面模式需要） |
| `make build` 无内嵌前端 | 普通 `go build` 不注入 overlay；文档明确「打包/开发用 `mygo build`」 |
| 放弃 rpm 与 `checksums.txt` | 换用 `mygo build` 的产物形状；如需回归，在发布 job 补 `sha256sum` 与 rpm 步骤 |
| 不做文件关联 / 自更新 | 留作 backlog；拖拽入队已覆盖主要场景 |
