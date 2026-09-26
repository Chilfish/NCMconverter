# flac：Vorbis comment 与 picture 写入

> 子系统：`internal/tag/flac`｜ 关键文件：[`internal/tag/flac/flac.go`](../../../internal/tag/flac/flac.go)｜ 依赖：`github.com/go-flac/go-flac/v2`、`flacvorbis/v2`、`flacpicture/v2`

`flac.Tag` 是 [`Tagger`](tag.md) 的 flac 实现。flac 的元数据不是 ID3，而是**元数据块**序列：文本走 Vorbis comment 块，封面走 picture 块。

## 1. 打开：为什么不用 `flac.ParseFile`

`New` **不用** `flac.ParseFile`，而是自己 `os.Open` 再 `flac.ParseBytes(flac.NewBufIOWithInner(fd))`。原因：`go-flac` 的 `ParseFile` 在解析失败时**泄漏文件句柄**（它不把可关闭的对象交给调用方）。自己持有 `fd` 就能在失败时 `fd.Close()`。

`NewBufIOWithInner` 这层包装是必要的——库在重写流时要能重新找到文件。

打开后扫描 `file.Meta`，找到 Vorbis comment 块并记下它的**下标 `commentAt`**（-1 表示还没有）；若出现多个 comment 块（规范不允许，但容忍），保留最后一个。

## 2. 写入语义

| `Tagger` 方法 | 行为                                                                        |
| ------------- | --------------------------------------------------------------------------- |
| `SetTitle`    | `setOnce(FIELD_TITLE, …)`                                                   |
| `SetAlbum`    | `setOnce(FIELD_ALBUM, …)`                                                   |
| `SetComment`  | `setOnce(FIELD_DESCRIPTION, …)`                                             |
| `SetLyrics`   | `setOnce("LYRICS", …)`                                                      |
| `SetArtists`  | 仅当无现有 `ARTIST` 时，**每条名字一个 `ARTIST` 条目**（Vorbis 支持多值）    |
| `SetCover`    | 追加 picture 块（`PictureTypeFrontCover`，描述 `"Front cover"`）            |
| `SetCoverURL` | picture 块，MIME 用 `flacpicture.MIMEURL`，图片数据是 URL 字符串            |

- `setOnce(key, value)`：先 `comments.Get(key)`，只有**没有**该 key 时才 `Add`——保证不比源文件里已有的标签更"强势"。
- **`LYRICS` 是自定义 key**：Vorbis 规范没有为歌词定义标准字段名，`flacvorbis` 也没有对应常量；`LYRICS` 是播放器普遍认同的写法，故在包内定义为常量 `lyricsField`。

## 3. 保存：复用同一个 comment 块

FLAC 规范**只允许一个 Vorbis comment 块**。`Save()`：

1. 把 `comments` marshal 成块；
2. 若 `commentAt >= 0` → **原位替换** `file.Meta[commentAt]`；否则 append 新块；
3. `file.Save(path)` —— go-flac 会**就地重写元数据并平移其后的音频帧**，所以目标可以是刚打开解析的那个文件；
4. 标记 `closed = true`（`Save` 已经释放了两个句柄）。

早期实现每次 `Save` 都 append，反复保存会不断追加 comment 块，违反规范（见 [postmortem 007](../../postmortem/007-flac-duplicate-comment-block.md)）。

`Close()` 幂等、不写入。

## 4. 回归锚点

| 行为                                            | 测试                                                          |
| ----------------------------------------------- | ------------------------------------------------------------- |
| 标签写入且音频帧保留                            | `flac_test.go::TestSaveWritesTagsAndKeepsTheAudio`            |
| 两次保存只保留一个 comment 块，且已有值优先     | `flac_test.go::TestSaveTwiceKeepsOneCommentBlock`             |
| 链接封面（MIME = `MIMEURL`，数据是 URL）        | `flac_test.go::TestSetCoverURLStoresALink`                    |
| `Close` 幂等、`Save` 在 `Close` 后失败          | `flac_test.go::TestCloseIsIdempotentAndSaveAfterCloseFails`   |
| 非 FLAC 文件被拒绝                              | `flac_test.go::TestNewRejectsNonFLACFiles`                    |

端到端层面，`internal/app/integration_test.go` 还会断言产物**恰好一个** comment 块、picture 的 MIME 与类型正确。

## 关键文件

- [`internal/tag/flac/flac.go`](../../../internal/tag/flac/flac.go)

## 相关测试 / postmortem

- 测试：`internal/tag/flac/flac_test.go`
- [postmortem 007 — flac 重复 comment 块](../../postmortem/007-flac-duplicate-comment-block.md)
- [postmortem 012 — 上游契约陷阱（go-flac 句柄泄漏）](../../postmortem/012-upstream-contract-traps.md)
