# 测试计划：桌面应用

> 关联：[PRD §7 验收标准](../01-requirements/PRD.md)｜[UI 设计](../02-design/ui-design.md)｜[设计](../02-design/architecture.md)
> 基线约定见 [`../../engineering/testing.md`](../../engineering/testing.md)（三层结构、`ncmtest` 哲学、可证伪性）。

## 1. 策略

本功能的风险集中在**复用层的行为改变**、**入口分支**与**界面状态**，而不是界面像素。因此：

- **重点测 Go 侧**：`app.OnFile` 的语义、`internal/desktop` 的转换内核（计数与错误归一）与视图结构/状态、双模式分支的可判定性。
- **界面写自动化测试（无需窗口）**：界面已由 MyGo **原生 UI**（`github.com/egoist/mygo/ui`）在 Go 里重建；视图可由 `ui.NewTester` 在**内存里渲染帧**、按文本与点击断言，因此**不需要窗口/WebView** 即可当普通单元测试跑。转换内核（`runner`）同样无窗口。真机观感仍靠人工冒烟（§3）。
- **不新增真实素材依赖**：夹具一律走 `internal/ncmtest`（[ADR-003]）。
- **可证伪性**：每条新测试都写明它锁住的行为；改坏实现必须变红。

## 2. 新增单元测试

### 2.1 `internal/app/report_test.go`（回调语义）

| 测试 | 断言（改坏会红） |
| --- | --- |
| `TestRunReportsPlannedFilesOnADryRun` | 逐文件收到 `OutcomePlanned`，`Source` 列举完整，**磁盘无任何产物** |
| `TestRunReportsConvertedFiles` | 收到 `OutcomeConverted`，`Dest` 等于实际落盘路径，`Err` 为 nil |
| `TestRunReportsSkippedFiles` | `SkipExisting` 且目标已存在时收到 `OutcomeSkipped`，且目标内容未被改写 |
| `TestRunReportsFailures` | 坏容器收到 `OutcomeFailed` 且 `Err != nil`；同批的好容器仍是 `OutcomeConverted`（失败不中断） |
| `TestRunWithoutACallbackStillSucceeds` | `OnFile == nil` 时行为与从前一致（保证回调是纯附加） |

夹具复用 `integration_test.go` 的 `container` / `mp3Container` / `frames` / `readFile`；产物放 `t.TempDir()`。

并发安全：`Run` 会在互斥锁内回调，测试里累加切片无需自己加锁；`make test-race` 必须全绿。

### 2.2 `internal/desktop/runner_test.go`（转换内核，无窗口）

直接驱动 `runner.convert`，用**收集回调**收下每一条 `Report`；夹具走 `internal/ncmtest` 合成容器，因此**不需要窗口**即可跑通整条链路。

| 测试 | 断言 |
| --- | --- |
| `TestRunCountsEveryOutcome` | 一个成功 + 一个 `SkipExisting` 命中 + 一个坏容器 → `Result{Total:3, Converted:1, Skipped:1, Failed:1}`，且成功产物落盘 |
| `TestRunReportsFileFailuresWithoutAnError` | 有失败时 `Run` **不返回错误**（逐条 Report 已带原因），`Result.Failed > 0` |
| `TestRunRejectsASecondRun` | 第一个运行未结束时再次 `Run` 返回错误（单飞） |
| `TestRunNormalizesCancellation` | 预取消的 context → 归一为 `Canceled` 且无错误 |
| `TestScanMatchesWhatARunHandles` | `Scan` 的路径集合与 `Run` 实际处理的容器集合一致（`--depth` 语义） |
| `TestRunRejectsNonPositiveThreads` | `Threads: 0` → 返回错误（透传 `internal/app` 的校验） |

### 2.3 `internal/desktop/view_test.go`（视图，无窗口）

用 MyGo 的 `ui.NewTester` 在**内存里渲染帧**，按元素文本查找并断言（点击与 `HasText`/`Texts`），把界面当普通单元测试跑——无需窗口、无需 WebView。

| 测试 | 断言 |
| --- | --- |
| `TestViewShowsTheSections` | 侧栏出现「转换队列 / 选项 / 日志」三区 |
| `TestViewShowsTheDropHintWhenTheQueueIsEmpty` | 队列为空时显示拖放提示 |
| `TestViewShowsTheEmptyLog` | 空日志给出占位 |
| `TestViewShowsTheQueue` | 队列列出已添加的路径 |
| `TestViewAddsAPathOnlyOnce` | 同一路径只入队一次 |
| `TestViewShowsWhereEachSourceIs` | 每个来源显示它已达到的阶段 |
| `TestQueueEntryStatus` | 表驱动：各来源阶段（`待处理`／`转换中 · d/t`／`N 已转换 · M 跳过 · K 失败`／`N 个将转换`／`无 .ncm`／`路径不可用`）的措辞 |
| `TestViewShowsTheRunOutcomes` | 显示每个文件的结果 |
| `TestViewShowsTheRunSummary` | 队列区显示运行汇总 |
| `TestViewShowsTheRunStateFromEverySection` | 从任一分区都能看到运行状态 |
| `TestViewShowsAFailedRun` | 运行失败时的界面表现 |
| `TestSummarize` | 汇总文案组装 |
| `TestProgressSummary` | 进度文案（`d / t`）组装 |
| `TestOutcomeLabels` | 结果标签文案 |
| `TestSectionTitles` | 分区标题文案 |

> 对话框（系统的打开/保存面板）由操作系统拥有，`ui.NewTester` 无法驱动，因此涉及对话框的路径仍归入 §3 的人工冒烟。

## 3. 需要人工验证的部分

CI 与 `go test` 覆盖不到的，按 [PRD §7](../01-requirements/PRD.md) 逐条走：

| 验收 | 手工步骤 | 期望 |
| --- | --- | --- |
| AC-1 | `go tool mygo dev` | 原生窗口打开：半透明侧栏含「转换队列 / 选项 / 日志」三区（各带图标与计数徽标），内容区自绘内嵌标题栏；无报错 |
| AC-2 | 打包后 `ncmconverter.exe --version`（**从终端**） | 输出 `ncmconverter <ver> (commit …, built …)`；验证 `AttachConsole` |
| AC-3 | `bin/ncmconverter --dry-run -d 2 <目录>` | 列出文件，与改动前逐字一致 |
| AC-4 | 界面跑一次真实 `.ncm` | 产物可播放，带标签与（若有）`.lrc` 歌词 |
| AC-5 | 运行中点「取消」（标题栏 `取消`） | 不再写新文件，汇总显示「已取消」 |
| AC-6 | 队列里掺一个坏容器 | 该来源显示「失败」+ 原因，整批不中断，其余照常完成 |
| 额外 | 拖拽一个目录进窗口 | 队列出现该来源并显示其阶段（`待处理 · N 个`），统计数字与 AC-3 一致 |
| 额外 | 观察每个来源的阶段流转 | 等待（圆环）→ 转换中（`转换中 · d/t`）→ 完成（`N 已转换 · M 跳过 · K 失败`）；运行状态卡与标题栏胶囊同步 |
| 额外 | 勾「试运行」跑一次 | 日志全是「将转换」，磁盘无新文件，汇总标「试运行」 |
| 额外 | 点「打开输出目录」（状态卡内） | 系统文件管理器打开到输出目录 |
| 额外 | 队列放一个不可读路径 | 标记为「路径不可用」，不把整个计数清零，运行仍报告它 |

## 4. 生成物与类型

界面是 Go 代码（MyGo 原生 UI），**没有生成物、也没有生成客户端**：不再有 `mygo generate`/`typecheck`，CI 不再校验任何生成文件同步。视图结构与状态由无窗口的 `ui.NewTester` 测试保证（§2.3），类型一致性由 Go 编译器保证。

## 5. 门禁（与既有保持一致）

```sh
make fmt-check      # gofmt -l . 无输出
make vet            # go vet ./...（会抓 runner 的值接收者复制锁）
make ui-vet         # go tool mygo vet：另抓「ui.Element 被存进状态/闭包/goroutine」
make test-race      # 含新增用例
make build          # go build ./cmd/ncmconverter（CLI 路径可编译）
make coverage       # 覆盖率 artifact（无阈值门禁，沿用现状）
make lint           # golangci-lint v2.14.0
make desktop        # go tool mygo build（打包桌面产物；纯 Go 工具链，无 Node/bun）
```

`generate`/`typecheck` 目标已删除（不再有前端与生成客户端）；整条门禁里不再出现任何 Node/bun。

## 6. 行为 → 测试映射（追加到 engineering/testing.md）

### `internal/app`

| 行为 | 测试 |
| --- | --- |
| 逐文件回调：试运行/成功/跳过/失败 | `report_test.go::TestRunReports*` |
| 无回调时行为不变 | `report_test.go::TestRunWithoutACallbackStillSucceeds` |
| 遍历语义（`--depth`、去重、大小写） | `app_test.go::TestFindNCMHonoursDepth`、`…IgnoresUppercaseExtensionsCaseInsensitively`、`…CollectSkipsDuplicates`（改名后为 `Scan…`） |

### `internal/desktop`

转换内核（`runner_test.go`，无窗口）与视图（`view_test.go`，`ui.NewTester`，无窗口）分列：

| 行为 | 测试 |
| --- | --- |
| 三种结果的计数 | `runner_test.go::TestRunCountsEveryOutcome` |
| 逐文件失败不升级为聚合错误 | `runner_test.go::TestRunReportsFileFailuresWithoutAnError` |
| 单飞 | `runner_test.go::TestRunRejectsASecondRun` |
| 取消归一 | `runner_test.go::TestRunNormalizesCancellation` |
| `Scan` 与 `Run` 的集合一致 | `runner_test.go::TestScanMatchesWhatARunHandles` |
| threads 校验透传 | `runner_test.go::TestRunRejectsNonPositiveThreads` |
| 侧栏三区 | `view_test.go::TestViewShowsTheSections` |
| 空队列的拖放提示 | `view_test.go::TestViewShowsTheDropHintWhenTheQueueIsEmpty` |
| 空日志占位 | `view_test.go::TestViewShowsTheEmptyLog` |
| 队列列出路径 | `view_test.go::TestViewShowsTheQueue` |
| 路径去重 | `view_test.go::TestViewAddsAPathOnlyOnce` |
| 每个来源的阶段 | `view_test.go::TestViewShowsWhereEachSourceIs` |
| 阶段措辞（表驱动） | `view_test.go::TestQueueEntryStatus` |
| 每文件结果 | `view_test.go::TestViewShowsTheRunOutcomes` |
| 运行汇总 | `view_test.go::TestViewShowsTheRunSummary` |
| 状态处处可见 | `view_test.go::TestViewShowsTheRunStateFromEverySection` |
| 运行失败 | `view_test.go::TestViewShowsAFailedRun` |
| 汇总文案 | `view_test.go::TestSummarize` |
| 进度文案 | `view_test.go::TestProgressSummary` |
| 结果标签 | `view_test.go::TestOutcomeLabels` |
| 分区标题 | `view_test.go::TestSectionTitles` |

## 7. 不做的事

- **不做像素/快照测试**：界面只断言结构与状态（`ui.NewTester`，§2.3），不对渲染结果做像素级比对——易 flaky，也会把视图锁死在某帧的绘制细节上。
- 不用真实 `.ncm` 素材跑新增测试（既有 `sample_test.go` 的真实 fixture 断言保持原样）；新测试夹具一律走 `internal/ncmtest`。
- **门禁里不放 Node**：不再有 bun/Vite/前端构建；`make fmt-check/vet/test-race/build`、`make desktop` 全是纯 Go 工具链（`go tool mygo build`）。
