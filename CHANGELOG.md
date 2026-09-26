# 更新日志

本文件记录面向用户的变化。格式参考 [Keep a Changelog][kac]，版本号遵循[语义化版本][semver]。

## [Unreleased]

尚未发布任何带 tag 的版本，以下内容都在 `main` 上。

### Added

- 转换时会把与源文件同名的 `.lrc` 嵌入产物：mp3 写入 `USLT` 帧，flac 写入 `LYRICS` 字段。歌词侧车文件不存在是正常情况，不影响转换；带 BOM 或 GBK 编码的文件会被自动处理。
- 跨平台发布产物：`v*` tag 触发 GoReleaser，交叉编译 linux/darwin/windows × amd64/arm64，附带 `checksums.txt`。
- CI 增加 macOS 与 Windows 的测试矩阵，以及覆盖率报告。
- 补齐开源社区文件：贡献指南、行为准则、安全政策、issue 与 PR 模板、dependabot、`.editorconfig`/`.gitattributes`，以及 `docs/architecture.md`。

### Changed

- `--depth` 语义变更（**破坏性**）：以前 `--depth 0` 传目录等于什么都不做，现在表示"只处理该目录下的直接子文件"。`--deepth`、`--thread` 仍作为别名可用。
- `--version` 现在同时输出构建时的 commit 与时间，便于定位运行中的二进制。

[kac]: https://keepachangelog.com/zh-CN/1.1.0/
[semver]: https://semver.org/lang/zh-CN/
