# 尸检报告索引（Postmortems）

> **开写任何代码前，先读本页。** 本仓库历史踩坑全部沉淀于此，对照「高频雷区」自查后再动工，避免重复返工。
>
> 原则：**blameless** —— 不追究「谁写错了」，只追「什么系统条件允许它发生」，然后修系统和流程。
>
> 这里的每一篇都在 v0.1.0 发布前被发现并被测试锁定（多数来自项目重写期），因此绝大多数状态是「已预防」。新报告按 [TEMPLATE.md](TEMPLATE.md) 沉淀。

## 索引表

| #   | 主题                       | 严重级 | 分类         | 状态         | 一句话根因                                                     |
| --- | -------------------------- | ------ | ------------ | ------------ | -------------------------------------------------------------- |
| [001](001-audio-tail-corruption.md) | 音频反混淆尾部被破坏 | SEV-2 | Bug | 🟢 Mitigated | 短读按整块长度写出，尾部补陈旧字节并丢掉最后一块 |
| [002](002-magic-header-check.md) | 容器魔法校验 | SEV-3 | Bug | 🟢 Mitigated | 两个 magic 用 `and` 组合，只损坏一个仍通过 |
| [003](003-string-identifiers-zeroed.md) | 元数据标识符 | SEV-2 | Bug | 🟢 Mitigated | nil map 让字符串型 ID 的回退路径不可达，静默解成 0 |
| [004](004-artist-decode-panic.md) | Artist 解码 | SEV-2 | Bug | 🟢 Mitigated | 未检查的类型断言 + 直接取下标，畸形数组直接 panic |
| [005](005-non-latin-tags-lost.md) | 非拉丁标签 | SEV-2 | Bug | 🟢 Mitigated | 沿用源文件的 ID3v2.3，默认编码无法表示中日文文本 |
| [006](006-artists-replaced.md) | 多艺人写入 | SEV-3 | Bug | 🟢 Mitigated | `SetArtist` 替换整个帧，逐个设置只剩最后一个 |
| [007](007-flac-duplicate-comment-block.md) | flac comment 块 | SEV-3 | Bug | 🟢 Mitigated | 每次保存都 append，违反「只允许一个 comment 块」 |
| [008](008-save-error-discarded.md) | 标签保存 | SEV-3 | Bug | 🟢 Mitigated | `Save` 丢弃错误且未统一关闭句柄 |
| [009](009-empty-tag-values.md) | 空值标签 | SEV-3 | Bug | 🟢 Mitigated | 无条件写入，产出空标题、把曲名当专辑、嵌入空封面 |
| [010](010-nil-album-deref.md) | 无 meta 容器 | SEV-2 | Bug | 🟢 Mitigated | 假定 album 一定存在，无 meta 段时解引用 nil |
| [011](011-fixture-empty-run.md) | 端到端空跑 | SEV-3 | Process | 🟢 Mitigated | fixture 目录为空时 skip，端到端检查假装通过 |
| [012](012-upstream-contract-traps.md) | 上游契约陷阱 | SEV-3 | Dependency | 🟡 Active | 把第三方库与格式文档当稳定契约（句柄泄漏 / 临时残留 / 校验字段名不实） |

## 高危文件（改前自查）

| 文件                                  | 出现于        | 关注点                                     |
| ------------------------------------- | ------------- | ------------------------------------------ |
| `internal/converter/converter.go`     | #001, #003    | 块边界重置、元数据形态                     |
| `internal/converter/meta.go`          | #003, #004    | `Int64` 回退路径、`Artist` 类型断言        |
| `internal/tag/tag.go`                 | #009, #010    | 空值跳过、nil 安全                         |
| `internal/tag/mp3/mp3.go`             | #005, #006, #008 | 编码版本、帧替换、保存错误              |
| `internal/tag/flac/flac.go`           | #007, #012    | comment 块复用、句柄泄漏                   |
| `internal/app/sample_test.go`         | #011          | fixture 空跑守卫                           |
| `internal/ncm/ncm.go`                 | #002, #012    | 双 magic 校验、未校验字段                  |

## 高频雷区（写码前自查）

### 1. 音频块边界会毁音频（#001）

混淆的密钥盒位置在**每个 `0x8000` 块开头重置**（`j := byte(i - off + 1)` 用的是块内相对偏移）。任何「按整块长度写出」、「少读最后一块」或「补零」都会静默破坏尾部。改动 `HandleMusic` 前，先跑 `converter_test.go::TestHandleMusicKeepsTheTailIntact`（覆盖 `0/1/0x8000±1/多块`）。

### 2. 两个 magic 都要校验（#002）

`CTENFDAM` 是两个 `uint32`。**用「或」判断不合格，不是「与」** —— 只损坏一个也得拒绝。改动 `checkHeader` 前跑 `ncm_test.go::TestValidateRejectsWrongMagicNumbers`。

### 3. 元数据形态不固定（#003/#004）

`musicId`/`albumId`/artist id 可能是数字，也可能是**字符串**（甚至浮点）；artist 是 `[name, id]` 数组，id 可缺。`Int64.UnmarshalJSON` 的字符串回退路径必须真实可达；`Artist.UnmarshalJSON` 对畸形输入**只能返回错误，不能 panic**。改动后跑 `meta_test.go`。

### 4. 非拉丁文本必须 UTF-8（#005）

容器里的音频常自带 ID3v2.3，其默认 ISO-8859-1 编码无法表示中日文。mp3 标签**必须顶到 ID3v2.4 并用 UTF-8**。`mp3.New` 打开后立刻 `SetVersion(4)`，改动前跑 `mp3_test.go::TestSaveWritesTagsAndKeepsTheAudio`（含非拉丁断言）。

### 5. 帧/块是「设置」还是「追加」（#006/#007）

- mp3 的 `SetArtist` **替换**帧：多艺人必须连接成一个值，不能逐个 set。
- flac 的 comment 块**只能有一个**：保存必须复用已有块并原位重写，不能每次 append。
改动后跑 `mp3_test.go` 与 `flac_test.go::TestSaveTwiceKeepsOneCommentBlock`。

### 6. 空值不写、已有值优先（#009/#010）

- 空字符串字段**不要写入**（不写空标题、不把曲名当专辑、不嵌空封面）。
- 每个 `Set*` 只在字段**尚不存在**时写入，不覆盖源标签。
- 读取 `meta.Album` 前先判 nil（无 meta 段是合法容器）。改动前跑 `tag_test.go`。

### 7. 错误不能吞（#008）

`Save` 的错误要冒泡或 `errors.Join`，并保证句柄被关闭；`Close` 要幂等。改动后跑 `mp3_test.go::TestSaveTwiceFails` 与 `flac_test.go::TestCloseIsIdempotentAndSaveAfterCloseFails`。

### 8. 测试不能空跑（#011）

门禁「全绿」可能只是「没有断言失败」。fixture 缺失时**该失败就失败**：目录不存在才 `Skip`，目录存在但为空要 `Fatal`。写测试时先问「改坏被测行为它会红吗」。详见 [`../engineering/testing.md`](../engineering/testing.md)。

### 9. 上游库与格式文档不是稳定契约（#012）

- `go-flac` 的 `ParseFile` 失败时泄漏句柄 → 自持 `os.Open` + `ParseBytes`。
- `id3v2` 的 `Save` 失败时残留临时文件 → 靠 `TestRunLeavesNoTemporaryFiles` 兜底。
- 「meta 段末尾是 CRC32」的文档说法**不成立** → 故意不校验，别按错误假设重新实现。
引入/升级依赖前重读相关注释与 [postmortem 012](012-upstream-contract-traps.md)。

## 报告全览

### 按严重级

- **SEV-2（5 篇）**：001 / 003 / 004 / 005 / 010
- **SEV-3（7 篇）**：002 / 006 / 007 / 008 / 009 / 011 / 012

### 按状态

- 🟢 **Mitigated（已预防）**：除 012 外全部
- 🟡 **Active（上游未修，仍可能复发）**：#012

### 按根因归类

- **设计建模**：001 / 002 / 004 / 006 / 007
- **工具反馈**：003 / 005 / 008 / 009 / 010
- **流程缺失**：011
- **依赖**：012
