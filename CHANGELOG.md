# 更新日志

本文件记录面向用户的变化。格式参考 [Keep a Changelog][kac]，版本号遵循[语义化版本][semver]。

## [Unreleased]

### Added

- 桌面应用（单二进制双模式）：同一个 `ncmconverter` 无参数启动即打开窗口，带参数仍是原来的命令行工具。窗口支持添加文件/文件夹、拖拽入队、逐文件进度（已转换/已跳过/失败）、取消与「打开输出目录」，并可设置全部 CLI 选项（输出目录、命名模板、`depth`、`threads`、写标签、跳过已存在、试运行）。基于 [MyGo](https://github.com/egoist/mygo)（纯 Go、无 cgo），与 CLI 共用同一套转换逻辑。
- Windows 控制台垫片：GUI 子系统构建从终端调用时仍能正常输出（`AttachConsole`）。
- `internal/app` 增加逐文件回调 `Options.OnFile` 并导出 `Scan`，供桌面展示进度与预览工作量；新增 `internal/desktop` 绑定服务，前端类型由 `mygo generate` 生成并提交。

### Changed

- 发布链路从 GoReleaser/nfpm 改为 `mygo build`：删除 `.goreleaser.yaml`，产物变为 macOS `.app`+`dmg`、Windows `exe`+NSIS 安装器、Linux `tar.gz`（含 `install.sh`）+`deb`。**不再产出 `rpm` 与 `checksums.txt`**（恢复办法见 backlog 跟进项）。
- `make build` 不再等同发布产物：它是普通 `go build`，**不内嵌前端**，桌面模式打不开页面；桌面开发与打包改走 `mygo dev` / `mygo build`（`make desktop`）。
- CI 增加三平台桌面对应 job（`bun install` + `make typecheck` + 校验生成的 `desktop/src/mygo.ts` + `make desktop`）；Go 门禁仍不依赖 bun。
- Linux 的 `deb` 会声明依赖 GTK/WebKitGTK（仅桌面模式需要）；命令行模式不受影响。

## [0.1.0] - 2026-09-27

首个正式版本：把网易云音乐的 `.ncm` 容器转换成可播放的 mp3 或 flac，并保留其中的元数据、封面与歌词。

### Added

- 转换时会把与源文件同名的 `.lrc` 嵌入产物：mp3 写入 `USLT` 帧，flac 写入 `LYRICS` 字段。歌词侧车文件不存在是正常情况，不影响转换；带 BOM 或 GBK 编码的文件会被自动处理。
- CLI 增加四个开关：`--output-template` 按 `{name}`/`{title}`/`{artist}`/`{album}`/`{id}`/`{format}` 命名结果、`--dry-run` 只列出将要转换的文件、`--skip-existing` 跳过已存在的产物、`--quiet` 只输出警告与错误。
- 跨平台发布产物：`v*` tag 触发 GoReleaser，交叉编译 linux/darwin/windows × amd64/arm64，附带 `checksums.txt`。
- 发布产物同时生成 Linux 的 `deb` 与 `rpm` 安装包（amd64/arm64），二进制装到 `/usr/bin/ncmconverter`。
- CI 增加 macOS 与 Windows 的测试矩阵，以及覆盖率报告。
- 补齐开源社区文件：贡献指南、行为准则、安全政策、issue 与 PR 模板、dependabot、`.editorconfig`/`.gitattributes`，以及 `docs/architecture.md`。

### Changed

- 模块路径改为全小写 `github.com/chilfish/ncmconverter`，符合 Go 的路径惯例。导入路径与 `go install` 命令相应变化；本版本即首个发布版本，旧路径从未对外发布过，因此未提供重定向。
- `--depth` 语义变更（**破坏性**）：以前 `--depth 0` 传目录等于什么都不做，现在表示"只处理该目录下的直接子文件"。`--deepth`、`--thread` 仍作为别名可用。
- `--version` 现在同时输出构建时的 commit 与时间，便于定位运行中的二进制。

[Unreleased]: https://github.com/Chilfish/NCMconverter/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Chilfish/NCMconverter/releases/tag/v0.1.0

[kac]: https://keepachangelog.com/zh-CN/1.1.0/
[semver]: https://semver.org/lang/zh-CN/
