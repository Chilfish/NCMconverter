# 测试与验证

> 最后更新：2026-09-27｜ 命令定义见 [`../../Makefile`](../../Makefile)

## 1. 三层结构

| 层                    | 范围                                                          | 代表文件                                             |
| --------------------- | ------------------------------------------------------------- | ---------------------------------------------------- |
| **单元**              | 纯函数与单包逻辑（AES、`Int64`、MIME、歌词解码、命名/深度）    | `aes_test.go`、`meta_test.go`、`mime_test.go`、`lyrics_test.go`、`app_test.go` |
| **合成容器端到端**    | 用 `internal/ncmtest` 造容器/音频，真实跑一遍转换并逐字节校验  | `converter_test.go`、`app/integration_test.go`、`mp3_test.go`、`flac_test.go` |
| **真实 fixture 端到端** | 用 `testdata/` 的真实 `.ncm` 转换并断言标签/歌词/音频完整      | `app/sample_test.go`                                 |

## 2. `ncmtest` 的设计哲学（关键）

`internal/ncmtest` **独立重新实现**容器格式（AES、XOR、密钥盒、段拼装各写一份），**不复用被测的 `ncm`/`converter` 代码**。

理由：如果夹具复用被测代码，读取端的一个 bug 会同时藏进「本该抓住它」的夹具里——测试照常通过。独立实现让读取端的错误无处可藏。`ncmtest` 与生产代码的重复是**刻意的**（[ADR-003](../planning/architecture.md)）。

`ncmtest` 提供的构造器：

- `Build(Options)` — 拼装完整容器；`Options` 可分别控制 key 材料、meta（或 `RawMeta` 用于构造畸形 meta）、封面、明文音频。
- `PNG()` / `JPEG()` — **真实可解码**的极小图（封面处理会解码记录尺寸，随便一串字节不行）。
- `MinimalMP3(frames)` — 一个空 ID3v2.3 标签 + 帧数据；**版本刻意是 v2.3**，用来验证打标签能对抗源文件的旧版本编码。
- `MinimalFLAC(frames)` — go-flac 能接受的最短流（StreamInfo 块 + 帧同步码）。

## 3. 可证伪性（「改坏它会红吗」）

一条测试的价值在于**改坏被测行为时它必须变红**。撰写/审查测试时先问这个问题：

- 断言必须是**行为断言**，不是「没有 panic」或「跑完了」。
- 空跑要报绿的反面案例见 [postmortem 011](../postmortem/011-fixture-empty-run.md)（fixture 目录空时曾静默 skip）。
- 锁定回归的测试要带注释写清**锁住的是什么**（如 `TestHandleMusicKeepsTheTailIntact` 锁「短读按整块写出导致补零丢尾」）。

## 4. 命令

```sh
make test          # go test ./...
make test-race     # go test -race ./...（CI 门禁）
make coverage      # go test -covermode=atomic -coverprofile=coverage.out ./...
make vet
make fmt-check
make lint          # golangci-lint v2
```

`internal/app` 的 `TestMain` 把 `slog` 输出丢进 `io.Discard`，避免转换进度污染测试输出。

## 5. 边界清单（改对应代码时必查）

- **音频尾部**：块大小 ±1、多块、恰好整块（`0x8000`）——防补齐/丢尾。
- **容器截断**：某段声明长度越过文件末尾必须报错。
- **魔法头**：两个 magic 各自损坏都要被拒。
- **meta 段**：缺失（长度 0）、畸形（前缀不符）都要正确处理。
- **key 段**：前缀被破坏要报错；未解 key 就解音频要报错。
- **AES**：短输入、空输入、非法 PKCS#7 padding（三种坏形态）。
- **密钥盒**：是 256 的置换、确定性、拒绝空 key。
- **元数据**：`Int64` 的数字/字符串/浮点/null/非法；`Artist` 的缺元素、多余元素、类型不符、null。
- **歌词**：缺失、空、BOM、GBK、超大（>1MiB）。
- **标签**：空值跳过、已有值优先、二次保存、`Close` 幂等、封面下载成功/失败降级。
- **编排**：`--depth` 边界、去重、threads ≤ 0、dry-run 不写盘、skip-existing、模板逃逸。

## 6. 行为 → 测试 映射表

> 用于「改这段代码该跑哪些测试」以及 postmortem 的锚点溯源。

### `internal/ncm`（`ncm_test.go`）

| 行为                                   | 测试                                        |
| -------------------------------------- | ------------------------------------------- |
| 四段长度与内容全部读对                 | `TestParseReadsEverySection`                |
| 每个 magic 单独损坏都被拒绝            | `TestValidateRejectsWrongMagicNumbers`      |
| 非 `.ncm` 扩展名被拒绝                 | `TestValidateRejectsNonNCMExtension`        |
| 截断容器报错                           | `TestParseRejectsTruncatedContainer`        |
| 打开缺失文件报错                       | `TestOpenMissingFile`                       |

### `internal/converter`

| 行为                                   | 测试（文件）                                                |
| -------------------------------------- | ----------------------------------------------------------- |
| 四段解出且音频逐字节相等               | `converter_test.go::TestHandleAllDecodesEverySection`       |
| 音频尾部完整（块边界）                 | `converter_test.go::TestHandleMusicKeepsTheTailIntact`      |
| 无 meta 时嗅探格式                     | `converter_test.go::TestResolveFormatSniffsAudioWhenMetadataIsAbsent` |
| 声明优先且小写化                       | `converter_test.go::TestResolveFormatPrefersTheDeclaredFormat` |
| 未知音频报 `ErrUnknownFormat`          | `converter_test.go::TestResolveFormatRejectsUnknownAudio`   |
| 畸形 meta 报错                         | `converter_test.go::TestHandleMetaRejectsMalformedSection`  |
| 未解 key 解音频报错                    | `converter_test.go::TestHandleMusicRequiresAResolvedKey`    |
| key 前缀损坏被拒                       | `converter_test.go::TestHandleKeyRejectsWrongKeyPrefix`     |
| AES 往返 / 短输入 / 坏 padding         | `aes_test.go::TestDecryptAES128ECB*`                        |
| 密钥盒置换 / 确定性 / 空 key           | `aes_test.go::TestBuildKeyBox*`                             |
| `Int64` 四形态                         | `meta_test.go::TestInt64*`                                  |
| 字符串型标识符                         | `meta_test.go::TestMetaDecodesIdentifiersEncodedAsStrings`  |
| `Artist` 正常/畸形不 panic             | `meta_test.go::TestArtist*`                                 |

### `internal/tag`

| 行为                                   | 测试（文件）                                                |
| -------------------------------------- | ----------------------------------------------------------- |
| 每个字段都写 / 空值跳过                | `tag_test.go::TestWriteTagsWritesEveryField`、`…SkipsAbsentFields` |
| 无 album 不 panic                      | `tag_test.go::TestWriteTagsWithoutAlbumDoesNotPanic`        |
| 封面下载内嵌 / 失败降级                | `tag_test.go::TestWriteTagsDownloadsCoverAndDetectsItsType`、`…FallsBackToALink` |
| 封面嵌不进不阻断其余标签               | `tag_test.go::TestWriteTagsKeepsGoingWhenTheCoverCannotBeEmbedded` |
| 错误冒泡 / nil 元数据                  | `tag_test.go::TestWriteTagsPropagatesTaggerErrors`、`…RejectsNilMetadata` |
| 不支持的格式                           | `tag_test.go::TestNewTaggerRejectsUnsupportedFormats`       |
| MIME 探测 / 下载（含 404/取消/坏 URL） | `mime_test.go`                                              |
| mp3 非拉丁/多艺人/歌词                 | `mp3/mp3_test.go::TestSaveWritesTagsAndKeepsTheAudio`       |
| mp3 二次保存失败 / 已有值优先 / Close  | `mp3/mp3_test.go::TestSaveTwiceFails`、`…SettersPreserveExistingValues`、`…CloseIsIdempotent` |
| flac 复用单一 comment 块               | `flac/flac_test.go::TestSaveTwiceKeepsOneCommentBlock`      |
| flac 链接封面 / Close 幂等 / 非 FLAC   | `flac/flac_test.go::TestSetCoverURLStoresALink`、`…CloseIsIdempotentAndSaveAfterCloseFails`、`…NewRejectsNonFLACFiles` |

### `internal/app`

| 行为                                   | 测试（文件）                                                |
| -------------------------------------- | ----------------------------------------------------------- |
| `--depth` 语义 / 大小写扩展名 / 去重   | `app_test.go::TestFindNCMHonoursDepth`、`…IgnoresUppercaseExtensionsCaseInsensitively`、`…CollectSkipsDuplicates` |
| 缺失输入 / threads ≤ 0 / 无匹配成功    | `app_test.go::TestCollectReportsMissingInputs`、`TestRunRejectsNonPositiveThreads`、`TestRunWithNoMatchesSucceeds` |
| dry-run / skip-existing / 默认覆盖     | `options_test.go::TestRunDryRunWritesNothing`、`…SkipExistingLeavesTheDestinationAlone`、`…OverwritesByDefault` |
| 模板命名 / 回退 / 逃逸防护 / 占位符    | `options_test.go::TestRunOutputTemplate*`、`TestRenderName*` |
| mp3/flac 端到端音频与标签              | `integration_test.go::TestRunConverts*EndToEnd`             |
| 歌词侧车嵌入 / 超大歌词                | `integration_test.go::TestRunEmbedsTheLyricsSidecar`、`…EmbedsLargeLyrics` |
| 失败不中断 / 取消 / 无临时残留         | `integration_test.go::TestRunContinuesAfterAFailure`、`…HonoursCancellation`、`…LeavesNoTemporaryFiles` |
| 真实 fixture 音频完整 + 标签           | `sample_test.go::TestRunConvertsTheRepositorySample`、`TestRunTagsTheConvertedFileFromTheRepositorySample` |

## 7. 真实 fixture 的处理

- `testdata/` 下存在 `.ncm` 时启用真实端到端断言；这些夹具是**回归锚点**，不要为了跑通测试而修改。
- `sample_test.go::repositorySamples`：目录**不存在**才 `t.Skip`；目录存在但**没有容器**则 `t.Fatalf` 失败——避免端到端检查空跑报绿（[postmortem 011](../postmortem/011-fixture-empty-run.md)）。
- 夹具的来源与版权说明见 [`../../testdata/README.md`](../../testdata/README.md)。

## 8. 资源与覆盖

- **不留临时文件**：`TestRunLeavesNoTemporaryFiles` 断言转换后目录里只有源、`.lrc` 与产物——用来兜住标签库的临时文件泄漏。
- **竞态**：CI 与服务端门禁都跑 `-race`。
- **覆盖率**：`make coverage` 写 `coverage.out` 并作为 artifact 上传；**当前未设阈值门禁**（见 [`../planning/backlog.md`](../planning/backlog.md)）。
