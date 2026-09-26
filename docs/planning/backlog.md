# Backlog（未决任务清单）

**最后更新**: 2026-09-27

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

## 不做清单（裁决为删除/延后）

| 条目                                             | 裁决           | 理由                                                                 |
| ------------------------------------------------ | -------------- | -------------------------------------------------------------------- |
| 支持 mp3/flac 之外的输出格式（如 ogg/m4a）       | 删除（不接）   | 容器内的音频本就只有这两种；转换到其它格式是转码，超出「容器解析」定位 |
| 歌词的联网抓取 / 搜索                            | 删除（不接）   | 只消费本地同名 `.lrc`；联网会引入账号与风控面                        |
| GUI / 常驻服务                                   | 删除（不接）   | 定位是批处理 CLI                                                     |

## 归档记录

| 日期       | 内容                                                                 | 去向                                                            |
| ---------- | -------------------------------------------------------------------- | --------------------------------------------------------------- |
| 2026-09-27 | 歌词嵌入、跨平台发布产物、命令行打磨、社区文件、测试与 CI 加固等全部完成的行动计划 | [../archive/TODO.md](../archive/TODO.md)                        |
