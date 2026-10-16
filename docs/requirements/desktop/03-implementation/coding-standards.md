# 编码规范（桌面应用部分）

> 基线是 [`../../engineering/code-style.md`](../../engineering/code-style.md)（gofmt、`go vet`、golangci-lint v2、英文注释、错误用 `%w`、导出标识符必须有以自身名开头的文档注释）。本文只写**本功能新增的约定与局部红线**。

## 1. 包与文件

| 位置 | 约定 |
| --- | --- |
| `cmd/ncmconverter/main.go` | 只做「分支 + 组装」，不放业务逻辑 |
| `cmd/ncmconverter/console_windows.go` | `//go:build windows`；只含 `attachConsole` |
| `cmd/ncmconverter/console_other.go` | `//go:build !windows`；同签名空实现 |
| `internal/desktop/app.go` | App 状态、窗口、菜单、对话框与运行编排 |
| `internal/desktop/view.go` | 视图与它的辅助函数（`ui.*`） |
| `internal/desktop/runner.go` | 无窗口的转换内核：`Request`/`Report`/`Result`、单飞、错误归一 |
| `internal/app/report.go` | `Outcome` 常量与 `FileResult` |

- 命名沿用仓库惯例：包名小写单词（`desktop`）、导出类型 PascalCase、未导出小写。
- 视图与辅助函数只放 `view.go`，无窗口的转换内核只放 `runner.go`；二者的红线见 §5。

## 2. 依赖方向（红线）

- `internal/app` **禁止**导入 `github.com/egoist/mygo` 或 `internal/desktop`。它是 UI 无关的编排层。
- `internal/desktop` 可以导入 `internal/app` 与 mygo；**禁止**反向。
- `internal/{ncm,converter,tag}` 保持原样，本功能不碰它们。

> 理由见 [ADR-002](../../../planning/architecture.md)；这是「一套逻辑、两个入口」的结构保证。

## 3. 并发与生命周期

- `internal/app` 的 `OnFile` 回调**必须在既有互斥锁内调用**（串行化），调用方无需自己加锁。
- `internal/desktop` 的 runner 含 `sync.Mutex`：**所有方法用指针接收者**，否则 `go vet` 报 `passes lock by value`。
- 一次只允许一个运行（单飞）；`begin/finish` 成对，`finish` 必须释放 `cancel` 与 `running`。
- 阻塞调用（`mygo.Dialog.*`）不得在主线程执行：从菜单项或视图回调里调时放 `go func()`，完成后经 `Window.Update` 回写状态（见 §5）。

## 4. 错误处理

- 归一化只在 runner 一处做，规则见 [设计 §5.3](../02-design/architecture.md)：取消不算错、逐文件失败不算错、其余返回。
- CLI 的日志与退出码语义**不得改变**；新增回调不替代既有 `slog` 输出。

## 5. 原生 UI（红线）

界面是 MyGo 的**原生 UI**（`github.com/egoist/mygo/ui`），不是网页，也没有绑定层。以下约定不可逾越：

- **视图跑在主线程**：直接读写 `App` 状态即可；**从别的 goroutine 改动状态必须经 `Window.Update`**——转换 goroutine 绝不直接碰状态。
- **`ui.Element` 只在一趟 build 内有效**：不得存进状态、闭包或 goroutine，否则下一帧即悬垂。MyGo 的 `go tool mygo vet` 会抓这类错误。
- **集合要有身份**：列表/表格等集合必须给稳定的 `Key`/`ListState`，让每帧与上一帧对得上（日志列表跟随末尾用 `ListState.FollowEnd`）。
- 业务文案中文；标识符英文。

## 6. 禁止事项（本功能特有）

| 禁止 | 替代 |
| --- | --- |
| 在 `internal/app` 里 import mygo，或把 mygo 的类型塞进 `Options` | 用 `OnFile func(FileResult)` 这种纯 Go 回调 |
| 让桌面重写 `convert`/`outputPath`/`lyrics` | 复用 `internal/app`（否则两套逻辑必然漂移） |
| 在转换 goroutine 里直接读写 `App` 状态 | 状态变化一律经 `Window.Update`（§5） |
| 把 `ui.Element` 存进状态、闭包或 goroutine | 每趟 build 重新构造；`go tool mygo vet` 会抓 |
| 给列表/集合不设 `Key`/`ListState` | 用稳定 Key 与 `ListState` 保持身份（§5） |
