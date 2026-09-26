# 标签写入门面：`Tagger` 与 `WriteTo`

> 子系统：`internal/tag`｜ 关键文件：[`internal/tag/tag.go`](../../../internal/tag/tag.go)、[`internal/tag/mime.go`](../../../internal/tag/mime.go)

`internal/tag` 是标签写入的**门面**：它定义各格式共有的一组 setter，按格式分派到 [`mp3`](mp3-id3.md) 或 [`flac`](flac-vorbis.md)，并统一封面获取策略与错误语义。上层（[`internal/app`](../cli/cli-and-orchestration.md)）只认这个门面，不直接碰具体格式库。

## 1. `Tagger` 接口

```go
type Tagger interface {
    SetCover(cover []byte, mime string) error
    SetCoverURL(coverURL string) error
    SetTitle(title string) error
    SetAlbum(album string) error
    SetArtists(artists []string) error
    SetComment(comment string) error
    SetLyrics(lyrics string) error
    Save() error
    Close() error
}
```

两条贯穿所有实现的语义：

- **记录式 setter**：每个 `Set*` 只把值记进内存结构，不落盘。
- **`Save` 一次性落盘并释放文件；`Close` 在不写入的前提下释放文件，且可重复调用**——后者是为了能和 `defer` 搭配（`WriteTo` 正是 `defer tagger.Close()` + 中途 `WriteTags` 里 `Save`）。

`NewTagger(path, format)` 按 `strings.ToLower(format)` 分派到 `mp3.New` / `flac.New`；其它格式返回 `ErrFormat`（`"only mp3 and flac can be tagged"`）。

## 2. `WriteTo` / `WriteTags` 流程

`WriteTo(ctx, path, cover, meta, lyrics)`：打开文件 → `WriteTags` → `defer Close`（Close 出错只记警告）。`WriteTags` 是核心，接收一个已打开的 `Tagger`：

1. **封面**（`writeCover`，见 §3）。
2. **标题**：仅当 `meta.Name != ""`。
3. **专辑**：仅当 `meta.Album != nil && meta.Album.Name != ""`。
4. **艺术家**：`artistNames` 收集非空名字，非空才写。
5. **comment**：仅当 `meta.Comment != ""`（就是原始 meta 段文本）。
6. **歌词**：仅当 `lyrics != ""`。
7. **`Save`**。

两条重要约定：

- **空值跳过而不是写空串**。写空标题、把曲名误当专辑、嵌入空封面都是过去的回归（见 [postmortem 009](../../postmortem/009-empty-tag-values.md)）。
- **`meta` 为 nil 直接报错**（`"write tags: metadata is nil"`），不继续。

## 3. 封面三级策略（`writeCover`）

| 条件                           | 行为                                                       |
| ------------------------------ | ---------------------------------------------------------- |
| `cover` 非空（容器内自带）     | 按魔数判 MIME（PNG 或 JPEG）后**内嵌**                      |
| `cover` 为空且 `album.CoverURL` 有值 | 下载后内嵌；下载失败则**降级为存链接**（`SetCoverURL`） |
| 都没有                         | 不写封面                                                   |

- **装饰性数据不致命**：内嵌封面失败（`SetCover` 返回错误）只记一条警告，**不阻断**其余元数据；下载失败同理（记警告 + 存链接）。
- MIME 判定：`detectImageMIME` 见到 PNG 八字节签名（`0x89 PNG\r\n\x1a\n`）返回 `image/png`，否则一律 `image/jpeg`。测试覆盖空输入、截断 PNG 签名等边界。
- 下载：`fetchCover` 用 `http.NewRequestWithContext`（尊重取消），30s 超时的 `coverHTTPClient`，非 200 状态报错，`io.LimitReader` 把读取上限设为 `maxCoverSize = 16 MiB`。

## 4. 错误语义

- **致命**：`meta == nil`、setter/`Save` 返回的错误——用 `%w` 包一层上下文（如 `"set title: %w"`、`"save tags: %w"`）向上冒泡。
- **非致命（仅警告）**：封面内嵌失败、封面下载失败、`Close` 失败。

## 5. 边界与回归锚点

| 行为                                        | 测试                                                          |
| ------------------------------------------- | ------------------------------------------------------------- |
| 每个字段都写到（含 MIME）                   | `tag_test.go::TestWriteTagsWritesEveryField`                  |
| 空字段一个都不写，但 `Save` 仍调用          | `tag_test.go::TestWriteTagsSkipsAbsentFields`                 |
| 无 album 不 panic                           | `tag_test.go::TestWriteTagsWithoutAlbumDoesNotPanic`          |
| 下载封面并探测类型                          | `tag_test.go::TestWriteTagsDownloadsCoverAndDetectsItsType`   |
| 下载失败降级为链接                          | `tag_test.go::TestWriteTagsFallsBackToALink`                  |
| setter 错误被包装后冒泡                     | `tag_test.go::TestWriteTagsPropagatesTaggerErrors`            |
| nil 元数据被拒绝                            | `tag_test.go::TestWriteTagsRejectsNilMetadata`                |
| 封面嵌不进也要写完其余标签                  | `tag_test.go::TestWriteTagsKeepsGoingWhenTheCoverCannotBeEmbedded` |
| 不支持的格式报 `ErrFormat`                  | `tag_test.go::TestNewTaggerRejectsUnsupportedFormats`         |
| MIME 探测 / 下载（成功、404、取消、坏 URL） | `mime_test.go`                                                |

## 关键文件

- [`internal/tag/tag.go`](../../../internal/tag/tag.go) — `Tagger`、`NewTagger`、`WriteTo`/`WriteTags`/`writeCover`、`artistNames`
- [`internal/tag/mime.go`](../../../internal/tag/mime.go) — MIME 探测、`fetchCover`

## 相关测试 / postmortem

- 测试：`internal/tag/tag_test.go`、`internal/tag/mime_test.go`
- [postmortem 009 — 空值标签被写入](../../postmortem/009-empty-tag-values.md)
- [postmortem 010 — 无 meta 时 album 为 nil](../../postmortem/010-nil-album-deref.md)
