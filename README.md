# NCMconverter

[![CI](https://github.com/chilfish/NCMconverter/actions/workflows/ci.yml/badge.svg)](https://github.com/chilfish/NCMconverter/actions/workflows/ci.yml)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/chilfish/NCMconverter)](https://pkg.go.dev/github.com/chilfish/NCMconverter)

NCMconverter 将网易云音乐的 `.ncm` 文件转换为可播放的 mp3 或 flac，并保留其中的
元数据与封面。

格式的最初解析参考了 [yoki123/ncmdump][1]。本实现直接解析容器格式，并支持并发
转换多个文件。

## 安装

    go install github.com/chilfish/NCMconverter/cmd/ncmconverter@latest

或在源码目录下构建：

    make build          # 生成 bin/ncmconverter

## 使用

    ncmconverter [options] <files/dirs>

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-o`、`--output` | 与源文件同目录 | 转换结果的输出目录。 |
| `-t`、`--tag` | `true` | 是否把元数据与封面写入转换后的文件。用 `--tag=false` 关闭。 |
| `-d`、`--depth` | `0` | 在传入目录下向下查找的层数。`0` 只处理该目录下的直接子文件。 |
| `-n`、`--threads` | `10` | 同时转换的最大文件数。 |
| `-h`、`--help` | | 显示帮助。 |
| `-v`、`--version` | | 显示版本。 |

`--deepth` 与 `--thread` 作为 `--depth`、`--threads` 的别名继续可用。

```sh
# 转换单个文件，结果写在源文件旁边
ncmconverter song.ncm

# 向下两层转换整个目录树，输出到 ./out
ncmconverter -d 2 -o ./out ~/Music/ncm

# 不写入任何元数据
ncmconverter --tag=false song.ncm
```

转换结果沿用源文件名，扩展名取自其中音频的实际格式：`song.ncm` 会变成
`song.mp3` 或 `song.flac`。

格式优先采用容器元数据中声明的值，缺失时再由音频开头的魔数判定；当元数据声明的
格式与音频实际不符时，以元数据为准。

单个文件转换失败只会被记录并跳过，不会中断整批任务；只要有文件失败，进程最终仍
以非零状态退出。

## 开发

```sh
make test          # go test ./...
make test-race     # go test -race ./...
make vet
make fmt-check
make lint          # 需要 golangci-lint v2
```

### 测试

测试套件不依赖任何真实 `.ncm` 素材：它按格式定义在内存中构造容器，这样读取逻辑
里的 bug 不会同时藏进本该抓住它的夹具里。

把 `.ncm` 文件放进 `testdata/` 会额外启用一个端到端测试，它会真实转换该文件，并
逐字节校验结果中的音频与容器内容完全一致（含标签）。这些文件不会被提交。

## 目录结构

| 路径 | 内容 |
| --- | --- |
| `cmd/ncmconverter` | 命令行入口。 |
| `internal/app` | 编排：转换哪些文件、并发多少。 |
| `internal/ncm` | 容器解析。 |
| `internal/converter` | 解密与元数据解码。 |
| `internal/tag` | 向 mp3 与 flac 写入标签。 |
| `internal/ncmtest` | 供测试使用的合成容器。 |

## 许可

[MIT](LICENSE)

[1]: https://github.com/yoki123/ncmdump
