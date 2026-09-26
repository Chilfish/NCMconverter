---
编号: 012
主题: 上游库与格式文档不是稳定契约
日期: 2026-09-27
严重度: 低（但可能长期潜伏）
状态: Active
根因归类: 依赖
---

# 012 — 上游契约陷阱

## 摘要

三处「把第三方库或格式文档当成稳定契约」的陷阱：`go-flac` 的 `ParseFile` 在解析失败时**泄漏文件句柄**；`id3v2` 的 `Save` 在写入失败时**残留 `-id3v2` 临时文件**；以及格式文档普遍声称 meta 段末尾的 4 字节是 **CRC32**，实测**根本不是**（用任何算法都对不上）。前两处已经在本地绕开，第三处选择**故意不校验**。

## 影响

- **句柄泄漏**：解析失败时文件无法被释放，Windows 上会占用文件。
- **临时文件残留**：写入失败时留下无法删除的残留（Windows 更明显）。
- **错误的校验假设**：若按「CRC32」实现校验，会拒绝所有真实容器——比不校验更糟。

## 时间线

| commit / 事件 | 说明                                                              |
| ------------- | ----------------------------------------------------------------- |
| `c86be82`     | 重写管线，`internal/tag/flac`、`internal/ncm` 落位                |
| `9b3df58`     | docs: record that the meta checksum is not validated              |
| `3634951`     | test: assert a conversion leaves no temporary files behind        |

## 根因分析（blameless）

- **Why 踩坑？** 默认「库函数会妥善处理自己的资源」「文档描述的字段就是它说的那样」。
- **Why 不成立？**
  - `go-flac` 的 `ParseFile` 在失败时不交出可关闭对象——API 本身没有给调用方关闭的机会。
  - `id3v2` 的 `Save` 失败路径没有清理自己创建的临时文件。
  - meta 末尾字段在真实样本上是 `0xf372d5d0`，而 `hash/crc32.ChecksumIEEE` 对「存储字节 / 去混淆文本 / 解密明文」分别得到 `0x05f35836` / `0x6bbec699` / `0x25b8169a`，都不匹配。
- **Why 值得记录？** 这些是「不写下来就会被下一个人按错误假设重新实现」的知识。

**贡献因素**：第三方库的资源契约不完整；格式文档与实测不符；这类问题 typecheck/lint/test 都覆盖不到。

## 做得对的地方

- **绕开而非分叉**：flac 改成自持 `os.Open` + `ParseBytes`（`NewBufIOWithInner` 包装），不修改上游。
- **明确的「不校验」**：把「这不是 CRC32」的实测结论写进 `ncm.go` 包注释，而不是硬凑一个校验。
- **用测试兜底**：`TestRunLeavesNoTemporaryFiles` 断言转换后目录无残留，作为临时文件/句柄泄漏的行为防线。

## 行动项

### 缓解

- [x] flac 自持句柄，避开 `ParseFile` 的泄漏。
- [x] meta 校验字段**故意不校验**，理由写入注释（提交 `9b3df58`）。
- [x] `TestRunLeavesNoTemporaryFiles` 覆盖 mp3/flac（提交 `3634951`）。

### 预防

- [ ] 向上游反馈 `go-flac` 的 `ParseFile` 句柄泄漏与 `id3v2` 的临时文件残留。
- [ ] 升级 `go-flac` / `id3v2` 前重读相关注释与本篇。
- [x] 「高频雷区」第 9 条固化：上游库与格式文档不是稳定契约。

## 教训

第三方库的资源契约和格式文档都可能不实：解析失败可能泄漏句柄、写入失败可能残留临时文件、文档字段可能是错的——绕开并写清理由，别按文档的假设去校验。

## Changed Files

```
internal/tag/flac/flac.go
internal/ncm/ncm.go
internal/app/integration_test.go
```
