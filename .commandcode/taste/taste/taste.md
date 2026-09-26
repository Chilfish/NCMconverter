# Taste
- Writes in Chinese and expects responses in Chinese, including plans, status updates and summaries. Confidence: 0.85
- Wants legacy Go projects brought up to current Go conventions ("按最新的golang项目规范") — idiomatic error wrapping with `%w`, `log/slog`, generics where they fit, `io.ReadFull`, `os.ReadDir`, and removing deprecated `io/ioutil` — rather than minimally patching what already builds. Confidence: 0.7
- Prefers the newest versions across the board: the `go` directive bumped to match the locally installed toolchain, and dependencies taken to their latest major lines even when they carry breaking changes (e.g. urfave/cli v2→v3, go-flac v0.3→v2), instead of staying on conservative baselines. Confidence: 0.6
- Prefers the conventional `cmd/` + `internal/` module layout even for small single-binary CLIs. Confidence: 0.6
- Prefers permissive MIT licensing: had an inherited GPL-3.0 `LICENSE` replaced with MIT and the README's license section updated to match. Confidence: 0.6
- Expects thorough automated testing (unit tests plus end-to-end runs) and explicit proof that artifacts are actually correct/usable — e.g. output format magic bytes, readable tags, byte-for-byte preserved audio — not merely a green build. Confidence: 0.65
