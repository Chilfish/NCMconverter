# 解密、密钥盒与音频反混淆

> 子系统：`internal/converter`｜ 关键文件：[`converter.go`](../../../internal/converter/converter.go)、[`aes.go`](../../../internal/converter/aes.go)

`converter` 把 [`ncm.File`](../container/container-format.md) 切出的四段字节，变成可播放的 `MusicData` 与结构化的 `MetaData`。整条链是单向的、分步的，`HandleAll()` 按依赖顺序把它们串起来：

```
HandleKey → HandleMeta → HandleMusic → resolveFormat
   key 段      meta 段       music 段      格式判定
```

## 1. 常量与密钥

| 名称             | 值 / 长度          | 用途                                             |
| ---------------- | ------------------ | ------------------------------------------------ |
| `keyPrefix`      | `neteasecloudmusic`| key 段解密后的明文前缀                           |
| `keyXorMask`     | `0x64`             | key 段解密前的逐字节异或掩码                     |
| `metaPrefix`     | `163 key(Don't modify):` | meta 段去混淆后的前缀（base64 之前）       |
| `metaXorMask`    | `0x63`             | meta 段去混淆的逐字节异或掩码                    |
| `metaJSONPrefix` | `music:`           | meta 段解密后 JSON 的前缀                        |
| `musicChunkSize` | `0x8000`           | 音频反混淆的块大小                               |
| `aesCoreKey`     | 16 字节            | 解密 key 段的 AES-128 密钥                       |
| `aesModifyKey`   | 16 字节            | 解密 meta 段的 AES-128 密钥                      |

导出的格式常量：`FormatMP3 = "mp3"`、`FormatFLAC = "flac"`；哨兵错误 `ErrUnknownFormat`、`ErrInvalidPadding`。

## 2. key 段

1. 逐字节异或 `0x64`。
2. AES-128-ECB 解密（`decryptAES128ECB`，含 PKCS#7 去填充）。
3. 明文必须以 `neteasecloudmusic` 开头，否则报错——这是**格式有效性的一道闸**，坏 key 段在这里就被挡住。
4. 去掉前缀后的字节即「派生音频密钥盒的种子」，存入 `KeyData`。

## 3. meta 段

1. 逐字节异或 `0x63`。
2. 必须以 `163 key(Don't modify):` 开头，否则报错。
3. 去掉前缀后 **base64 解码**。
4. AES-128-ECB 解密，明文必须以 `music:` 开头。
5. 去掉 `music:` 后是一段 JSON。同一段 JSON **既含曲目字段也含专辑字段**，因此解析两次：一次装入 `Meta`，一次装入 `Album`，再把 `Album` 挂到 `meta.Album`。
6. `meta.Comment` 记录**原始（去混淆后的）meta 段文本**，用于日志与标签的 comment 字段。

**meta 段长度为 0 是合法的**：`HandleMeta` 直接给一个空 `Meta{}` 返回，格式留待从音频帧判定。

## 4. music 段（反混淆）

音频帧的混淆是一个**自逆**变换：`buildKeyBox` 从 key 种子派生一个 256 字节的置换盒，再按下面的方式逐字节异或：

```go
for off := 0; off < len(source); off += musicChunkSize {
    end := min(off+musicChunkSize, len(source))
    for i := off; i < end; i++ {
        j := byte(i - off + 1)   // 注意：相对块首，不是相对文件
        k := box[j]
        decoded[i] = source[i] ^ box[k+box[k+j]]
    }
}
```

**关键不变量：`j` 由「相对当前块首的偏移」算出，即密钥盒位置在每个 `0x8000` 块开头重置。** 这行 `i - off + 1` 是整条管线最易错的地方：块边界算错会直接破坏音频。`byte` 运算天然截断到 256，这正是这些查表下标能留在盒内的原因（源码有注释说明）。

`buildKeyBox`：

- 先填 `box[i] = byte(i)`，再做一次变体的 Fisher–Yates 洗牌（`c = (box[i] + lastByte + key[keyOffset]) & 0xff`）。
- 结果是 256 个值的一个**置换**（测试断言无重复）。
- key 为空时报错。

## 5. 格式判定

`resolveFormat()` 的优先级：**元数据声明的 `format` 优先**，并且会被小写化；只有声明缺失时，才用 `formatFromMagic` 从音频开头嗅探：

| 开头字节            | 判定   |
| ------------------- | ------ |
| `fLaC`              | flac   |
| `ID3`               | mp3    |
| `0xFF` 且第二字节高 3 位为 `111`（MPEG 同步字） | mp3 |
| 其它                | `ErrUnknownFormat`（错误里带上开头几字节） |

**声明与实测冲突时以声明为准**——即使音频看起来像 mp3，只要 meta 说 flac 就按 flac 处理。这是刻意的（见 [`planning/architecture.md`](../../planning/architecture.md)），因为后续写标签必须知道真实容器类型，而元数据是更权威的来源。

## 6. 边界与回归锚点

| 行为                                          | 测试                                                        |
| --------------------------------------------- | ----------------------------------------------------------- |
| 四段全部解出，且音频逐字节等于原文            | `converter_test.go::TestHandleAllDecodesEverySection`       |
| 音频尾部不被补齐/丢弃（块大小 ±1 与多块）     | `converter_test.go::TestHandleMusicKeepsTheTailIntact`      |
| 无 meta 时从音频嗅探格式                      | `converter_test.go::TestResolveFormatSniffsAudioWhenMetadataIsAbsent` |
| 声明优先且被小写化                            | `converter_test.go::TestResolveFormatPrefersTheDeclaredFormat` |
| 无法识别的音频报 `ErrUnknownFormat`           | `converter_test.go::TestResolveFormatRejectsUnknownAudio`   |
| 损坏的 meta 段报错并提及前缀                  | `converter_test.go::TestHandleMetaRejectsMalformedSection`  |
| 未解 key 就解音频要报错                       | `converter_test.go::TestHandleMusicRequiresAResolvedKey`    |
| 损坏的 key 前缀被拒绝                         | `converter_test.go::TestHandleKeyRejectsWrongKeyPrefix`     |
| AES 往返、短输入、非法 padding                | `aes_test.go`（`TestDecryptAES128ECB*`）                    |
| 密钥盒是置换 / 确定性 / 拒绝空 key            | `aes_test.go`（`TestBuildKeyBox*`）                         |

## 关键文件

- [`internal/converter/converter.go`](../../../internal/converter/converter.go) — 常量、`Converter`、`Handle*`、`resolveFormat`、`formatFromMagic`
- [`internal/converter/aes.go`](../../../internal/converter/aes.go) — `decryptAES128ECB`、`unpadPKCS7`、`buildKeyBox`

## 相关测试 / postmortem

- 测试：`internal/converter/converter_test.go`、`internal/converter/aes_test.go`
- [postmortem 001 — 音频反混淆尾部被破坏](../../postmortem/001-audio-tail-corruption.md)
