# .ncm 容器格式与解析

> 子系统：`internal/ncm`｜ 关键文件：[`internal/ncm/ncm.go`](../../../internal/ncm/ncm.go)、[`reader.go`](../../../internal/ncm/reader.go)、[`errors.go`](../../../internal/ncm/errors.go)

`.ncm` 是网易云音乐的加密容器。它把「派生音频密钥的种子」「曲目元数据」「封面图」「加密音频帧」四样东西按长度前缀顺序拼在一起。本子系统只负责**按字节偏移把四段切出来**，不做任何解密或解码（那是 [`converter`](../converter/decryption.md) 的职责）。

## 1. 字节布局

一个容器是若干带长度前缀的段落。除最前面的魔数外，每段前都有一个 little-endian 的 `uint32` 记录其后字节数：

| 偏移                        | 大小         | 内容                                        |
| --------------------------- | ------------ | ------------------------------------------- |
| 0                           | 8            | 魔数，ASCII 的 `CTENFDAM`                   |
| 8                           | 2            | 填充（忽略）                                |
| 10                          | 4            | key 段长度                                  |
| 14                          | `keyLen`     | key 段                                      |
| `14+keyLen`                 | 4            | meta 段长度                                 |
| `18+keyLen`                 | `metaLen`    | meta 段                                     |
| `metaEnd`                   | 4            | 校验字段，**不是 CRC32**（见 §4，不校验）   |
| `metaEnd+4`                 | 1            | meta 段版本，实测恒为 `0x01`（不校验）      |
| `metaEnd+5`                 | 4            | reserved，实测与 cover 长度相同（不读取）   |
| `metaEnd+9`                 | 4            | cover 段长度                                |
| `metaEnd+13`                | `coverLen`   | cover 段，一张 JPEG 或 PNG                  |
| `metaEnd+13+coverLen`       | 剩余         | 混淆后的音频帧                              |

> 布局是对真实容器实测得到的。原实现照抄的文档把 `metaEnd` 处写成 CRC32，实测对不上（详见 §4）。`metaEnd+5` 的 reserved **不读取**，但其后的 cover 长度必须被计入，否则音频偏移会错位。

## 2. 读取模型

[`ncm.go`](../../../internal/ncm/ncm.go) 把段抽象成 `Section{Length, Bytes}`，容器抽象成 `File`：

```go
type File struct {
    Path, FileDir, FileName, Ext string // 路径元信息，供输出命名使用
    Key, Meta, Cover, Music      Section
    fd *os.File
}
```

- `Open(path)` 打开文件并保存句柄（调用方负责 `Close`）。扩展名、目录、基名在这一步就算好，避免后续反复 `filepath`。
- `Parse()` 按 **key → meta → cover → music** 顺序读段，每步失败都包一层上下文（`"read key section: %w"`）。
- 读段必须**严格按顺序**：每一段的偏移取决于前面所有段的实测长度，所以 `keyOffset → metaOffset → coverOffset → musicOffset` 是一条链：

```go
keyOffset   = 8 + 2
metaOffset  = keyOffset + 4 + Key.Length
coverOffset = metaOffset + 4 + Meta.Length + 5 /*meta 尾部*/ + 4 /*reserved*/
musicOffset = coverOffset + 4 + Cover.Length
```

- meta 段**允许长度为 0**（无元数据容器是合法的），此时 `Meta.Length == 0`，后续偏移仍然成立。
- music 段没有长度前缀，读到文件末尾为止（`io.ReadAll`）。

`section(off)` 统一实现「定位 → 读 4 字节长度 → 按长度 `io.ReadFull`」；短读会返回错误而不是静默截断。

## 3. 校验

`Validate()` 做两件事：

1. 扩展名必须（大小写不敏感）是 `.ncm`，否则返回 `ErrExtNcm`。
2. 文件头两个 `uint32` 必须分别等于 `MagicHeader1`(`0x4e455443`) 与 `MagicHeader2`(`0x4d414446`)，否则返回 `ErrMagicHeader`。

两个魔数是把 ASCII `CTEN` / `FDAM` 当 little-endian `uint32` 读出来的结果。**两个魔数都要校验**——早期实现把它们用 `and` 组合，只坏一个仍会通过（见 [postmortem 002](../../postmortem/002-magic-header-check.md)）。

`errors.go` 暴露两个哨兵错误，供调用方 `errors.Is` 判定：`ErrExtNcm`、`ErrMagicHeader`。

## 4. 未校验字段的来龙去脉

`metaEnd` 处的 4 字节字段，格式文档普遍称之为 meta 段的 CRC32。实测不成立：

- 真实样本上该字段读作 `0xf372d5d0`；
- 对「存储的 meta 字节」「去混淆后的文本」「AES 解密后的明文」分别算 `hash/crc32.ChecksumIEEE`，得到 `0x05f35836`、`0x6bbec699`、`0x25b8169a`，**都对不上**。

结论：该字段定义未知，**故意不校验**，避免以后有人按错误假设重新实现。这段结论写在 `ncm.go` 的包注释里，详见 [postmortem 012](../../postmortem/012-upstream-contract-traps.md)。

`metaEnd+4` 的版本字节实测恒为 `0x01`，`metaEnd+5` 的 reserved 实测等于 cover 长度——两者都记录在注释里但**不读取/不校验**，属于已知风险点（见 [`planning/backlog.md`](../../planning/backlog.md)）。

## 5. 边界与回归锚点

| 行为                             | 测试                                                              |
| -------------------------------- | ----------------------------------------------------------------- |
| 四段长度与内容全部读对           | `ncm_test.go::TestParseReadsEverySection`                         |
| 每一个魔数单独损坏都要被拒绝     | `ncm_test.go::TestValidateRejectsWrongMagicNumbers`               |
| 非 `.ncm` 扩展名被拒绝           | `ncm_test.go::TestValidateRejectsNonNCMExtension`                 |
| 段长度越界（截断容器）要报错     | `ncm_test.go::TestParseRejectsTruncatedContainer`                 |
| 打开不存在的文件要报错           | `ncm_test.go::TestOpenMissingFile`                                |

## 关键文件

- [`internal/ncm/ncm.go`](../../../internal/ncm/ncm.go) — 布局常量、`File`、偏移链、`Parse`/`Validate`
- [`internal/ncm/reader.go`](../../../internal/ncm/reader.go) — 泛型 `readUint[T]`
- [`internal/ncm/errors.go`](../../../internal/ncm/errors.go) — 哨兵错误

## 相关测试 / postmortem

- 测试：`internal/ncm/ncm_test.go`
- [postmortem 002 — 魔数校验用 and 组合](../../postmortem/002-magic-header-check.md)
- [postmortem 012 — 上游契约陷阱](../../postmortem/012-upstream-contract-traps.md)
