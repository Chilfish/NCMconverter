# 歌词侧车（.lrc）

> 子系统：`internal/app`（`lyrics.go`）｜ 关键文件：[`internal/app/lyrics.go`](../../../internal/app/lyrics.go)

**容器格式本身不保存歌词**——四段里只有 key/meta/cover/music。歌词只可能来自与容器**同名的兄弟 `.lrc` 文件**（`song.ncm` ↔ `song.lrc`）。本子系统负责定位、读取、解码并把内容交给[标签写入](../tagging/tag.md)。

## 1. 定位与读取

`readLyrics(source)`：

1. 把源路径扩展名换成 `.lrc`（`strings.TrimSuffix(source, filepath.Ext(source)) + ".lrc"`）。
2. `os.ReadFile`。**文件不存在是正常情况**（`errors.Is(err, fs.ErrNotExist)` → 返回 `""` 且 nil error）——大多数容器没有歌词，缺失绝不能导致转换失败。其它读取错误才返回。
3. `decodeLyrics` 解码。

约束（已在 `docs/archive/TODO.md` 中定案）：

- **只查源 `.ncm` 同目录**，不递归、不看输出目录。
- 随 `--tag` 一起生效，不新增开关；`--tag=false` 时不读歌词（`convert` 在写标签前才调用）。
- 不把 `.lrc` 复制到输出目录。

## 2. 解码：BOM 与 GBK

`decodeLyrics(data)`：

1. 先剥离 UTF-8 BOM（`0xEF 0xBB 0xBF`）——编辑器常在开头留下它。
2. 若剩余内容是合法 UTF-8，原样转字符串。
3. 否则按 **GBK** 解码（`simplifiedchinese.GBK.NewDecoder()`）——网易云早期把歌词存成 GBK。解码失败才报错。

## 3. 原样嵌入

内容**不做任何归一化**：网易云在 `.lrc` 开头写的几行 JSON 曲目信息（`){"t":0,"c":[...]}` 之类）会与标准 `[mm:ss.mmm]` 时间轴一起原样保留，避免丢信息。这部分内容最终进入：

- mp3 → `USLT` 帧（[mp3-id3.md](../tagging/mp3-id3.md)）
- flac → `LYRICS` Vorbis 字段（[flac-vorbis.md](../tagging/flac-vorbis.md)）

## 4. 边界与回归锚点

| 行为                                | 测试                                                          |
| ----------------------------------- | ------------------------------------------------------------- |
| 读到同名兄弟侧车                    | `lyrics_test.go::TestReadLyricsReturnsTheSiblingSidecar`      |
| 无侧车不是错误                      | `lyrics_test.go::TestReadLyricsWithoutASidecarIsNotAnError`   |
| 剥离 UTF-8 BOM                      | `lyrics_test.go::TestReadLyricsStripsTheByteOrderMark`        |
| GBK 解码                            | `lyrics_test.go::TestReadLyricsDecodesGBK`                    |
| 空侧车                              | `lyrics_test.go::TestReadLyricsHandlesAnEmptySidecar`         |
| 超大侧车完整读取（不截断）          | `lyrics_test.go::TestReadLyricsReadsALargeSidecar`            |
| 端到端：歌词进入 mp3/flac 产物      | `integration_test.go::TestRunEmbedsTheLyricsSidecar`          |
| 端到端：超大歌词（>1MiB）完整嵌入   | `integration_test.go::TestRunEmbedsLargeLyrics`               |

`largeLyrics()` 构造一条数 MB、且行内含多字节字符的 payload，专门用来越过「标签库可能悄悄限制长度」的边界。

## 关键文件

- [`internal/app/lyrics.go`](../../../internal/app/lyrics.go) — `readLyrics`、`decodeLyrics`、`lyricsExtension`、`utf8BOM`

## 相关测试 / postmortem

- 测试：`internal/app/lyrics_test.go`、`internal/app/integration_test.go`
- [postmortem 009 — 空值/缺省不写标签](../../postmortem/009-empty-tag-values.md)（歌词为空时同样被跳过）
