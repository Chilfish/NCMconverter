---
编号: 007
主题: flac 反复保存不断追加 comment 块
日期: 2026-09-27
严重度: 低（违反格式规范 / 文件膨胀）
状态: Mitigated
根因归类: 设计建模
---

# 007 — flac 每次保存都追加一个 comment 块

## 摘要

FLAC 规范**只允许一个 Vorbis comment 块**。若 `Save` 每次都 `append` 一个 comment 块，同一文件被保存多次后就会有多个 comment 块——不符合规范，也让播放器对「用哪个」产生分歧。正确做法是记录已有 comment 块的下标，保存时**原位替换**。

## 影响

- 非规范文件：严格的解析器可能拒绝或只读第一个块。
- 文件随保存次数增长，元数据出现重复。
- 反复转换同一文件会不断膨胀。

## 时间线

| commit / 事件 | 说明                                                                        |
| ------------- | --------------------------------------------------------------------------- |
| `c86be82`     | 重写管线，`internal/tag/flac` 落位                                          |
| 补测试时      | `flac/flac_test.go::TestSaveTwiceKeepsOneCommentBlock` 锁定该回归           |

## 根因分析（blameless）

- **Why 会重复？** 保存时无条件 append 新块，没有复用已存在的块。
- **Why 会无条件 append？** 从「新建文件」的路径推广到「已有文件」时，漏掉了「已经有一个 comment 块」的情况。
- **Why 强调单块？** FLAC 的元数据块模型（StreamInfo 必须唯一、Vorbis comment 只允许一个）与 ID3 的帧模型不同，直觉容易把「追加」当作安全操作。
- **Why 测试要保存两次？** 只保存一次永远只有一个块，重复问题只在第二次保存时出现。

**贡献因素**：打开时未记录 comment 块位置；格式规范与 ID3 的差异未被显式建模。

## 做得对的地方

- 修复方案是在 `New` 时扫描 `file.Meta` 并记录 `commentAt`，`Save` 据此原位替换或首次追加。
- 测试**连续保存两次**并断言「恰好一个块」且「原有值被保留」。

## 行动项

### 缓解

- [x] `New` 记录 `commentAt`；`Save` 原位重写已有 comment 块。
- [x] `TestSaveTwiceKeepsOneCommentBlock` 锁定。
- [x] 端到端断言产物恰好一个 comment 块。

### 预防

- [x] 「高频雷区」第 5 条纳入 flac 单块约束。
- [x] `features/tagging/flac-vorbis.md` 记录规范约束。

## 教训

flac 只允许一个 Vorbis comment 块：保存必须复用已有块并原位重写，不能每次 append；测试要连续保存多次来暴露重复。

## Changed Files

```
internal/tag/flac/flac.go
internal/tag/flac/flac_test.go
internal/app/integration_test.go
```
