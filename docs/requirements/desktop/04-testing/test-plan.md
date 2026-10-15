# 测试计划：桌面应用

> 关联：[PRD §7 验收标准](../01-requirements/PRD.md)｜[API 契约](../02-design/api-contract.md)
> 基线约定见 [`../../engineering/testing.md`](../../engineering/testing.md)（三层结构、`ncmtest` 哲学、可证伪性）。

## 1. 策略

本功能的风险集中在**复用层的行为改变**与**入口分支**，而不是界面像素。因此：

- **重点测 Go 侧**：`app.OnFile` 的语义、`internal/desktop` 的计数与错误归一、双模式分支的可判定性。
- **界面不写自动化测试**：MyGo 的 web 前端测试需要窗口/WebView，收益低且易 flaky；界面用 `mygo dev` 人工冒烟（同仓库既有做法：GUI 相关不引入脆弱测试）。
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

### 2.2 `internal/desktop/service_test.go`（服务行为，无窗口）

直接调 `Converter.Run`，用**零值** `&mygo.Channel[Report]{}` 作为通道参数——`Channel.Send` 在内部为 nil 时返回 `ErrChannelClosed`，服务端忽略该错误，因此**不需要窗口**即可跑通整条链路。

| 测试 | 断言 |
| --- | --- |
| `TestRunCountsEveryOutcome` | 一个成功 + 一个 `SkipExisting` 命中 + 一个坏容器 → `Result{Total:3, Converted:1, Skipped:1, Failed:1}`，且成功产物落盘 |
| `TestRunReportsFileFailuresWithoutAnError` | 有失败时 `Run` **不返回错误**（逐条 Report 已带原因），`Result.Failed > 0` |
| `TestRunRejectsASecondRun` | 第一个运行未结束时再次 `Run` 返回错误（单飞） |
| `TestRunNormalizesCancellation` | 预取消的 context → `Result.Canceled == true` 且无错误 |
| `TestScanMatchesWhatARunHandles` | `Scan` 的路径集合与 `Run` 实际处理的容器集合一致（`--depth` 语义） |
| `TestRunRejectsNonPositiveThreads` | `Threads: 0` → 返回错误（透传 `internal/app` 的校验） |

> 若零值 `Channel` 在上游 MyGo 版本上表现不同（例如 panic），退路：把 `Options.OnFile` → `Channel.Send` 的映射抽成纯函数 `reportOf(app.FileResult) Report`，并为它写表驱动测试，`Run` 只做调用。届时在 [design-decisions](../05-review/design-decisions.md) 记一笔。

## 3. 需要人工验证的部分

CI 与 `go test` 覆盖不到的，按 [PRD §7](../01-requirements/PRD.md) 逐条走：

| 验收 | 手工步骤 | 期望 |
| --- | --- | --- |
| AC-1 | `bun run dev` | 窗口打开，页面可用，无控制台报错 |
| AC-2 | 打包后 `ncmconverter.exe --version`（**从终端**） | 输出 `ncmconverter <ver> (commit …, built …)`；验证 `AttachConsole` |
| AC-3 | `bin/ncmconverter --dry-run -d 2 <目录>` | 列出文件，与改动前逐字一致 |
| AC-4 | 界面跑一次真实 `.ncm` | 产物可播放，带标签与（若有）`.lrc` 歌词 |
| AC-5 | 运行中点「取消」 | 不再写新文件，汇总显示「已取消」 |
| AC-6 | 队列里掺一个坏容器 | 日志一行「失败」+ 原因，其余照常完成 |
| 额外 | 拖拽一个目录进窗口 | 队列出现该目录，统计数字与 AC-3 一致 |
| 额外 | 勾「试运行」跑一次 | 日志全是「将转换」，磁盘无新文件 |
| 额外 | 点「打开输出目录」 | 系统文件管理器打开到输出目录 |

## 4. 生成物与类型

- **绑定同步**（CI，Linux）：`bun run generate` 后 `git diff --exit-code -- desktop/src/mygo.ts`。
- **前端类型**：`bun run typecheck`（`tsc --noEmit`），保证页面调用与生成的客户端一致。

## 5. 门禁（与既有保持一致）

```sh
make fmt-check      # gofmt -l . 无输出
make vet            # go vet ./...（会抓 Converter 的值接收者复制锁）
make test-race      # 含新增用例
make build          # go build ./cmd/ncmconverter（CLI 路径可编译）
make coverage       # 覆盖率 artifact（无阈值门禁，沿用现状）
make lint           # golangci-lint v2.14.0
```

## 6. 行为 → 测试映射（追加到 engineering/testing.md）

### `internal/app`

| 行为 | 测试 |
| --- | --- |
| 逐文件回调：试运行/成功/跳过/失败 | `report_test.go::TestRunReports*` |
| 无回调时行为不变 | `report_test.go::TestRunWithoutACallbackStillSucceeds` |
| 遍历语义（`--depth`、去重、大小写） | `app_test.go::TestFindNCMHonoursDepth`、`…IgnoresUppercaseExtensionsCaseInsensitively`、`…CollectSkipsDuplicates`（改名后为 `Scan…`） |

### `internal/desktop`

| 行为 | 测试 |
| --- | --- |
| 三种结果的计数 | `service_test.go::TestRunCountsEveryOutcome` |
| 逐文件失败不升级为聚合错误 | `service_test.go::TestRunReportsFileFailuresWithoutAnError` |
| 单飞 | `service_test.go::TestRunRejectsASecondRun` |
| 取消归一 | `service_test.go::TestRunNormalizesCancellation` |
| `Scan` 与 `Run` 的集合一致 | `service_test.go::TestScanMatchesWhatARunHandles` |
| threads 校验透传 | `service_test.go::TestRunRejectsNonPositiveThreads` |

## 7. 不做的事

- 不为界面写像素/快照测试。
- 不用真实 `.ncm` 素材跑新增测试（既有 `sample_test.go` 的真实 fixture 断言保持原样）。
- 不把 bun/Vite 引入 `make vet/test-race/build` 的依赖链。
