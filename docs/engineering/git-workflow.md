# Git 开发流程

> 最后更新：2026-09-27

## 1. 分支模型

采用简化的 **Trunk-Based Development**：`main` 始终可发布，功能/修复从 `main` 切出、合并后删除。

| 分支类型     | 命名格式                  | 用途               |
| ------------ | ------------------------- | ------------------ |
| `main`       | —                         | 稳定分支           |
| `feat/*`     | `feat/lyrics`             | 功能开发           |
| `fix/*`      | `fix/music-tail`          | Bug 修复           |
| `refactor/*` | `refactor/unify-media-url`| 重构               |
| `docs/*`     | `docs/architecture`       | 文档               |
| `release/*`  | `release/v0.2.0`          | 发布准备           |

## 2. 提交信息（Conventional Commits）

格式 `type: subject`，破坏性变更在正文写明并加 `BREAKING CHANGE:` 脚注。

| 类型       | 用途                     |
| ---------- | ------------------------ |
| `feat`     | 新增功能                 |
| `fix`      | 修复缺陷                 |
| `docs`     | 只改文档                 |
| `test`     | 只改测试                 |
| `refactor` | 不改变行为的重构         |
| `chore`    | 构建、依赖、配置等杂项   |
| `build`    | 构建/打包                |
| `ci`       | 只改 CI 配置             |

规则（详见 [`CONTRIBUTING.md`](../../CONTRIBUTING.md)）：

- 主题行用**英文祈使句**，说明「做了什么」而非「改了哪一行」；首字母小写、不加句号。
- **先想 commit message，再动工**；一个 PR 只做一件事，顺带的重构拆成单独 PR。
- 行为变化**必须**同步 `README.md` / `docs/`，用户可见变化写进 `CHANGELOG.md` 的 Unreleased。
- Release 说明按类型分组并**过滤** `chore`/`ci`/`docs`/`test`/`build`/`refactor` 与任何 `(release)` 提交（见 [`../features/release/release-artifacts.md`](../features/release/release-artifacts.md) §2）。

## 3. Pull Request

1. 从 `main` 切分支（命名看得出主题）。
2. 提交前本地跑通门禁：`make fmt-check vet test-race`（并确保 `make build` 通过）。
3. 按 [PR 模板](../../.github/PULL_REQUEST_TEMPLATE.md) 填写「做了什么 / 关联 issue / 检查清单」。
4. 至少 1 人 Approve + CI 全绿后合并（Create a Merge Commit）。

### 审查清单

- [ ] 正确性：解析/解密/标签逻辑覆盖边界（尾部块、截断、空值、坏 padding、magic）
- [ ] 资源：新增的 `os.Open`/标签句柄都有 `Close`，`defer` 与 `Save` 不冲突
- [ ] 错误：用 `%w` 包装，`errors.Is` 判定，非致命路径有日志
- [ ] 测试：新行为有测试；修 bug 有能让它变红的回归测试；夹具走 `ncmtest`
- [ ] 文档：行为变化更新 `docs/`；新 Bug 模式写 `docs/postmortem/`
- [ ] 用户可见变化写进 `CHANGELOG.md` 的 Unreleased

## 4. Issue

- Bug 用 [Bug 报告模板](../../.github/ISSUE_TEMPLATE/bug_report.yml)（必填版本、系统、复现命令，并询问源文件旁边是否有同名 `.lrc`）。
- 新功能用 [功能请求模板](../../.github/ISSUE_TEMPLATE/feature_request.yml)。
- **安全问题不走走公开 issue**：改用 [私密安全公告](https://github.com/chilfish/NCMconverter/security/advisories/new)，详见 [`SECURITY.md`](../../SECURITY.md)。
- 空白 issue 已关闭，必须走模板。

## 5. 依赖更新

[`dependabot.yml`](../../.github/dependabot.yml) 每周分别更新 `gomod` 与 `github-actions`，各自用 `groups` 合并成**一个** PR。升级依赖后必须重跑门禁；`go-flac` 与 `id3v2` 的已知陷阱见 [postmortem 012](../postmortem/012-upstream-contract-traps.md)。

## 6. 版本发布

版本号遵循 [SemVer 2.0.0](https://semver.org/lang/zh-CN/)。发布由 tag 触发：

1. 从 `main` 创建 `release/x.y.z` 分支。
2. 更新 `CHANGELOG.md`（把 Unreleased 归入新版本并加日期）。
3. 按 [`release-checklist.md`](release-checklist.md) 过本地门禁与快照试产。
4. 合并回 `main`。
5. 在 `main` 上打 tag：`git tag vX.Y.Z && git push origin vX.Y.Z`。
6. GitHub Actions 的 `release.yml` 调用 GoReleaser 产出各平台归档、deb/rpm 与 `checksums.txt` 并创建 Release。

> **tag 只打在门禁绿的 commit 上**；版本号严格递增，禁止回退。
