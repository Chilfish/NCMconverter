# NCMconverter

[![CI](https://github.com/chilfish/NCMconverter/actions/workflows/ci.yml/badge.svg)](https://github.com/chilfish/NCMconverter/actions/workflows/ci.yml)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/chilfish/ncmconverter)](https://pkg.go.dev/github.com/chilfish/ncmconverter)

NCMconverter 将网易云音乐的 `.ncm` 文件转换为可播放的 mp3 或 flac，并保留其中的元数据与封面。

格式的最初解析参考了 [yoki123/ncmdump][1]。本实现直接解析容器格式，并支持并发转换多个文件。

## 安装

```sh
go install github.com/chilfish/ncmconverter/cmd/ncmconverter@latest
```

或在源码目录下构建：

```sh
make build          # 生成 bin/ncmconverter
```

## 使用

```sh
ncmconverter [options] <files/dirs>
```

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-o`、`--output` | 与源文件同目录 | 转换结果的输出目录。 |
| `--output-template` | 空（沿用源文件名） | 结果文件的命名模板，见下文。 |
| `-t`、`--tag` | `true` | 是否把元数据与封面写入转换后的文件。用 `--tag=false` 关闭。 |
| `-d`、`--depth` | `0` | 在传入目录下向下查找的层数。`0` 只处理该目录下的直接子文件。 |
| `-n`、`--threads` | `10` | 同时转换的最大文件数。 |
| `--dry-run` | `false` | 只列出将要转换的文件，不写任何文件。 |
| `--skip-existing` | `false` | 目标文件已存在时跳过；默认是覆盖。 |
| `-q`、`--quiet` | `false` | 只输出警告与错误。 |
| `-h`、`--help` | | 显示帮助。 |
| `-v`、`--version` | | 显示版本。 |

`--deepth` 与 `--thread` 作为 `--depth`、`--threads` 的别名继续可用。

`--output-template` 支持的占位符：`{name}`（源文件名，不含扩展名）、`{title}`、`{artist}`（第一位艺人）、`{album}`、`{id}`（曲目 ID）、`{format}`。扩展名始终取自音频的实际格式，无法用模板改写。元数据里的路径分隔符会被替换掉，模板也不能把文件写到输出目录之外。

```sh
# 转换单个文件，结果写在源文件旁边
ncmconverter song.ncm

# 向下两层转换整个目录树，输出到 ./out
ncmconverter -d 2 -o ./out ~/Music/ncm

# 按「艺人 - 标题」命名，但不动已经转换过的文件
ncmconverter -o ./out --output-template '{artist} - {title}' --skip-existing ~/Music/ncm

# 先看看会转换哪些文件
ncmconverter --dry-run -d 2 ~/Music/ncm

# 不写入任何元数据
ncmconverter --tag=false song.ncm
```

转换结果沿用源文件名，扩展名取自其中音频的实际格式：`song.ncm` 会变成 `song.mp3` 或 `song.flac`。

格式优先采用容器元数据中声明的值，缺失时再由音频开头的魔数判定；当元数据声明的格式与音频实际不符时，以元数据为准。

容器本身不保存歌词。如果源文件旁边有同名的 `.lrc`（例如 `song.ncm` 与 `song.lrc`），转换时会把它的内容一并嵌入输出的 mp3（`USLT` 帧）或 flac（`LYRICS` 字段）。没有 `.lrc` 是正常情况，不影响转换。文本按 UTF-8 读取，带 BOM 或 GBK 编码的文件会自动处理；内容原样嵌入，因此网易云在开头写入的 JSON 曲目信息也会保留。

单个文件转换失败只会被记录并跳过，不会中断整批任务；只要有文件失败，进程最终仍以非零状态退出。

## 开发

```sh
make test          # go test ./...
make test-race     # go test -race ./...
make coverage      # 写入 coverage.out
make vet
make fmt-check
make lint          # 需要 golangci-lint v2
```

### 测试

测试套件不依赖任何真实 `.ncm` 素材：它按格式定义在内存中构造容器，这样读取逻辑里的 bug 不会同时藏进本该抓住它的夹具里。

把 `.ncm` 文件放进 `testdata/` 会额外启用一个端到端测试，它会真实转换该文件，并逐字节校验结果中的音频与容器内容完全一致（含标签）。仓库自带一份这样的素材，其来源与版权说明见 [`testdata/README.md`](testdata/README.md)。

## 目录结构

| 路径 | 内容 |
| --- | --- |
| `cmd/ncmconverter` | 命令行入口。 |
| `internal/app` | 编排：转换哪些文件、并发多少。 |
| `internal/ncm` | 容器解析。 |
| `internal/converter` | 解密与元数据解码。 |
| `internal/tag` | 向 mp3 与 flac 写入标签。 |
| `internal/ncmtest` | 供测试使用的合成容器。 |
| `docs/` | 项目文档体系，入口见 [`docs/INDEX.md`](docs/INDEX.md)。 |

## 许可

[MIT](LICENSE)

[1]: https://github.com/yoki123/ncmdump
