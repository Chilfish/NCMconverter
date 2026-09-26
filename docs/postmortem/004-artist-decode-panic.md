---
编号: 004
主题: Artist 解码对畸形输入 panic
日期: 2026-09-27
严重度: 中（崩溃 / 整批受牵连）
状态: Mitigated
根因归类: 设计建模
---

# 004 — Artist 解码对畸形输入 panic

## 摘要

NCM 把艺术家编码成 `[name, id]` 数组。`Artist.UnmarshalJSON` 早期用**未检查的类型断言**取 name、并**直接取 `fields[1]`** 拿 id。遇到空数组、缺元素、类型不符或 `null` 时会 panic，而不是返回一个可诊断的错误——一个畸形容器就能让整批转换在同一处崩溃。

## 影响

- **崩溃**：解码阶段直接 panic，而非把该文件计为一次失败并继续。
- 违背了「单文件失败不中断整批」的设计初衷（[ADR-007](../planning/architecture.md)）。

## 时间线

| commit / 事件 | 说明                                                                 |
| ------------- | -------------------------------------------------------------------- |
| `c86be82`     | 重写管线，`Artist` 解码落位                                          |
| 补测试时      | `meta_test.go::TestArtistMalformedPayloadsDoNotPanic` 锁定该回归      |

## 根因分析（blameless）

- **Why 会 panic？** 未检查的类型断言在形态不符时直接 panic；`fields[1]` 在数组不足两元素时越界。
- **Why 会这样写？** 数组的两元素看起来「结构固定」，于是省略了防御。
- **Why 需要防御？** 输入来自外部文件，形态由上传者决定——这是系统边界，不能假定结构合法。
- **Why 测试没早发现？** 正常样本总是两元素、且名字是字符串，panic 只在畸形输入下出现。

**贡献因素**：把「外部输入」当成了「内部保证」；`encoding/json` 的错误返回被 panic 抢先。

## 做得对的地方

- 修复后测试枚举了 `[]`、`["name"]`、`["name",1,2]`、对象、数字名字、嵌套数组、`null` 七类形态，区分「应报错」与「应放宽」两种情况。

## 行动项

### 缓解

- [x] 改成显式检查：空数组报错、name 解码失败报错、id 缺省视为 0、多余元素忽略。
- [x] `TestArtistMalformedPayloadsDoNotPanic` 锁定全部形态。

### 预防

- [x] `code-style.md` 禁止事项固化：不用未检查的类型断言 / 越界索引。
- [x] 「高频雷区」第 3 条纳入 artist 数组形态。

## 教训

外部输入的解码只能用「显式检查 + 返回错误」，绝不能用未检查的类型断言或直接下标；畸形数据应产生可诊断的失败，而不是 panic。

## Changed Files

```
internal/converter/meta.go
internal/converter/meta_test.go
```
