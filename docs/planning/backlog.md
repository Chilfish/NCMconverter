# Backlog（未决任务清单）

**最后更新**: 2026-10-15

> 本清单**只保留当前未决的任务**，不累积已完成条目。历史完成记录见 [../archive/TODO.md](../archive/TODO.md)（v0.1.0 前的行动计划，已全部勾选或裁定）。

## 约定

- 每个条目：`- [ ] <主题>（关联文档：… / 前置：…）`
- 技术债/重构用 `[refactor]` 前缀；加固/健壮性用 `[hardening]` 前缀
- 需求变更需要文档跟进时，标注关联文档路径
- 裁决语义：**采纳** / **延后**（注明并入阶段）/ **删除**（进不做清单，附理由）

## 技术债与加固

- [ ] `[hardening]` **`reserved` 字段语义未证实**：布局中 `metaEnd+5` 处 4 字节实测与 cover 长度相同（真实样本两者同为 224893）。若将来格式有变，这里是风险点，值得找更多样本比对（关联：[../features/container/container-format.md](../features/container/container-format.md)）。
- [ ] `[refactor]` **上游库问题**：
  - `go-flac/v2` 的 `ParseFile` 在解析失败时**泄漏文件句柄**，项目已在 `internal/tag/flac/flac.go` 改用自持 `os.Open` + `ParseBytes` 绕开。建议向上游反馈并保留注释（关联：[postmortem 012](../postmortem/012-upstream-contract-traps.md)）。
  - `bogem/id3v2` 的 `Save()` 在写入失败时**不关闭 `-id3v2` 临时文件**，Windows 上会留下无法删除的残留。锁到 ID3v2.4 + UTF-8 后不再触发，但仍是隐患（关联：[../features/tagging/mp3-id3.md](../features/tagging/mp3-id3.md)）。
- [ ] `[hardening]` **覆盖率无阈值门禁**：CI 目前只上传 `coverage.out` artifact，未设最低覆盖率门槛。可评估是否引入阈值或 codecov（关联：[../engineering/testing.md](../engineering/testing.md)）。
- [ ] `[hardening]` **fixture 空跑守卫的推广**：`repositorySamples` 已做到「目录存在但无容器则失败」，可检查其它依赖外部素材的测试是否也需要同类守卫（关联：[postmortem 011](../postmortem/011-fixture-empty-run.md)）。

## 桌面应用与发布（ADR-010 之后）

- [ ] `[feat]` **`.ncm` 文件关联**：把 `.ncm` 关联到本应用，双击即打开桌面并加入队列。需要处理「页面就绪前到达的启动参数」（在 Go 侧缓冲、页面启动后取回），已从本次范围裁出（关联：[architecture.md §5](../requirements/desktop/02-design/architecture.md)）。
- [ ] `[feat]` **自更新**：接入 `mygo.Updater` 的 `updates` 配置（签名密钥、发布托管、delta）。与单二进制双模式正交（关联：[ADR-010](architecture.md)）。
- [ ] `[build]` **恢复 `checksums.txt` 与 rpm**：发布链路改由 `mygo build` 驱动后不再产出这两者。可在发布 job 里补 `sha256sum` 步骤与 rpm 生成（关联：[ADR-010](architecture.md)、[design-decisions DD-4](../requirements/desktop/05-review/design-decisions.md)）。
- [ ] `[hardening]` **Linux 桌面运行时依赖策略**：改用原生 UI 后（[ADR-011](architecture.md)），deb 只依赖 **GTK 3**（原生 UI 不再需要 WebKitGTK），顾虑大为减轻；但 GTK 仍会为 CLI-only 用户牵连安装，是否改 `Recommends` 或拆分产物仍待评估（关联：[ADR-011](architecture.md)、[ADR-010](architecture.md)、[PRD §9](../requirements/desktop/01-requirements/PRD.md)）。
- [ ] `[hardening]` **Windows 控制台垫片的回归**：`AttachConsole` 在 ConPTY/Windows Terminal 下的行为值得一次实机回归，并纳入发布 checklist 的功能冒烟（关联：[design §3.1](../requirements/desktop/02-design/architecture.md)）。已修一处：垫片改为只接管「没有自己句柄」的流，重定向（`ncmconverter --dry-run > list.txt`）不再被丢掉。
- [ ] `[build]` **打包产物的 `--version` 报 `dev`**：`mygo build` 不注入 `main.version`/`commit`/`date`，只把 `mygo.json` 的版本链进 MyGo 自己的元数据，所以发布产物的 CLI `--version` 显示 `dev (commit none, built unknown)`（`make build`/`go install` 的产物不受影响）。`mygo build` 没有 ldflags 透传，可选：用 `debug.ReadBuildInfo()` 的 `vcs.revision`/`vcs.time` 兜底（拿得到 commit/date，版本号仍是 `(devel)`），或向上游要一个 ldflags 开关（关联：[release-checklist](../engineering/release-checklist.md#发布产物试产mygo-build)）。
- [ ] `[feat]` **桌面原生 UI 的窗口尺寸/布局打磨**（关联：[ADR-011](architecture.md)、[ui-design.md](../requirements/desktop/02-design/ui-design.md)）。

## 不做清单（裁决为删除/延后）

| 条目                                             | 裁决           | 理由                                                                 |
| ------------------------------------------------ | -------------- | -------------------------------------------------------------------- |
| 支持 mp3/flac 之外的输出格式（如 ogg/m4a）       | 删除（不接）   | 容器内的音频本就只有这两种；转换到其它格式是转码，超出「容器解析」定位 |
| 歌词的联网抓取 / 搜索                            | 删除（不接）   | 只消费本地同名 `.lrc`；联网会引入账号与风控面                        |
| ~~GUI / 常驻服务~~                               | **撤销（2026-10-15）** | 原裁决为「删除（不接），定位是批处理 CLI」。现改为：**桌面应用已采纳**（[ADR-010](architecture.md)、[PRD](../requirements/desktop/01-requirements/PRD.md)）；但**托盘常驻与目录监听自动转换仍然不接**，与「批处理/整理」定位不符 |
| 桌面应用内的音频播放                             | 删除（不接）   | 超出「转换器」定位，播放交给用户的播放器                             |

## 归档记录

| 日期       | 内容                                                                 | 去向                                                            |
| ---------- | -------------------------------------------------------------------- | --------------------------------------------------------------- |
| 2026-09-27 | 歌词嵌入、跨平台发布产物、命令行打磨、社区文件、测试与 CI 加固等全部完成的行动计划 | [../archive/TODO.md](../archive/TODO.md)                        |
