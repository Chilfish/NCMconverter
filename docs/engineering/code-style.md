# Go 代码规范

> 最后更新：2026-09-27｜ 适用：本仓库全部 Go 代码。门禁命令见 [`git-workflow.md`](git-workflow.md)。

## 1. 格式与静态检查

- **`gofmt`** 是唯一的格式化标准，CI 用 `make fmt-check` 兜底（`gofmt -l .` 必须无输出）。
- **`go vet ./...`** 必须干净。
- **`golangci-lint` v2**（配置见 [`../../.golangci.yml`](../../.golangci.yml)）：默认 `standard`，并额外启用
  `bodyclose`、`errorlint`、`misspell`、`nilerr`、`revive`、`unparam`。
  - **测试目录豁免 `unparam`**：测试辅助函数为可读性写，某个调用没用到某个参数不算问题。
  - 本地 golangci-lint 版本需与 CI 锁定的 `v2.14.0` 保持一致。
- 编辑器约定见 [`.editorconfig`](../../.editorconfig)：UTF-8、LF、结尾换行、去尾随空格；**Go 与 Makefile 用 Tab 缩进**；`*.md` 允许尾随空格（Markdown 里两个空格是换行）。
- 行尾与二进制标记见 [`.gitattributes`](../../.gitattributes)：统一 checkout 为 LF，媒体/容器标为 binary，`go.sum` 不参与 diff。

## 2. 包组织

```
cmd/ncmconverter/    # 唯一入口：flag → app.Options → app.Run
internal/app/        # 编排（Options、Run、collect/findNCM、convert、outputPath、lyrics.go）
internal/ncm/        # 容器解析（ncm.go 布局、reader.go、errors.go）
internal/converter/  # 解密与解码（converter.go、aes.go、meta.go）
internal/tag/        # 标签门面（tag.go、mime.go）+ mp3/ + flac/
internal/ncmtest/    # 测试用合成容器构造器
```

- **单向依赖**：只允许 `app → {ncm, converter, tag}`、`tag → converter`（复用 `Meta` 与格式常量），不允许反向或成环。
- **`internal/` 一律私有**：这是格式实现的自由空间，也是[ADR-002](../planning/architecture.md) 的约束。
- 新增能力优先落到既有包；只有当某格式的标签逻辑继续膨胀时，才在 `internal/tag/` 下再开子包。

## 3. 命名与结构

| 类型        | 约定                                                              |
| ----------- | ----------------------------------------------------------------- |
| 包名        | 小写单词，无下划线（`ncm`、`converter`、`tag`、`ncmtest`）        |
| 导出类型    | PascalCase（`File`、`Converter`、`Meta`、`Tagger`）               |
| 未导出字段  | 小写（`fd`、`commentAt`、`aesCoreKey`、`musicChunkSize`）         |
| 哨兵错误    | `Err` 前缀（`ErrExtNcm`、`ErrMagicHeader`、`ErrUnknownFormat`）   |
| 常量        | 导出用 PascalCase（`FormatMP3`），内部用小驼峰（`keyXorMask`）    |
| 测试文件    | `*_test.go`，与源码同包（`package app`）或黑盒（`package ncm_test`） |

- 函数签名里的错误一律是 `error` 返回值，不用 panic 表达可恢复错误；`ncmtest` 里的 panic 仅用于「构造夹具时不可能失败的步骤」。
- 导出标识符必须有文档注释，且以标识符名开头（`// File is ...`）。

## 4. 错误处理

- **用 `%w` 包上下文**，让调用方还能 `errors.Is`：`fmt.Errorf("read key section: %w", err)`。
- **判定用 `errors.Is`**，不要字符串比较：`errors.Is(err, fs.ErrNotExist)`、`errors.Is(err, ErrFormat)`。
- **批量失败用 `errors.Join`** 聚合（见 `app.Run` 的 `failures`）。
- **不要吞错**：要么返回，要么至少记一条 `slog.Warn`/`slog.Error`（如 `Close`、封面内嵌这类非致命路径）。
- 关键不变量失败要**显式报错**而非静默兜底：例如 key/meta 前缀不符、格式无法识别，都返回带上下文的错误。

## 5. 注释写「为什么」

- 注释解释**为什么**这么做，而不是复述代码在做什么。
- 布局、常量、上游怪癖这类「反直觉但正确」的地方**必须**留注释：
  - `ncm.go` 包注释里的字节布局，以及「`metaEnd` 不是 CRC32，故意不校验」的结论；
  - `converter.go` 里 `j := byte(i - off + 1)` 的块内相对偏移说明；
  - `flac.go` 里绕开 `ParseFile` 句柄泄漏的理由；
  - `mp3.go` 里锁定 ID3v2.4 的理由。
- **注释与提交信息用英文**；面向用户的 `docs/` 用中文。

## 6. 测试约定

- **行为测试优先**：每个 `Set*`/解析/边界行为都要有测试；修 bug 时先补一条能让它变红的测试（见 [postmortem 011](../postmortem/011-fixture-empty-run.md) 的「改坏它会红吗」）。
- **回归锚点注释**：锁定既有回归的测试，用注释写清它锁住的是什么（如 `// ... locks the regression where ...`）——这些注释是 postmortem 的溯源入口。
- **夹具不复用被测代码**：需要容器/音频时用 `internal/ncmtest`，不要用 `ncm`/`converter` 去造（[ADR-003](../planning/architecture.md)）。
- **测试隔离**：用 `t.TempDir()` 存放产物，`t.Cleanup` 释放句柄；不要在测试间共享可变状态。
- 详见 [`testing.md`](testing.md)。

## 7. 禁止事项

| 禁止                                   | 替代                                                             |
| -------------------------------------- | ---------------------------------------------------------------- |
| 用 `panic` 表达可恢复错误              | 返回 `error`                                                      |
| 字符串比较错误信息                     | `errors.Is` / `errors.As`                                         |
| 丢弃 `Save`/`Close` 的错误             | 返回或用 `errors.Join` 合并，至少 `slog.Warn`                    |
| 未检查的类型断言 / 越界索引            | 显式检查并返回错误（[postmortem 004](../postmortem/004-artist-decode-panic.md)） |
| 直接解引用可能为 nil 的 `Meta.Album`   | 先判 nil（[postmortem 010](../postmortem/010-nil-album-deref.md)） |
| 静默跳过本该失败的测试                 | 失败或用 `t.Fatal`（[postmortem 011](../postmortem/011-fixture-empty-run.md)） |
| 为跑通测试而修改 `testdata/` 夹具      | 夹具是回归锚点，不要动                                           |
