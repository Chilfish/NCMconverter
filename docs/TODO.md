# 行动计划

本文档记录后续要做的事项与优先级。完成一项就勾掉，并尽量附上对应的提交。

优先级定义：

- **P0** —— 明确的用户需求，或当前功能/发布链路的硬缺口。
- **P1** —— 仓库规范化与工程化，让项目达到"可被外部贡献"的水准。
- **P2** —— 打磨项与已知小尾巴，有精力再做。

---

## P0

### 1. 把歌词嵌入转换产物（提交 94dbaef）

目标：转换时如果存在歌词，就写进 mp3/flac，使其在播放器里可见。

**本轮已确认的事实**（对 `testdata/Ave Mujica - KINGS (Cover).ncm` 实测）：

- 容器**只有四段**：key（128B）、meta（706B）、cover（224893B JPEG）、music（5084382B）。**容器内部不含歌词**，所以歌词只能来自外部。
- fixture 配套了同目录的 `testdata/Ave Mujica - KINGS (Cover).lrc`，即来源应是**与 `.ncm` 同名的兄弟 `.lrc` 文件**。
- 该 `.lrc` 是**混合格式**，不是纯 LRC：

  ```text
  {"t":0,"c":[{"tx":"作词: "},{"tx":"atsuko"}]}
  {"t":253,"c":[{"tx":"作曲: "},{"tx":"KATSU"},{"tx":"/"},{"tx":"atsuko"}]}
  {"t":506,"c":[{"tx":"编曲: "},{"tx":"UYKADO"}]}
  [00:00.761] Worries slowly come and kiss Tell me what's your name?
  [00:14.419] また惹かれ合っては
  ```

  前 3 行是网易云的 JSON 曲目信息，其后才是标准 `[mm:ss.mmm]` 时间轴。

**待决策**（已定）：

- [x] 原样保留，不做归一化；`docs/architecture.md` 里说明了开头是 JSON。
- [x] 只查源 `.ncm` 同目录，不递归、不看输出目录。
- [x] 随 `--tag` 一起生效，不新增开关。
- [x] 不复制 `.lrc` 到输出目录。

**实现落点**（提交 94dbaef）：

- [x] `internal/tag/tag.go`：`Tagger` 增加 `SetLyrics(lyrics string) error`，并在 `WriteTags` 中按 `meta`/新参数写入；注意 `WriteTags` 目前的入参需要扩展（歌词不属于 `converter.Meta`，建议作为独立参数传入，不要塞进 Meta）。
- [x] `internal/tag/mp3/mp3.go`：用 `id3v2.AddUnsynchronisedLyricsFrame` 写 `USLT` 帧（`UnsynchronisedLyricsFrame{Encoding: EncodingUTF8, Language: "XXX", Lyrics: ...}`）。注意我们已把标签锁到 ID3v2.4 + UTF-8，歌词含日文/中文，必须走 UTF-8。
- [x] `internal/tag/flac/flac.go`：写 Vorbis comment 的 `LYRICS` 字段（`flacvorbis` 没有对应常量，需要自定义 key；注意与既有的 `setOnce` 语义保持一致）。
- [x] `internal/app/convert.go`：在写文件后、`tag.WriteTo` 前定位 `.lrc` 并读取；文件不存在是**正常情况**，不能报错（老样本就没有 `.lrc`）。
- [x] 测试（用户已预告"相关测试用例稍后也得改改"）：
  - [x] `internal/tag/tag_test.go`：fake tagger 记录 `SetLyrics` 调用；无歌词时不调用。
  - [x] `internal/tag/mp3/mp3_test.go`、`internal/tag/flac/flac_test.go`：写入后回读断言。
  - [x] `internal/app/integration_test.go`：合成容器 + 合成 `.lrc`，断言 USLT/Vorbis 字段存在。
  - [x] `internal/app/sample_test.go`：对真实 fixture 断言歌词已嵌入。
- [x] 边界：空 `.lrc`、编码问题（UTF-8 BOM / GBK）。
- [ ] 边界：超大歌词文件（尚未单独覆盖）。

### 2. 跨平台构建与发布产物（提交 d4ea048、5c750eb）

当前**没有任何发布产物**，且 CI 只在 `ubuntu-latest` 上跑测试。Go 是跨平台的，目标平台至少应覆盖 `linux` / `darwin` / `windows` × `amd64` / `arm64`。

- [x] 引入 [GoReleaser](https://goreleaser.com/)，新增 `.goreleaser.yaml`：交叉编译上述平台、生成 `tar.gz`/`zip` 归档、`checksums.txt`、并附带 `LICENSE` 与 `README.md`。
- [x] 新增 `.github/workflows/release.yml`：在 `v*` tag 上触发 `goreleaser release`。
- [x] `main.version` 已可通过 `-ldflags` 注入（见 `Makefile`），确认 GoReleaser 传值一致（已同时注入 `commit` 与 `date`，`cmd/ncmconverter/main.go` 增加了对应变量）。
- [ ] 可选：Homebrew tap / Scoop bucket / AUR；`nfpm` 生成 deb/rpm。
- [x] CI 增加 OS 矩阵（`windows-latest`、`macos-latest`）跑 `go test`——本轮已在 Windows 上踩到文件句柄与临时文件差异，多平台跑测试很有价值。
- [x] 确认产物命名与 `Windows` 的 `.exe` 后缀（本机已验证 `go build -o bin/` 会按平台补后缀，GoReleaser 快照构建也产出了 `.exe`）。

### 3. 让测试在歌词功能落地后同步更新（提交 94dbaef）

- [x] 见上一节测试清单。
- [x] `internal/app/sample_test.go` 目前假设 fixture **一定有** artist 与 album（`decoded.MetaData.Artists[0].Name`、`decoded.MetaData.Album.Name`）。换一个没有 artist/album 的样本会 panic，而不是给出清晰失败。建议加 nil 保护。（改为经 `firstArtistName`/`albumName` 取值，缺失时断言为空。）
- [x] 样本换成 `Ave Mujica - KINGS (Cover).ncm` 后测试**当前仍然全部通过**——但新样本的 meta 里 `musicId`/`albumId`/`artist id` 全是**字符串**（`"2164260966"`），可考虑补一条针对该形态的回归用例以固化 `Int64` 的行为。（`internal/converter/meta_test.go` 已有数字与字符串两种形态的用例，无需新增。）

---

## P1

### 4. 按现代开源规范补齐仓库文件（提交 a60c1fc）

参考同类成熟项目，目前缺少：

- [x] `CHANGELOG.md`（或由 release-please / git-chglog 自动生成）
- [x] `CONTRIBUTING.md`（含本地开发、`make` 目标、提交信息用 Conventional Commits）
- [x] `CODE_OF_CONDUCT.md`（Contributor Covenant）
- [x] `SECURITY.md`（漏洞报告方式）
- [x] `.github/ISSUE_TEMPLATE/`（bug report、feature request；bug 模板要求附平台、Go 版本、`.ncm` 的 `format` 字段，以及是否带 `.lrc`）
- [x] `.github/PULL_REQUEST_TEMPLATE.md`
- [x] `.github/dependabot.yml`（gomod + github-actions 每周）
- [x] `.editorconfig`、`.gitattributes`（统一 LF、标记二进制文件）
- [x] `docs/` 下补架构说明：容器字节布局（本轮已在 `internal/ncm/ncm.go` 的包注释里写了实测布局，可提炼出来）、解密流程、标签写入策略（见 `docs/architecture.md`）
- [x] 仓库描述与 topics（描述与 7 个 topics 已设置；如需再补充请直接在 GitHub 设置里改）

### 5. `Makefile` 命名与内容（提交 d4ea048）

- [x] 文件名是 `makefile`（小写）。约定俗成用 `Makefile`；在大小写不敏感的文件系统上需要 `git mv makefile Makefile` 才能让 git 记录改名。
- [x] 补 `coverage` 目标（`go test -coverprofile`）。
- [ ] CI 里可复用 `make` 目标，避免 workflow 与 Makefile 两处维护。

### 6. 覆盖率与质量门禁（提交 5c750eb）

- [x] 接入覆盖率（codecov 或 `go test -cover` + 上传 artifact）。（采用 `go test -coverprofile` + 上传 artifact，未接 codecov。）
- [x] 在 CI 里锁定 `golangci-lint` 版本（当前用 `version: latest`，会有非预期升级）。（已锁定 v2.14.0。）
- [ ] 考虑开 `-race` 之外的内存/泄漏检查（例如在测试里断言无残留临时文件）。

---

## P2

### 7. 代码层面已知小尾巴

- [x] **模块路径大小写**：`github.com/chilfish/NCMconverter` 含大写 `NCMconverter`，不符合 Go 的路径小写惯例。改动会破坏既有 `go install` 路径，需先决定是否另发一轮并做重定向说明。（已改为全小写 `github.com/chilfish/ncmconverter`，提交 ecca91c。因为尚无发布版本，不需要重定向说明。）
- [x] **容器 CRC32 未校验**：`internal/ncm/ncm.go` 中 meta 段后的 CRC32 被跳过，版本字节 `0x01` 也未校验。校验它们可以在文件损坏时给出更明确的错误。（**实测该字段并不是 meta 段的 CRC32**：真实样本存的是 `0xf372d5d0`，而对存储字节、去混淆文本、解密后明文分别算 `hash/crc32.ChecksumIEEE` 得到 `0x05f35836`、`0x6bbec699`、`0x25b8169a`，都对不上。其定义未知，**故意不校验**，把发现写进了包注释，避免以后按错误假设重新实现。）
- [ ] **`reserved` 字段语义未证实**：布局中 `metaEnd+5` 处 4 字节实测与 cover 长度相同（见包注释；真实样本上两者同为 224893）。若将来格式有变，这里是风险点，值得找更多样本比对。
- [ ] **上游库问题**：
  - `go-flac/v2` 的 `ParseFile` 在解析失败时**泄漏文件句柄**（无法从外部关闭），我们已在 `internal/tag/flac/flac.go` 改成自己 `os.Open` + `ParseBytes` 绕开。建议向上游反馈，并保留该注释。
  - `bogem/id3v2` 的 `Save()` 在写入失败时不会关闭 `-id3v2` 临时文件，Windows 上会留下无法删除的残留。我们把标签锁到 ID3v2.4 + UTF-8 后不再触发，但仍是隐患。
- [x] **CLI 打磨**：`--dry-run`（只列出将要转换的文件）、`--overwrite`/`--skip-existing`、自定义输出文件名模板、`--quiet`。（已实现 `--dry-run`、`--skip-existing`（默认仍为覆盖）、`--quiet`/`-q`、`--output-template`（占位符 `{name}`/`{title}`/`{artist}`/`{album}`/`{id}`/`{format}`，扩展名固定，且不允许写出输出目录）。未单独提供 `--overwrite`，因为覆盖本就是默认行为。）
- [x] **`--depth` 语义变更**属于破坏性变更，首次发版时需要在 `CHANGELOG` 里写清：以前 `--depth 0` 传目录等于什么都不做，现在表示"只处理该目录下的直接子文件"。（已写入 `CHANGELOG.md` 的 Unreleased。）

### 8. 测试 fixture 的版权与体积

- [ ] `testdata/Ave Mujica - KINGS (Cover).ncm`（5.3MB）与同名 `.lrc` 是**商业歌曲的翻唱**。`.gitignore` 里已有 `!testdata/*` 例外，因此它们会被提交并进入公开仓库——发布前请确认这是否可接受；若不接受，改为在 CI 里按需下载或让测试继续跳过。
- [x] 考虑给 fixture 加一份 `testdata/README.md`，说明来源与用途。（提交 a60c1fc。）
