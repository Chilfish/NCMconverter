# 需求文档（Requirements）

> 功能级需求文档区。多数功能以「领域文档 + 功能→文件定位」（[`../features/`](../features/)，见 [`../INDEX.md`](../INDEX.md)）承载；**当功能足够复杂、需要独立 PRD / 用户故事时**，在此建独立需求目录。

## 约定

- 复杂功能按如下目录结构组织：

```
docs/requirements/<feature>/
├── 01-requirements/PRD.md
├── 01-requirements/user-stories.md
├── 02-design/architecture.md · api-contract.md
├── 03-implementation/coding-standards.md · phased-plan.md
├── 04-testing/test-plan.md
├── 05-review/design-decisions.md
└── 06-community/CHANGELOG.md · CONTRIBUTING.md
```

- 需求变更时同步更新对应文档（PRD / 设计 / 测试），并记录到 [`../development-log/`](../development-log/)。
- 验收标准应落到可运行的测试（见 [`../engineering/testing.md`](../engineering/testing.md)），并在 [`../features/`](../features/) 留领域文档。

## 现有功能级需求文档

| 功能                                   | 目录                             | 状态                  |
| -------------------------------------- | -------------------------------- | --------------------- |
| 桌面应用（单二进制双模式，MyGo）        | [`desktop/`](desktop/)            | 已定稿，待实现        |

### [`desktop/`](desktop/) 目录内容

```
docs/requirements/desktop/
├── 01-requirements/PRD.md              # 目标、非目标、功能/非功能需求、验收标准、风险
├── 01-requirements/user-stories.md     # 角色与用户故事（含反向故事）
├── 02-design/architecture.md           # 结构、依赖方向、双模式入口、Windows 垫片、构建契约
├── 02-design/api-contract.md           # 绑定 API 与生成的 TypeScript 表面
├── 03-implementation/coding-standards.md
├── 03-implementation/phased-plan.md    # 分阶段实施步骤（含提交划分与交付物清单）
├── 04-testing/test-plan.md             # 新增测试、手工冒烟、映射表
└── 05-review/design-decisions.md       # DD-1…DD-7（含被否方案与否决反转）
```
