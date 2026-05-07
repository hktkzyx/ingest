# Changelog

本项目所有显著变更都会记录在本文件中。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Changed

- **设备识别规则不再写死**：内置 `BuiltinRules` 已删除。规则从配置文件 `$XDG_CONFIG_HOME/ingest/devices.yaml`（默认 `~/.config/ingest/devices.yaml`）加载；首次运行时自动把内嵌的出厂默认（Sony / DJI Pocket3）写到该路径，之后所有读取都走文件，可任意编辑增删。
- **CLI 路径展开**：`--source`、`--target`、`--db`、`--devices` 全部经过 `expandPath`：`~` 和 `$VAR` 展开 + 相对路径转绝对路径 + `filepath.Clean`。Windows 上 `filepath` 包原生处理 `\`、盘符、UNC。
- **默认模板改用 `{device_name}`**：从 `…/origin-{device_id}` 改为 `…/origin-{device_name}`；Context 构造时 `device_name` 中的空格自动替换为 `_`，避免目录名带空格带来的 shell/同步工具兼容问题。`{device_id}` 仍可在自定义模板中使用。
- **`ingest devices list`** 现在显示规则来源路径，方便定位用户在编辑哪份配置。

### Added

- `--devices <path>` 全局 flag，覆盖默认配置文件位置。
- 直接依赖 `gopkg.in/yaml.v3`，用于 `devices.yaml` 解析。
- **跨平台 release 流水线**：`.goreleaser.yaml` + `.github/workflows/release.yml`，tag push (`v*.*.*`) 触发，自动构建 `linux/amd64,arm64` + `darwin/amd64,arm64` + `windows/amd64` 二进制并发草稿 release，附 `checksums.txt`。
- **CI workflow**（`.github/workflows/ci.yml`）：PR 与 push 触发，三平台跑 `go vet/build/test` + `goreleaser check` + snapshot single-target build。
- 构建注入 version：`-X main.version={{.Version}}` ldflags，正式版 `ingest version` 显示 tag 名。

## [0.0.1] - 2026-05-06

首个测试版本（Phase 1 MVP）。引擎已通过 CLI flag 端到端跑通，可用于人工触发的真实拷贝。TUI、EXIF、自动挂载检测、YAML 配置等功能见路线图，后续版本补齐。

### Added

- CLI 入口 `ingest`，子命令 `version`、`devices list`
- 源卷扫描器 (`internal/scanner`)：识别媒体文件与 sidecar
- 内置设备识别规则 (`internal/device`)：Sony ZV-E10M2、DJI Pocket3，按卷标 → 目录 → 文件名置信度排序
- 时间段推断 (`internal/period`)：基于文件 mtime 的最小/最大日期
- 路径模板渲染 (`internal/template`)：`{var}` 变量替换 + `[seg]` 可选段省略 + 文件系统非法字符净化
- 安全拷贝协议 (`internal/copier.SafeCopy`)：流式 xxHash64 + 临时文件 + 磁盘回读再校验 + 原子 `os.Rename` + mtime/权限恢复
- SQLite 历史库 (`internal/db`)：`ingest_history` 表，UNIQUE(target_path)，纯 Go 驱动 `modernc.org/sqlite`
- 增量跳过：基于尺寸 + 历史 hash 命中跳过；不命中时双端再哈希决定
- CLI 参数：`--source/-s`、`--target/-t`、`--name/-n`、`--device`、`--from/--to`、`--template`、`--db`、`--dry-run`、`--verbose/-v`
- 文档：中文 README、CONTRIBUTING（gitflow + Conventional Commits + 代码规范 + SafeCopy 不变量）

### Known limitations

- 仅支持显式 `--source` 路径，挂载点自动检测未实现
- 时间段仅基于 mtime，未读取 EXIF / QuickTime atom
- 设备规则只能在源码 `BuiltinRules` 中扩展，YAML 外置规则待实现
- 没有 TUI；事件名 `--name` 必填，无交互式提示
- 测试套件计划在 Phase 4 补齐，当前仓库无单元测试

[Unreleased]: https://github.com/hktkzyx/ingest/compare/v0.0.1...HEAD
[0.0.1]: https://github.com/hktkzyx/ingest/releases/tag/v0.0.1
