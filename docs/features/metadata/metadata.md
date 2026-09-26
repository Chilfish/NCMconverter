# 元数据模型与解码

> 子系统：`internal/converter`（`meta.go`）｜ 关键文件：[`internal/converter/meta.go`](../../../internal/converter/meta.go)

meta 段解密后是一段 JSON（详见 [decryption §3](../converter/decryption.md)）。本页描述这段 JSON 被映射成的 Go 结构，以及两个「NCM 不老实」的地方：标识符既可能是数字也可能是字符串，艺术家是 `[name, id]` 数组。

## 1. 数据模型

```go
type Meta struct {
    ID       Int64    `json:"musicId"`
    Name     string   `json:"musicName"`
    Artists  []Artist `json:"artist"`
    BitRate  Int64    `json:"bitrate"`
    Duration Int64    `json:"duration"`
    Format   string   `json:"format"`

    Album   *Album `json:"-"` // 与上面的字段来自同一段 payload
    Comment string `json:"-"`
}

type Album struct {
    ID       Int64  `json:"albumId"`
    Name     string `json:"album"`
    CoverURL string `json:"albumPic"`
}

type Artist struct {
    Name string
    ID   Int64
}
```

- `Album` 与 `Meta` 由**同一段 JSON 解析两次**得到（[decryption §3](../converter/decryption.md)），所以 `Album` 的 JSON tag 是 `-`：它不从 `Meta` 反序列化，而是显式挂上去的指针。
- `Comment` 存原始 meta 段文本，JSON tag 也是 `-`。它会被写进产出文件的 comment 字段，所以**不该出现在日志里**——`Meta.String()` 特意省略了它（有测试锁）。
- `Album` 是指针：容器可能没有 meta 段，此时 `MetaData.Album == nil`。所有读取方都要容忍 nil（见 [postmortem 010](../../postmortem/010-nil-album-deref.md)）。

## 2. `Int64`：数字与字符串双形态

NCM 的标识符字段（`musicId`、`albumId`、artist id，乃至 `duration`）可能被编码成 JSON 数字，也可能被编码成字符串（例如新样本里全是 `"2164260966"` 这种带引号的形态）；个别还会是浮点。

`Int64` 通过自定义 `UnmarshalJSON` 统一接收：

| 输入                | 结果     |
| ------------------- | -------- |
| `2611651882`        | 2611651882 |
| `"2611651882"`      | 2611651882 |
| `123.75` / `"123.75"` | 123（截断） |
| `-5`                | -5       |
| `null`              | 0        |
| `""`                | 0        |
| `"abc"` / `{}`      | 报错     |

实现要点：先 `strings.Trim(data, "\"")` 去引号，空串/`null` 归零，然后 `strconv.ParseInt`；失败再尝试 `strconv.ParseFloat`（兜浮点），仍失败才报错。**不丢大整数精度**——用 `ParseInt(..., 64)` 而非先过 float。

> 曾有一处回归：字符串回退路径因一个 nil map 变得不可达，导致带引号的 id 全部静默解成 0（见 [postmortem 003](../../postmortem/003-string-identifiers-zeroed.md)）。

## 3. `Artist`：`[name, id]` 数组

NCM 把艺术家编码成两元素数组：`["結束バンド", 54103171]`，其中 id 也可能是字符串。`Artist.UnmarshalJSON` 先解成 `[]json.RawMessage`，再分别解 `fields[0]` 为名字、`fields[1]`（若存在）为 id。

它必须对畸形输入**不 panic**（用错误返回而非未检查的类型断言/越界索引）：

| 输入                  | 行为              |
| --------------------- | ----------------- |
| `["結束バンド", 54103171]` | ✅ name + id  |
| `["結束バンド", "54103171"]` | ✅ 字符串 id |
| `["name"]`            | ✅ 只 name，id 缺省 |
| `["name", 1, 2]`      | ✅ 多余元素忽略   |
| `[]`                  | ❌ 报错            |
| `{"name":"x"}`        | ❌ 报错（不是数组） |
| `[1, 2]`              | ❌ 报错（name 不是字符串） |
| `null`                | ❌ 报错            |

> 早期实现用未检查的类型断言 + 直接取 `fields[1]`，遇到缺元素或类型不符就 panic（见 [postmortem 004](../../postmortem/004-artist-decode-panic.md)）。

## 4. 日志渲染

`Meta.String()` 与 `Album.String()` 用 `json.Marshal` 输出，仅用于日志；`Meta.String()` 不含 `Comment`（避免把原始 meta 文本刷进日志）。

## 关键文件

- [`internal/converter/meta.go`](../../../internal/converter/meta.go) — `Int64`、`Meta`、`Album`、`Artist`、`String()`

## 相关测试 / postmortem

- 测试：`internal/converter/meta_test.go`
- [postmortem 003 — 字符串型 ID 静默归零](../../postmortem/003-string-identifiers-zeroed.md)
- [postmortem 004 — Artist 解码 panic](../../postmortem/004-artist-decode-panic.md)
- [postmortem 010 — 无 meta 时 album 为 nil](../../postmortem/010-nil-album-deref.md)
