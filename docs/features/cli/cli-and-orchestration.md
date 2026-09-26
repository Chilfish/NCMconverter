# 命令行与转换编排

> 子系统：`cmd/ncmconverter` + `internal/app`｜ 关键文件：[`cmd/ncmconverter/main.go`](../../../cmd/ncmconverter/main.go)、[`internal/app/app.go`](../../../internal/app/app.go)、[`convert.go`](../../../internal/app/convert.go)

`cmd/ncmconverter` 只做参数解析与 `app.Options` 组装；真正的编排（收集文件、并发调度、输出命名、写文件）在 `internal/app`。

## 1. CLI 参数

| flag                | 别名          | 默认值 | 说明                                                       |
| ------------------- | ------------- | ------ | ---------------------------------------------------------- |
| `--output`          | `-o`          | 空     | 输出目录；空则写在源文件旁                                 |
| `--output-template` |               | 空     | 结果命名模板（见 §4）                                      |
| `--tag`             | `-t`          | `true` | 是否写入元数据与封面，用 `--tag=false` 关闭                |
| `--depth`           | `-d`,`deepth` | `0`    | 传入目录下向下查找的层数；`0` 只处理该目录的直接子文件     |
| `--threads`         | `-n`,`thread` | `10`   | 同时转换的最大文件数                                       |
| `--dry-run`         |               | `false`| 只列出将转换的文件，不写任何文件                           |
| `--skip-existing`   |               | `false`| 目标已存在则跳过（默认覆盖）                               |
| `--quiet`           | `-q`          | `false`| 只输出警告与错误                                           |
| `--help` / `--version` | `-h`/`-v`  |        | 帮助 / 版本（版本附带 commit 与构建时间）                  |

`main.go` 用 `slog` 输出到 stderr，日志级别用 `slog.LevelVar` 持有——`--quiet` 在参数解析后（`Action` 内）才生效。无输入参数直接报错。`signal.NotifyContext(os.Interrupt)` 提供取消源。

## 2. 收集：`collect` 与 `findNCM`

- `collect(inputs, depth)`：逐个 `os.Stat`。文件直接加入；目录走 `findNCM`。用 `seen` map 去重（输入重叠时不重复转换）。
- `findNCM(dir, depth)`：递归读目录，**大小写不敏感**地匹配 `.ncm`（`strings.EqualFold(filepath.Ext(...), ".ncm")`）。递归语义：`depth <= 0` 不再下探；每进一层 `depth-1`。因此 `--depth 0` = 只处理直接子文件。

> `--depth 0` 传目录的语义是**破坏性变更**：旧行为等于什么都不做，新行为表示「只处理该目录的直接子文件」（见 `CHANGELOG.md`）。

## 3. 并发与失败模型

`Run(ctx, opts)`：

1. `Threads < 1` 直接报错。
2. `collect` 收集；**0 个**：`slog.Warn("no .ncm files to convert")` 并返回 nil（空目录不算失败）。
3. `DryRun`：逐个 `slog.Info("would convert", …)` 后返回，**不写盘**。
4. `errgroup.Group` + `SetLimit(opts.Threads)` 并发；每个任务先检查 `ctx.Err()`。
5. 单文件失败**不中断整批**：错误被记进受 mutex 保护的 `failures` 切片，任务本身返回 nil，让其余文件继续（错误文本带上源路径）。
6. `group.Wait()`：唯一能到这里的是**取消**。
7. 有失败 → 返回 `fmt.Errorf("%d of %d files failed: %w", …, errors.Join(failures...))`，进程最终以非零退出。

`convert(ctx, source, opts)` 的单文件流程：`ncm.Open`（`defer Close`）→ `Parse` → `converter.NewConverter(...).HandleAll()` → `outputPath` → `SkipExisting` 判定（命中则返回 `skipped=true`，算「已处理」而非失败）→ `writeFile` → `--tag` 时读歌词并 `tag.WriteTo`。

## 4. 输出命名与安全

`outputPath`：

- 目录取 `--output`，为空则取源文件所在目录。
- 名字默认沿用源基名（换扩展名）；有模板则 `renderName` 渲染，渲染结果为空则回退源名。
- 扩展名固定取自音频实际格式。
- **路径逃逸防护**：拼接后 `filepath.Rel(dir, dest)`，若结果是 `..` 或以 `..<sep>` 开头则报错——模板或元数据**不能把文件写到输出目录之外**。

`renderName` 支持的占位符（用 `strings.NewReplacer` 一次替换）：

| 占位符     | 取值                                             |
| ---------- | ------------------------------------------------ |
| `{name}`   | 源文件名（不含扩展名）                           |
| `{title}`  | `meta.Name`                                      |
| `{artist}` | 第一位艺人名（无则空串）                         |
| `{album}`  | 专辑名（无则空串）                               |
| `{id}`     | 曲目 ID                                          |
| `{format}` | 音频格式                                         |

模板里未被引用的花括号原样保留（`strings.Replacer` 只替换已知模式）。所有替换值先过 `sanitizeName`：**路径分隔符 `/`、`\` 换成空格，控制字符删除**——元数据无法把文件名变成目录。`writeFile` 负责 `MkdirAll` 父目录并以 0644 写文件。

## 5. 边界与回归锚点

| 行为                                        | 测试                                                              |
| ------------------------------------------- | ----------------------------------------------------------------- |
| `--depth` 0/1/2/超深 的语义                 | `app_test.go::TestFindNCMHonoursDepth`                            |
| `.NCM` 大写扩展名也匹配                     | `app_test.go::TestFindNCMIgnoresUppercaseExtensionsCaseInsensitively` |
| 输入重叠时去重                              | `app_test.go::TestCollectSkipsDuplicates`                         |
| 缺失输入报错                                | `app_test.go::TestCollectReportsMissingInputs`                    |
| threads ≤ 0 报错                            | `app_test.go::TestRunRejectsNonPositiveThreads`                   |
| 无匹配容器时成功返回                        | `app_test.go::TestRunWithNoMatchesSucceeds`                       |
| dry-run 完全不写盘                          | `options_test.go::TestRunDryRunWritesNothing`                     |
| `--skip-existing` 不动已有文件              | `options_test.go::TestRunSkipExistingLeavesTheDestinationAlone`   |
| 默认覆盖                                    | `options_test.go::TestRunOverwritesByDefault`                     |
| 模板命名 / 渲染为空回退                     | `options_test.go::TestRunOutputTemplateNamesTheFile`、`…FallsBackWhenItRendersEmpty` |
| 模板不能逃逸输出目录                        | `options_test.go::TestRunOutputTemplateCannotEscapeTheOutputDirectory` |
| 元数据里的分隔符被中和                      | `options_test.go::TestRenderNameNeutralisesSeparatorsInMetadata`  |
| 六个占位符全部生效                          | `options_test.go::TestRenderNameFillsEveryDocumentedPlaceholder`  |
| 单文件失败不中断整批但最终报错              | `integration_test.go::TestRunContinuesAfterAFailure`              |
| 取消传播                                    | `integration_test.go::TestRunHonoursCancellation`                 |
| 不残留临时文件                              | `integration_test.go::TestRunLeavesNoTemporaryFiles`              |

## 关键文件

- [`cmd/ncmconverter/main.go`](../../../cmd/ncmconverter/main.go) — flag 定义、logger、版本打印、`app.Run` 调用
- [`internal/app/app.go`](../../../internal/app/app.go) — `Options`、`Run`、`collect`、`findNCM`
- [`internal/app/convert.go`](../../../internal/app/convert.go) — `convert`、`outputPath`、`renderName`、`sanitizeName`、`writeFile`

## 相关测试 / postmortem

- 测试：`internal/app/app_test.go`、`options_test.go`、`integration_test.go`
- [postmortem 011 — fixture 空跑报绿](../../postmortem/011-fixture-empty-run.md)
