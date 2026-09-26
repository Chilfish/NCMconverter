# mp3：ID3v2.4 + UTF-8 标签写入

> 子系统：`internal/tag/mp3`｜ 关键文件：[`internal/tag/mp3/mp3.go`](../../../internal/tag/mp3/mp3.go)｜ 依赖：`github.com/bogem/id3v2`

`mp3.Tag` 是 [`Tagger`](tag.md) 的 mp3 实现，把元数据、封面与歌词写进 ID3v2 标签。

## 1. 为什么锁死 ID3v2.4 + UTF-8

`New` 打开文件后**立刻 `tag.SetVersion(4)`**。理由：容器里的音频常常自带一个 **ID3v2.3** 标签，而 ID3v2.3 默认文本编码是 **ISO-8859-1**，无法表示任何非拉丁文标题或艺人——中文/日文曲目会直接丢字。锁定 v2.4 后文本帧一律以 UTF-8 写出。

所有文本帧（标题、艺术家、comment、歌词、封面描述）都显式使用 `id3v2.EncodingUTF8`。

> 这是格式层面「必须顶掉源版本」的少数场景之一（见 [postmortem 005](../../postmortem/005-non-latin-tags-lost.md)）。

## 2. 字段映射

| `Tagger` 方法   | ID3v2 帧 / 行为                                                                 |
| --------------- | ------------------------------------------------------------------------------- |
| `SetTitle`      | 仅当 `tag.Title()` 为空才 `SetTitle`（已有值优先）                              |
| `SetAlbum`      | 仅当 `tag.Album()` 为空才 `SetAlbum`                                            |
| `SetArtists`    | 仅当无 `TPE1` 帧时，用 `"; "` **连接** 所有名字写进单个艺术家帧                 |
| `SetComment`    | 仅当无 `COMM` 帧时，写 `CommentFrame{EncodingUTF8, Language:"XXX"}`             |
| `SetLyrics`     | 仅当无 `USLT` 帧时，写 `UnsynchronisedLyricsFrame{EncodingUTF8, Language:"XXX"}` |
| `SetCover`      | `AddAttachedPicture`（`APIC`，`PTFrontCover`，MIME 由调用方判）                 |
| `SetCoverURL`   | `APIC`，MIME 用 `"-->"`（`mimeURL`），图片数据即 URL 字符串                      |

两个实现要点：

- **艺术家必须连接而非逐个 set**：`id3v2.SetArtist` 会**替换**整个帧，逐个设置只会留下最后一个名字（见 [postmortem 006](../../postmortem/006-artists-replaced.md)）。
- **歌词帧用 UTF-8**：歌词通常是中文/日文，v2.4 的 UTF-8 编码保证播放器能正确显示。
- **`-->` 是 ID3 规范里「图片数据其实是一个 URL」的约定 MIME**，供下载失败时的降级路径使用。

## 3. 生命周期

- `Save()`：若已关闭返回错误；否则 `tag.Save()`，并 `errors.Join(saveErr, t.Close())`——**保存失败也要确保句柄被关闭**。二次 `Save` 会失败（不是静默重复写）。
- `Close()`：幂等，多次调用返回 nil。

> 早期实现丢弃了 `Save` 的错误又重复关句柄（见 [postmortem 008](../../postmortem/008-save-error-discarded.md)）。

## 4. 已知上游陷阱

`bogem/id3v2` 的 `Save()` 在写入失败时**不会关闭它创建的 `-id3v2` 临时文件**，在 Windows 上会留下无法删除的残留。项目把标签锁到 ID3v2.4 + UTF-8 后不再走到那条失败路径，但这仍是隐患——见 [`planning/backlog.md`](../../planning/backlog.md) 与 [postmortem 012](../../postmortem/012-upstream-contract-traps.md)。集成测试 `TestRunLeavesNoTemporaryFiles` 会检查转换后目录里不留多余文件，作为这类泄漏的兜底。

## 5. 回归锚点

| 行为                                              | 测试                                                  |
| ------------------------------------------------- | ----------------------------------------------------- |
| 非拉丁标题/专辑/多艺人/comment/封面/歌词全部写入  | `mp3_test.go::TestSaveWritesTagsAndKeepsTheAudio`（含 `TPE1` = `"結束バンド; A"` 与 `USLT` 断言） |
| 二次 `Save` 失败                                  | `mp3_test.go::TestSaveTwiceFails`                     |
| 已有值优先（重开再写不覆盖）                      | `mp3_test.go::TestSettersPreserveExistingValues`      |
| `Close` 幂等                                      | `mp3_test.go::TestCloseIsIdempotent`                  |

## 关键文件

- [`internal/tag/mp3/mp3.go`](../../../internal/tag/mp3/mp3.go)

## 相关测试 / postmortem

- 测试：`internal/tag/mp3/mp3_test.go`
- [postmortem 005 — 非拉丁标签丢失](../../postmortem/005-non-latin-tags-lost.md)
- [postmortem 006 — 多位艺人只剩最后一位](../../postmortem/006-artists-replaced.md)
- [postmortem 008 — Save 丢弃错误](../../postmortem/008-save-error-discarded.md)
