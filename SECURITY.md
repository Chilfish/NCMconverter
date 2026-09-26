# 安全政策

## 支持的版本

安全修复只会落在**最新的发布版本**上。请先用 `ncmconverter --version` 确认版本，并尽量在最新版上复现问题。

## 报告漏洞

**请不要用公开 issue 报告安全问题**，那会让问题在被修复前就暴露出来。请改用以下任一私密渠道：

1. [GitHub 私密安全公告][advisory]（推荐）。
2. 邮件至 **chill4fish@gmail.com**。

报告中请尽量包含：

- `ncmconverter --version` 的输出，以及操作系统。
- 问题类型（例如崩溃、越界读写、路径穿越）与影响范围。
- 可复现的最小步骤，或一个能触发问题的文件。
- 若已知，附上修复建议。

## 处理流程

收到报告后，我们会确认问题、评估影响，并在某个发布版本中修复。修复发布后，如果你愿意，我们会在公告中致谢。

[advisory]: https://github.com/chilfish/NCMconverter/security/advisories/new
