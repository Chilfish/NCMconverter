---
编号: 008
主题: 标签保存丢弃错误且重复关闭句柄
日期: 2026-09-27
严重度: 低（错误被掩盖 / 资源隐患）
状态: Mitigated
根因归类: 工具反馈
---

# 008 — 标签保存丢弃错误且重复关闭句柄

## 摘要

mp3 标签的 `Save` 早期**丢弃了写入错误**，又未把「保存」与「关闭」统一处理：保存失败不会向上冒泡，句柄也可能被关闭两次或漏关。结果是「写入失败 = 静默成功」。正确行为是让 `Save` 返回错误，并用 `errors.Join` 保证无论保存成败句柄都被关闭；`Close` 则做成幂等。

## 影响

- 用户在写入失败时看不到任何错误，以为产物带上了标签。
- 句柄被重复关闭 / 未关闭会带来跨平台差异（尤其 Windows 上的文件占用与删除失败）。

## 时间线

| commit / 事件 | 说明                                                            |
| ------------- | --------------------------------------------------------------- |
| `c86be82`     | 重写管线，`internal/tag/mp3` 落位                               |
| 补测试时      | `mp3/mp3_test.go::TestSaveTwiceFails`、`TestCloseIsIdempotent` 锁定 |

## 根因分析（blameless）

- **Why 错误被掩盖？** `Save` 的返回值被忽略，错误没有进入返回链。
- **Why 句柄会被重复处理？** 保存内部会释放句柄，调用方又可能再 `Close` 一次，两边对「谁负责释放」没有唯一约定。
- **Why 要有唯一约定？** `stateful` 的标签对象需要明确「`Save` 落盘并释放、`Close` 不写入且幂等」的契约，否则调用点各写各的。
- **Why 测试要二次保存/二次关闭？** 生命周期错误只在重复操作时暴露。

**贡献因素**：错误处理与资源释放职责未明确；缺少针对生命周期的测试。

## 做得对的地方

- 修复后契约清晰：`Save` 返回错误并确保释放（`errors.Join(saveErr, t.Close())`），`Close` 幂等且不写入。
- flac 侧同构：`Save` 后标记 `closed`，`Close` 幂等；两格式都补了生命周期测试。

## 行动项

### 缓解

- [x] mp3 `Save` 返回 `errors.Join(saveErr, t.Close())`；`Close` 幂等。
- [x] flac `Save`/`Close` 同步对齐同一契约。
- [x] `TestSaveTwiceFails`、`TestCloseIsIdempotent`、`TestCloseIsIdempotentAndSaveAfterCloseFails` 锁定。

### 预防

- [x] 「高频雷区」第 7 条固化错误不可吞、句柄要关。
- [x] `code-style.md` 禁止事项：丢弃 `Save`/`Close` 错误。

## 教训

标签对象的 `Save` 必须返回错误并保证释放句柄（保存失败也要关），`Close` 必须幂等；写入失败绝不能是静默成功。

## Changed Files

```
internal/tag/mp3/mp3.go
internal/tag/flac/flac.go
internal/tag/mp3/mp3_test.go
internal/tag/flac/flac_test.go
```
