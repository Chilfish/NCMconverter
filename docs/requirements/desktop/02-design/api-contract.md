# API 契约：桌面绑定

> 关联：[architecture.md](architecture.md)｜[../04-testing/test-plan.md](../04-testing/test-plan.md)
> 契约来源：Go 代码里的 `internal/desktop`。**TS 客户端由 `mygo generate` 生成，不手写**；本文档描述生成结果应当长什么样，作为评审与测试的对照。

## 1. 生成规则（MyGo v0.5.5）

- 服务名 = Go 类型名（`Converter`）；前端 `Converter.info()` 调用 `Converter.Info`。
- 方法名 = 导出方法的 lowerCamelCase（`PickFiles` → `pickFiles`、`OpenPath` → `openPath`）。
- 参数名沿用 Go 源码里的形参名（去掉 `context.Context`）。
- 结构体 → `export interface`，字段名取 `json` tag。
- 事件名里的 `:` 会转成属性名：`files:opened` → `events.filesOpened`。
- `*mygo.Channel[T]` 参数 → `Channel<T>`（从 `mygo-runtime` 导入）。
- 返回 `(T, error)` → `Promise<T>`；`error` 会 reject 成 `CallError`。

## 2. Go 侧形状

```go
package desktop

var Opened = mygo.NewEvent[[]string]("files:opened")

type Info struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	MyGo     string `json:"myGo"`
	Platform string `json:"platform"`
	Arch     string `json:"arch"`
}

type Request struct {
	Inputs         []string `json:"inputs"`
	Output         string   `json:"output"`
	OutputTemplate string   `json:"outputTemplate"`
	Tag            bool     `json:"tag"`
	Depth          int      `json:"depth"`
	Threads        int      `json:"threads"`
	DryRun         bool     `json:"dryRun"`
	SkipExisting   bool     `json:"skipExisting"`
}

type Report struct {
	Source  string `json:"source"`
	Dest    string `json:"dest"`
	Outcome string `json:"outcome"` // planned | converted | skipped | failed
	Error   string `json:"error"`
}

type Result struct {
	Total      int  `json:"total"`
	Converted  int  `json:"converted"`
	Skipped    int  `json:"skipped"`
	Failed     int  `json:"failed"`
	Canceled   bool `json:"canceled"`
	DryRun     bool `json:"dryRun"`
	DurationMs int  `json:"durationMs"`
}

type Converter struct{ /* mu, running, cancel */ }

func (c *Converter) Info() Info
func (c *Converter) PickFiles(ctx context.Context) ([]string, error)
func (c *Converter) PickDirectory(ctx context.Context) (string, error)
func (c *Converter) PickOutput(ctx context.Context) (string, error)
func (c *Converter) Scan(inputs []string, depth int) ([]string, error)
func (c *Converter) Run(ctx context.Context, request Request, reports *mygo.Channel[Report]) (Result, error)
func (c *Converter) Cancel()
func (c *Converter) OpenPath(path string) error
```

> 约束：`Converter` 含 `sync.Mutex`，**所有方法用指针接收者**（`go vet` 的 `passes lock by value`）。

## 3. 生成的 TypeScript（对照）

```ts
export interface Info { name: string; version: string; myGo: string; platform: string; arch: string }
export interface Request {
  inputs: string[]; output: string; outputTemplate: string;
  tag: boolean; depth: number; threads: number; dryRun: boolean; skipExisting: boolean;
}
export interface Report { source: string; dest: string; outcome: string; error: string }
export interface Result {
  total: number; converted: number; skipped: number;
  failed: number; canceled: boolean; dryRun: boolean; durationMs: number;
}

export const Converter = {
  info(): Promise<Info>,
  pickFiles(): Promise<string[]>,
  pickDirectory(): Promise<string>,
  pickOutput(): Promise<string>,
  scan(inputs: string[], depth: number): Promise<string[]>,
  run(request: Request, reports: Channel<Report>): Promise<Result>,
  cancel(): Promise<void>,
  openPath(path: string): Promise<void>,
} as const;

export const events = { filesOpened: event<string[]>("files:opened") } as const;
```

## 4. 调用语义

### 4.1 逐文件进度（通道）

```ts
const reports = new Channel<Report>();
const done = Converter.run(request, reports);
for await (const report of reports) show(report);
const result = await done;
```

- 每条 `Report` 对应一个容器，**在它处理完时**推送，顺序不保证。
- 通道的值先于调用结果到达；`Converter.run` 返回时通道已关闭。
- **取消**：`Converter.cancel()` 让 `run` 以 `canceled: true` 正常返回；页面也可以 `reports.close()`（等价于停止本次调用）。

### 4.2 结果与错误

| 情况 | `Result` | 是否 reject |
| --- | --- | --- |
| 全部成功 | 计数齐全，`canceled=false` | 否 |
| 部分失败 | `failed>0`，每条 Report 带 `error` | **否**（失败已逐条可见） |
| 用户取消 | `canceled=true`，计数为已完成部分 | 否 |
| 输入不可读 / `threads<1` | 零值 | 是（`CallError`） |
| 已有运行在跑 | 零值 | 是（`CallError`） |

### 4.3 队列与预览

- `scan(inputs, depth)` 返回将转换的路径列表，语义与 `ncmconverter --dry-run -d <depth>` 一致（同一实现）。
- `pickFiles()` 取消时返回 `[]`；`pickDirectory()`/`pickOutput()` 取消时返回 `""`。
- `events.filesOpened` 仅在菜单路径触发（页面按钮直接用返回值）。

## 5. 契约测试

- **生成物同步**：CI 跑 `bun run generate` 后 `git diff --exit-code -- desktop/src/mygo.ts`；不一致即失败。
- **服务行为**：`internal/desktop/service_test.go` 用 `internal/ncmtest` 造容器，直接调 `Converter.Run`——传零值 `&mygo.Channel[Report]{}`（`Send` 返回 `ErrChannelClosed`，被忽略，**不需要窗口**），断言 `Result` 计数、产物落盘、`Scan` 计数、`Cancel` 归一。
- **前端类型**：`bun run typecheck` 保证页面调用与生成客户端一致。

详见 [../04-testing/test-plan.md](../04-testing/test-plan.md)。
