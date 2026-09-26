---
编号: 010
主题: 无 meta 段容器打标签时解引用 nil
日期: 2026-09-27
严重度: 中（崩溃）
状态: Mitigated
根因归类: 工具反馈
---

# 010 — 无 meta 段容器打标签时解引用 nil

## 摘要

**没有 meta 段的容器是合法的**（`HandleMeta` 会返回一个空 `Meta{}`，格式改由音频嗅探）。此时 `Meta.Album` 保持为 nil。`WriteTags` 早期假定专辑一定存在，直接解引用 `meta.Album.Name`，于是这类容器一打标签就 panic。

## 影响

- 无元数据容器（老样本常见）直接崩溃，违背「失败隔离」的批次语义。
- 崩溃点在标签阶段，错误信息也不指向真正的成因。

## 时间线

| commit / 事件 | 说明                                                                     |
| ------------- | ------------------------------------------------------------------------ |
| `c86be82`     | 重写管线，`internal/tag/tag.go` 落位                                     |
| 补测试时      | `tag_test.go::TestWriteTagsWithoutAlbumDoesNotPanic` 锁定该回归           |

## 根因分析（blameless）

- **Why 会 panic？** 写专辑前没有判 `meta.Album` 是否为 nil。
- **Why 会有 nil？** 「无 meta 段」是合法输入，此时专辑必然是 nil；模型用指针表达「可能不存在」，但使用处当成了「一定存在」。
- **Why 没早点发现？** 合成夹具总带 meta/album；真实老样本才会缺 meta。
- **Why 要专门测？** 必须显式构造「无 meta 段」的容器才能触发。

**贡献因素**：可选字段用指针表达，读侧缺少 nil 防御；夹具默认补齐了字段。

## 做得对的地方

- 修复点小而集中：`meta.Album != nil && meta.Album.Name != ""` 才写专辑；`firstArtistName`/`albumName` 等取值函数也都做 nil 安全。
- 测试直接构造无 album 的元数据，断言只写标题、不 panic。

## 行动项

### 缓解

- [x] `WriteTags` 与相关取值函数做 nil 安全。
- [x] `TestWriteTagsWithoutAlbumDoesNotPanic` 锁定。
- [x] 真实 fixture 断言改用 `firstArtistName`/`albumName`，缺失时断言为空而非崩溃。

### 预防

- [x] 「高频雷区」第 6 条纳入 nil 防御。
- [x] `code-style.md` 禁止事项：直接解引用可能为 nil 的 `Meta.Album`。

## 教训

「无 meta 段」是合法容器，`Meta.Album` 可能为 nil；所有读取方都必须做 nil 防御，夹具要显式覆盖缺字段的形态。

## Changed Files

```
internal/tag/tag.go
internal/tag/tag_test.go
internal/app/sample_test.go
```
