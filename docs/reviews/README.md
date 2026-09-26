# 代码审查记录（Reviews）

> 代码审查（含 AI 辅助审查）的记录区。每次**里程碑 / 阶段 / 工作流完成**后做一次 review，把问题沉淀到带日期的审查记录，避免同一类问题反复出现。

## 约定

- 文件名：`review-YYYY-MM-DD-<主题>.md`（如 `review-2026-09-27-v0.1.0.md`）
- 每条问题带编号前缀（`P0/P1/P2/P3`），标注文件与建议修法
- 审查视角可含：正确性、边界、错误处理、资源释放、可测性、依赖风险、文档一致性
- 已修问题在原记录中勾选并注明 commit；同类问题升级为 postmortem，或同步到 [`../planning/backlog.md`](../planning/backlog.md)

## 审查清单（写码/审查时逐项过）

- [ ] 遵循 [`../engineering/code-style.md`](../engineering/code-style.md)（错误 `%w` 包装、`errors.Is` 判定、注释写 why）
- [ ] 解析/解密/标签逻辑覆盖边界（尾部块、截断、空值、坏 padding、magic）
- [ ] 资源：新增的 `os.Open`/标签句柄都有 `Close`，`defer` 与 `Save` 不冲突
- [ ] 新行为有测试；修 bug 有能变红的回归测试；夹具走 `ncmtest`
- [ ] 不假定 `Meta.Album`/`meta` 一定存在（nil 安全）
- [ ] 上游库/格式文档的假设已核实（见 [postmortem 012](../postmortem/012-upstream-contract-traps.md)）
- [ ] 行为变化更新 `docs/`；新 Bug 模式写 postmortem；用户可见变化写 `CHANGELOG.md`

## 索引

| 日期       | 主题                       | 主要问题                                                                                       |
| ---------- | -------------------------- | ---------------------------------------------------------------------------------------------- |
| 2026-09-27 | v0.1.0 代码基线审查        | 无阻断项；P2×4（覆盖率无阈值 / fixture 版权 / reserved 字段语义未证实 / 上游库句柄与临时文件残留）、P3×3；P2 项已回填 backlog |
