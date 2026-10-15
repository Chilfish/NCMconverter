# 编码规范（桌面应用部分）

> 基线是 [`../../engineering/code-style.md`](../../engineering/code-style.md)（gofmt、`go vet`、golangci-lint v2、英文注释、错误用 `%w`、导出标识符必须有以自身名开头的文档注释）。本文只写**本功能新增的约定与局部红线**。

## 1. 包与文件

| 位置 | 约定 |
| --- | --- |
| `cmd/ncmconverter/main.go` | 只做「分支 + 组装」，不放业务逻辑 |
| `cmd/ncmconverter/console_windows.go` | `//go:build windows`；只含 `attachConsole` |
| `cmd/ncmconverter/console_other.go` | `//go:build !windows`；同签名空实现 |
| `internal/desktop/service.go` | 绑定给页面的服务与它的 DTO（`Info/Request/Report/Result`） |
| `internal/desktop/app.go` | MyGo 应用装配（`Run`、窗口、菜单），**不含**转换逻辑 |
| `internal/app/report.go` | `Outcome` 常量与 `FileResult` |
| `desktop/` | 纯前端资产；不放 Go 文件 |

- 命名沿用仓库惯例：包名小写单词（`desktop`）、导出类型 PascalCase、未导出小写。
- DTO 字段必须带 `json` tag（前端类型由它生成），tag 一律 lowerCamelCase。

## 2. 依赖方向（红线）

- `internal/app` **禁止**导入 `github.com/egoist/mygo` 或 `internal/desktop`。它是 UI 无关的编排层。
- `internal/desktop` 可以导入 `internal/app` 与 mygo；**禁止**反向。
- `internal/{ncm,converter,tag}` 保持原样，本功能不碰它们。

> 理由见 [ADR-002](../../../planning/architecture.md)；这是「一套逻辑、两个入口」的结构保证。

## 3. 并发与生命周期

- `internal/app` 的 `OnFile` 回调**必须在既有互斥锁内调用**（串行化），调用方无需自己加锁。
- `internal/desktop.Converter` 含 `sync.Mutex`：**所有方法用指针接收者**，否则 `go vet` 报 `passes lock by value`。
- 一次只允许一个运行（单飞）；`begin/finish` 成对，`finish` 必须释放 `cancel` 与 `running`。
- 阻塞调用（`mygo.Dialog.*`）不得在主线程执行：从菜单项里调时放 `go func()`；作为绑定方法调用时 MyGo 已经在独立 goroutine 上跑。

## 4. 错误处理

- 归一化只在 `Converter.Run` 一处做，规则见 [api-contract §4.2](../02-design/api-contract.md)：取消不算错、逐文件失败不算错、其余返回。
- CLI 的日志与退出码语义**不得改变**；新增回调不替代既有 `slog` 输出。

## 5. 前端

- 只用 `mygo generate` 生成的客户端调用 Go（`./mygo`），**不手写** `call("...")`。
- 业务文案中文；标识符英文。
- 页面不得假设运行在 MyGo 里：直接用浏览器打开 dev server 时，`Converter.*` 会失败——开发时以 `mygo dev` 为准。

## 6. 禁止事项（本功能特有）

| 禁止 | 替代 |
| --- | --- |
| 在 `internal/app` 里 import mygo，或把 mygo 的通道类型塞进 `Options` | 用 `OnFile func(FileResult)` 这种纯 Go 回调 |
| 让桌面重写 `convert`/`outputPath`/`lyrics` | 复用 `internal/app`（否则两套逻辑必然漂移） |
| 手改 `desktop/src/mygo.ts` | 改 Go 代码后跑 `bun run generate` |
| 为跑通界面而在 `desktop/` 里读取 `dist/` 之外的绝对路径 | 开发用 `mygo dev`，打包用 `mygo build` |
| 把 bun/Vite 加进 `make vet/test-race/build` 的依赖链 | 新增独立目标 `make desktop/typecheck`，Go 门禁保持无 Node 依赖 |
