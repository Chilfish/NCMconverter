# Release Checklist

> 目标：确认一次发布满足标准。
> 执行环境：本地 `make`（Go 门禁）+ `mygo build`（打包试产），最终由 `v*` tag 触发 CI 发布。

## 版本纪律

- **版本号单源**：`CHANGELOG.md` 记录用户可见变化；二进制版本由 tag/`-ldflags` 注入，不手改多份。
- **SemVer**：不兼容变更 `MAJOR`、新功能 `MINOR`、兼容修复 `PATCH`。
- **tag 只打在门禁绿的 commit 上**；版本号严格递增，禁止回退。
- 破坏性变更必须在 `CHANGELOG.md` 写清（例如 `--depth 0` 的语义变更、模块路径小写化）。

## 本地门禁（tag 前）

- [ ] `make fmt-check` 无输出
- [ ] `make vet` 干净
- [ ] `make test-race` 全绿
- [ ] `make build` 成功
- [ ] `make lint` 干净（golangci-lint 版本与 CI 锁定的 `v2.14.0` 一致）
- [ ] `make typecheck` 干净；`make generate` 后 `git diff --exit-code -- desktop/src/mygo.ts` 无差异（客户端与 Go 同步）
- [ ] `make desktop` 能出包（`mygo build`，需要 bun）
- [ ] 对照 [`../postmortem/README.md`](../postmortem/README.md) 做一次「高频雷区」自查：本次改动的文件是否落在历史热点上

## 发布产物试产（`mygo build`）

- [ ] `bun install --frozen-lockfile && bun run build`（`make desktop`）成功；指定平台用 `bun run build -- -platform …`
- [ ] 产物齐全：macOS `.app` + `dmg`（`darwin/universal`）；Windows `exe` + NSIS 安装器（`windows/amd64,arm64`）；Linux `tar.gz`（含 `install.sh`）+ `deb`（`linux/amd64,arm64`）
- [ ] 缺签名密钥时不阻塞：macOS ad-hoc（`-sign -`）签名、Windows 不签名；产物落在 `dist/<平台>/`
- [ ] 平台要求：macOS 的 `.app`/签名/磁盘镜像需 Mac；Windows 安装器需 `nsis`（Linux 上 `apt-get install nsis`）；Linux 桌面模式运行时需 GTK/WebKitGTK
- [ ] 抽验一个产物：`ncmconverter --version` 能跑且 commit/date 已注入；**无参数启动能打开窗口**
- [ ] 已知缺口：**不再产出 `rpm` 与 `checksums.txt`**（确认可接受，见 [backlog](../planning/backlog.md)）

## CHANGELOG 与文档

- [ ] `CHANGELOG.md` 的 Unreleased 归入新版本并补日期
- [ ] 用户可见变化同步到 `README.md`（参数/行为）
- [ ] 行为变化已更新 `docs/`（features / planning / engineering）
- [ ] 新踩的坑写成 `docs/postmortem/0NN-*.md`，并更新 postmortem 索引与「高频雷区」

## 打 tag 与发布

- [ ] 合并到 `main` 后再打 tag：`git tag vX.Y.Z && git push origin vX.Y.Z`
- [ ] `release.yml` 运行成功：`draft` job 建草稿 Release，`build` 三平台矩阵把 dmg / NSIS 安装器 / deb / tar.gz 挂上去
- [ ] Release 正文与 `CHANGELOG.md` 一致（用户可见变化以 `CHANGELOG.md` 为准）

## 功能冒烟（发布产物）

- [ ] 单个 `.ncm` 转换：产物可播放，扩展名取自实际格式
- [ ] 目录 + `--depth`：层级符合预期
- [ ] `--tag=false`：产物不带标签
- [ ] 有 `.lrc` 的样本：mp3 有 `USLT`、flac 有 `LYRICS`
- [ ] `--dry-run` 不写盘、`--skip-existing` 不动已有产物
- [ ] `--output-template` 按模板命名；含分隔符的元数据不会写到输出目录之外
- [ ] 坏容器与好容器同批：坏容器被记录、好容器仍转换、进程非零退出
- [ ] CLI 路径：`ncmconverter --help` 正常（`make build` 的 `bin/` 产物）
- [ ] 桌面路径：无参数启动打开窗口；拖入目录能统计并完成一次转换；「取消」与「打开输出目录」可用
- [ ] Windows：**从终端**运行打包产物，`--version` 有输出（验证 `AttachConsole`）
- [ ] 桌面勾选「试运行」不写盘；坏容器在界面显示原因且不中断整批

## 发布前已知尾巴（确认可接受再发）

- [ ] **测试 fixture 版权**：`testdata/Ave Mujica - KINGS (Cover).ncm` 是商业歌曲翻唱，会随 `!testdata/*` 例外进公开仓库——确认可接受，或改走按需下载。
- [ ] **`reserved` 字段语义未证实**：`metaEnd+5` 实测等于 cover 长度，格式若变是风险点。
- [ ] **上游库陷阱**：`go-flac` 的 `ParseFile` 句柄泄漏、`id3v2` 的临时文件残留（均已绕开，但仍是隐患）。
- [ ] **不再产出 rpm 与 `checksums.txt`**：发布链路改由 `mygo build` 驱动后的产物形状变化，确认可接受（[backlog](../planning/backlog.md)）。

> 详情见 [`../planning/backlog.md`](../planning/backlog.md) 与 [postmortem 012](../postmortem/012-upstream-contract-traps.md)。
