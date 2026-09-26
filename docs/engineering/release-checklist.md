# Release Checklist

> 目标：确认一次发布满足标准。
> 执行环境：本地 `make` + GoReleaser 快照，最终由 `v*` tag 触发 CI 发布。

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
- [ ] 对照 [`../postmortem/README.md`](../postmortem/README.md) 做一次「高频雷区」自查：本次改动的文件是否落在历史热点上

## 发布产物试产（GoReleaser 快照）

- [ ] `goreleaser release --snapshot --clean` 成功
- [ ] 归档齐全：`linux`/`darwin`/`windows` × `amd64`/`arm64`，Windows 为 `zip`
- [ ] `deb`/`rpm` 生成，二进制路径为 `/usr/bin/ncmconverter`，文档落在 `/usr/share/doc/ncmconverter/`
- [ ] `checksums.txt` 存在
- [ ] 抽验一个归档：解压后二进制能跑 `--version`，且 commit/date 已注入

## CHANGELOG 与文档

- [ ] `CHANGELOG.md` 的 Unreleased 归入新版本并补日期
- [ ] 用户可见变化同步到 `README.md`（参数/行为）
- [ ] 行为变化已更新 `docs/`（features / planning / engineering）
- [ ] 新踩的坑写成 `docs/postmortem/0NN-*.md`，并更新 postmortem 索引与「高频雷区」

## 打 tag 与发布

- [ ] 合并到 `main` 后再打 tag：`git tag vX.Y.Z && git push origin vX.Y.Z`
- [ ] `release.yml` 运行成功，Release 正文按类型分组（chore/ci/docs/test/build/refactor 与 `(release)` 已过滤）
- [ ] Release 正文回链 `CHANGELOG.md`

## 功能冒烟（发布产物）

- [ ] 单个 `.ncm` 转换：产物可播放，扩展名取自实际格式
- [ ] 目录 + `--depth`：层级符合预期
- [ ] `--tag=false`：产物不带标签
- [ ] 有 `.lrc` 的样本：mp3 有 `USLT`、flac 有 `LYRICS`
- [ ] `--dry-run` 不写盘、`--skip-existing` 不动已有产物
- [ ] `--output-template` 按模板命名；含分隔符的元数据不会写到输出目录之外
- [ ] 坏容器与好容器同批：坏容器被记录、好容器仍转换、进程非零退出

## 发布前已知尾巴（确认可接受再发）

- [ ] **测试 fixture 版权**：`testdata/Ave Mujica - KINGS (Cover).ncm` 是商业歌曲翻唱，会随 `!testdata/*` 例外进公开仓库——确认可接受，或改走按需下载。
- [ ] **`reserved` 字段语义未证实**：`metaEnd+5` 实测等于 cover 长度，格式若变是风险点。
- [ ] **上游库陷阱**：`go-flac` 的 `ParseFile` 句柄泄漏、`id3v2` 的临时文件残留（均已绕开，但仍是隐患）。

> 详情见 [`../planning/backlog.md`](../planning/backlog.md) 与 [postmortem 012](../postmortem/012-upstream-contract-traps.md)。
