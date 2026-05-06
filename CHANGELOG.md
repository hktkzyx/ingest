# Changelog

本项目所有显著变更都会记录在本文件中。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

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
