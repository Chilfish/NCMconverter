# 贡献指南

感谢你愿意为 NCMconverter 出力。本文档说明本地开发、测试与提交的约定；参与前请先阅读[贡献者公约](CODE_OF_CONDUCT.md)。

## 环境要求

- Go：版本以 [`go.mod`](go.mod) 里的 `go` 指令为准。
- `make`：可选，但大多数常用命令都收在 [Makefile](Makefile) 里。
- `golangci-lint` v2：运行 `make lint` 时需要，版本与 CI 中锁定的保持一致。

## 本地开发

```sh
make build          # 编译到 bin/ncmconverter
make run ARGS="--help"
make test           # go test ./...
make test-race      # go test -race ./...
make coverage       # 写入 coverage.out
make vet
make fmt            # gofmt -w .
make fmt-check      # 只检查，不修改
make lint           # 需要 golangci-lint v2
```

不想用 `make` 时，直接运行对应的 `go` 命令即可，Makefile 没有做额外的事情。

## 测试

测试套件**不依赖任何真实 `.ncm` 素材**：它按格式定义在内存中构造容器（见 `internal/ncmtest`）。这样做是有意的——如果读取逻辑有 bug，不该让本该抓住它的夹具一起出错。

`testdata/` 下若存在 `.ncm` 文件，会额外启用一个端到端测试，它会真实转换该文件，并逐字节校验结果中的音频与容器内容一致（含标签）。请**不要**为了让测试通过而修改这些夹具；它们来自真实文件，是回归的锚点。

新增行为时，请同时补上：

- 单元测试，覆盖正常路径与边界（空值、缺失字段、损坏输入）。
- 端到端测试，用合成容器证明产物确实可读、音频确实没被动过。

## 提交信息

提交信息遵循 [Conventional Commits][cc]，格式为 `type: subject`，常用类型：

| 类型 | 用途 |
| --- | --- |
| `feat` | 新增功能 |
| `fix` | 修复缺陷 |
| `docs` | 只改文档 |
| `test` | 只改测试 |
| `refactor` | 不改变行为的重构 |
| `chore` | 构建、依赖、配置等杂项 |
| `ci` | 只改 CI 配置 |

主题行用祈使句、说明"做了什么"而非"改动了哪一行"。破坏性变更在正文里写清影响，并加上 `BREAKING CHANGE:` 脚注。

```
feat: embed a lyrics sidecar into converted files

容器本身不保存歌词，改为读取与源文件同名的 .lrc。
```

## 分支与 Pull Request

1. 从 `main` 切出分支，命名随意但能看出主题（如 `feat/lyrics`）。
2. 一个 PR 只做一件事；顺带的重构请拆成单独的 PR。
3. 提交前本地跑通 `make fmt-check vet test-race`。
4. PR 描述里按模板填写"做了什么""关联 issue"与检查清单；有用户可见的变化时，同步更新 `README.md`、`docs/` 和 `CHANGELOG.md`（文档入口见 [`docs/INDEX.md`](docs/INDEX.md)）。

## 代码风格

- 提交前用 `gofmt` 格式化，`go vet` 与 `golangci-lint` 不得报错。
- 注释与提交信息用英文，面向用户的文档（`README.md`、`docs/`）用中文。
- 注释解释**为什么**这么做，而不是复述代码在做什么；测试里的注释尤其要写清楚它锁住的是哪个回归。

## 发布

发布由 tag 触发：推一个 `v*` 形式的 tag，GitHub Actions 会调用 [GoReleaser](.goreleaser.yaml) 交叉编译 linux/darwin/windows × amd64/arm64，生成 `tar.gz`（Windows 为 `zip`）与 `checksums.txt` 并上传到 Release。

[cc]: https://www.conventionalcommits.org/zh-hans/
