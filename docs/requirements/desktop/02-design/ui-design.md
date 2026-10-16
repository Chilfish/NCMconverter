# UI 设计：原生界面

> 关联：[architecture.md](architecture.md)｜[../01-requirements/PRD.md](../01-requirements/PRD.md)｜[../04-testing/test-plan.md](../04-testing/test-plan.md)
> 决策：[ADR-011](../../../planning/architecture.md)（原生 UI 取代 web 前端）、[design-decisions DD-3/DD-8](../05-review/design-decisions.md)
> 实现：[`internal/desktop/view.go`](../../../../internal/desktop/view.go)（视图）、[`app.go`](../../../../internal/desktop/app.go)（状态与装配）、[`runner.go`](../../../../internal/desktop/runner.go)（无窗口内核）

界面是 MyGo 的**原生 UI**（`github.com/egoist/mygo/ui`）：Go 代码逐帧声明，MyGo 在 GPU 上绘制，没有网页、没有 webview、没有绑定层。控件与配色来自**主题**，主题跟随桌面（外观、强调色、字号、对比度、减弱动效），因此观感与系统一致，而不是用 CSS 去模仿原生。

## 1. 原则

1. **状态可见**：任何时刻都能看出「队列里有什么、跑到哪一步、结果如何」，不靠用户从日志里推断。
2. **控件即系统控件**：优先用 `ui.Sidebar`/`ui.Form`/`ui.List`/`ui.Progress`/`ui.Switch`/`ui.Badge` 等原生控件，不手搓外观。
3. **颜色有语义**：成功/警告/失败分别取主题的 `Success`/`Warning`/`Danger`；未发生的事以 `TextMuted` 低调呈现，已发生的事以彩色胶囊强调。
4. **无窗口可测**：视图只依赖状态，能用 `ui.NewTester` 在内存里渲染并断言，不引入需要窗口的测试。

## 2. 窗口与外壳

窗口选项（`internal/desktop/app.go` 的 `openWindow`）：

| 项 | 值 | 理由 |
| --- | --- | --- |
| `Title` | `NCMconverter` | |
| `Width/Height` | `900×640` | 侧栏 + 内容区的最小舒适尺寸 |
| `MinWidth/MinHeight` | `680×460` | 再窄侧栏与表单会挤 |
| `StateKey` | `main` | 记住位置与大小 |
| `TitleBarStyle` | `mygo.TitleBarHiddenInset` | 标题栏由视图画（内嵌） |
| `TitleBarHeight` | `52` | 与视图自绘的标题栏一致 |
| `Vibrancy` | `mygo.VibrancySidebar` | 侧栏用系统材质 |
| `Content` | `ui.View(a.view)` | 原生视图 |

- **内嵌标题栏**：视图用 `Context.TitleBar()` 取窗口控件占用的空间（`Height`/`Left`/`Right`），标题栏行按它留白，并用 `Element.DragWindow()` 让空白处可拖动窗口。macOS 上红绿灯内嵌在侧栏之上；Windows/Linux 上窗口控件落在内容区，`bar.Right` 自动把它们让开。
- **材质**：`Vibrancy` 只在视图**不画背景**处透出。因此 `Context.Vibrancy()` 为真时根元素置透明、侧栏不画背景；为假时（Linux 没有材质）侧栏退化为不透明的 `Surface` 背景，内容区始终不透明。**这样一处代码在三平台都成立**。
- 窗口任意位置接收文件拖放（`Window.OnFileDrop`），也保留视图内的拖放区（`Element.DroppedFiles`）。

## 3. 布局

```
Row（Stretch）
├─ 侧栏（宽 216；有材质时不画背景，否则 Surface）
│   ├─ 标题栏高度的空白带（DragWindow，让开窗口控件）
│   └─ ui.Sidebar（Grow）
│        ├─ SidebarItem「转换队列」icon + 数量 Badge
│        ├─ SidebarItem「选项」
│        └─ SidebarItem「日志」icon + 数量 Badge
└─ 内容区（Grow，MinWidth 0，Background）
     ├─ 自绘标题栏（高 52，DragWindow）
     │    ├─ 分区名（Bold）
     │    ├─ 运行状态（运行中：Spinner + `d / t`；结束后：着色 Badge）
     │    ├─ Button「取消」（仅运行中）
     │    └─ PrimaryButton「开始转换」（运行中或队列为空时 Disabled）
     └─ Divider + 分区内容
          ├─ 转换队列：操作行 → 状态卡 → 来源列表（Grow）/ 拖放区 → 统计行
          ├─ 选项：   ui.Scroll → ui.Fieldset → ui.Form
          └─ 日志：   ui.List（`FollowEnd`，Grow）→ 条数
```

- **侧栏**用 `ui.Sidebar`，三个分区常驻可见；数量徽标让用户不切分区也能知道队列与日志的规模。
- **内容区宽度**在选项分区限制为 `MaxWidth 680`，避免表单被拉得过宽（readable measure）。
- **主题微调**：`Context.SetTheme` 把 `Radius` 调为 8，其余沿用默认主题（默认主题已跟随系统强调色、字号与对比度）。

## 4. 转换队列的状态流转

这是本界面的核心：**每个来源**在列表里显示它已达到的阶段，配合**运行状态卡**与**标题栏状态**，任何时刻都能读出「跑到哪、结果如何」。

### 4.1 来源的阶段

每个队列项是一个「来源」（一个文件或一个目录），`queueEntry` 记录它与本次运行对它的计数：

| 阶段 | 文案 | 标记 | 颜色 |
| --- | --- | --- | --- |
| 尚未统计 | `待处理` | 圆环 | `TextMuted` |
| 等待中 | `待处理 · N 个` | 圆环 | `TextMuted` |
| 进行中 | `转换中 · d/t` | Spinner | `Accent` |
| 完成 | `N 已转换`（另有跳过/失败时追加 `· M 跳过 · K 失败`） | 对勾 | `Success`（有失败则 `Danger`） |
| 试运行完成 | `N 个将转换` | 对勾 | `Success` |
| 无内容 | `无 .ncm` | 空箱 | `TextMuted` |
| 路径不可用 | `路径不可用` | 感叹号 | `Danger` |

**归属**：`runner.go` 的 `scanBySource` **逐个来源**单独展开，把每个容器映射回它来自的来源；与 `app.Scan` 的去重顺序一致（**先到先得**，重叠输入只归第一个来源），因此一次运行的每条 `Report` 都能唯一对应到一个队列项。某个来源打不开时只把它标为「路径不可用」，**不会把整行统计清零**；运行本身仍会照常报告该输入的错误。

### 4.2 运行状态卡

队列分区顶部，**运行一开始就出现、结束后保留**（结果不会随日志滚走）：

- 运行中：`ui.Progress`（`转换进度`）；计划数未知时用不确定长度（负值）。
- 计数胶囊：`已转换 N` / `已跳过 N` / `失败 N`——为零的画成低调的 `TextMuted`，非零的分别着色 `Success`/`Warning`/`Danger`。
- 汇总文案：`共 N 个 · 1.2s`，试运行/取消时追加 `· 试运行` / `· 已取消`。
- `打开输出目录` 按钮（无输出目录时禁用；试运行下不显示）。
- 运行级错误（输入不可读等）以 `Danger` 直接列在卡内。

### 4.3 标题栏状态

运行状态在**标题栏**也有一份，因此切到「选项」或「日志」分区同样看得到：

| 状态 | 显示 |
| --- | --- |
| 运行中 | Spinner + `d / t` |
| 完成 | `已完成`（`Success`） |
| 完成但有失败 | `完成 · N 个失败`（`Danger`） |
| 已取消 | `已取消`（`Warning`） |
| 试运行完成 | `试运行完成`（`TextMuted`） |
| 未能开始 | `运行失败`（`Danger`） |

## 5. 各分区的控件

| 分区 | 控件 |
| --- | --- |
| 转换队列 | `Button`（添加文件… / 添加文件夹… / 清空）、状态卡（`Progress` + `Badge` ×3 + `Button`）、`List`（每行：状态标记 + 路径 `SingleLine` + 阶段文案 + `移除`）、`Text`（统计行） |
| 选项 | `Fieldset` → `Form` → `Field`：`TextInput`（输出目录 + `选择…`）、`TextInput`（命名模板，`Description` 列出占位符）、`NumberInput`（depth 0–99、threads 1–64）、`Switch`（写入标签、跳过已存在、试运行） |
| 日志 | `List`（`FollowEnd`；每行：着色 `Badge` + 路径/`源 → 目标` `Selectable` + 失败原因） |
| 空态 | 转换队列为空时用虚线边框拖放区（`FileDragOver` 时边框转 `Accent`）；日志为空时居中提示 |

## 6. 输入与快捷键

| 操作 | 入口 |
| --- | --- |
| 添加文件 | `CmdOrCtrl+O`、菜单「文件」、分区按钮 |
| 添加文件夹 | `CmdOrCtrl+Shift+O`、菜单「文件」、分区按钮 |
| 开始转换 | `CmdOrCtrl+Return`、菜单「转换」、标题栏主按钮 |
| 取消 | 标题栏「取消」、菜单「转换」 |
| 打开输出目录 | 状态卡按钮、菜单「转换」 |
| 添加/移除/清空来源 | 分区按钮、行内「移除」、行右键菜单 |

运行期间队列与选项被禁用（`Disabled(running)`），避免与进行中的批次冲突；对话框（系统面板）在后台 goroutine 里调用，完成后经 `Window.Update` 回写状态。

## 7. 无障碍

- 纯图标按钮（侧栏项、状态标记）都带 `Label`；分区与列表带 `Label`。
- 颜色不是唯一信号：阶段既有文案也有标记形状，胶囊既有颜色也有文字。
- 主题跟随 `Preferences`：`TextScale` 放大字号、`HighContrast` 加深边框与次要文字、`ReduceMotion` 让动效立即到位——都是默认主题的行为，无需额外代码。

## 8. 文案表（中文）

| 位置 | 文案 |
| --- | --- |
| 分区 | 转换队列 / 选项 / 日志 |
| 操作 | 添加文件… / 添加文件夹… / 清空 / 移除 / 选择… / 开始转换 / 取消 / 打开输出目录 |
| 阶段 | 待处理 / 待处理 · N 个 / 转换中 · d/t / N 已转换 · M 跳过 · K 失败 / N 个将转换 / 无 .ncm / 路径不可用 |
| 运行状态 | 转换中 / 已完成 / 完成 · N 个失败 / 已取消 / 试运行完成 / 运行失败 |
| 结果标签 | 将转换 / 已转换 / 已跳过 / 失败 |
| 占位 | 把 .ncm 文件或文件夹拖进窗口 / 还没有记录。 |
| 选项 | 输出目录（留空则与源文件同目录）/ 命名模板 / 目录层级 depth / 并发数 threads / 写入标签与封面 / 跳过已存在的文件 / 试运行（只列出，不写入） |

## 9. 明确不做

- **不做像素/快照测试**：只断言结构与状态（`ui.NewTester`），见 [测试计划 §7](../04-testing/test-plan.md)。
- **不做自定义绘制取代原生控件**：外观全部交给主题与原生控件。
- **不做动画炫技**：只在有意义处用过渡（行的 hover 颜色 120ms），并遵守 `ReduceMotion`。
- **不做托盘/常驻/目录监听/内置播放**：沿用 [PRD §3 非目标](../01-requirements/PRD.md)。
