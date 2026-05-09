# Changelog

本项目所有显著变更都会记录在本文件中。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Fixed

- **scanner 过滤孤立 sidecar**：原来 `.xml/.thm/.srt/.lrc` 只看扩展名收入，导致 SONY 卡的 `PRIVATE/M4ROOT/THMBNL/CTRL_INF.xml`、`PRIVATE/DATABASE/MEDIAPRO.xml` 这类相机内部状态文件也被当作 sidecar 拷到目标目录，污染备份。改为：sidecar 只在**同目录存在同 base name 的媒体文件**时才收入；孤立 sidecar 跳过。

### Added

- **零参数交互式向导**：直接运行 `ingest`（不带任何 flag）就能跟着提示完成备份。
  - 启动 banner: `ingest <版本> — 智能素材导入向导`
  - 四个段落：`[1/4] 选择源` → `[2/4] 确认设备` → `[3/4] 扫描素材并按事件分段` → `[4/4] 确认目标并开始拷贝`
  - 每段开始前打 banner，过程中状态用统一中文文案展示
- **目标目录交互询问**：`--target` 没显式给时，弹 `保存到哪里? [默认: ~/Backups]:`，回车接受默认或粘贴新路径
- **拷贝前总览确认**：列出全部段（日期 / 事件名 / 文件数 / 字节 / 目标路径）+ 总文件数 + 设备名，最后 `继续? [Y/n]: `；`--yes` 跳过总览
- 新 prompt API：`AskTarget`、`ConfirmProceed`、`HumanBytes`（导出）

### Changed

- **全部交互提示中文化**：设备确认 prompt（`接受? [Y=接受 / n=拒绝 / list=查看列表]`）、设备列表选择、段编辑（`事件 X/Y`、`日期范围`、`事件名称`）、自动检测源信息、错误消息、verbose 时间来源标签、最终 `完成: X 个已拷贝...` 总结、设备识别 reason 字段（`卷标 / 目录结构 / 文件名模式 / 部分目录`）
- scanner 测试覆盖孤立 sidecar 过滤、配对 sidecar 保留、跨目录不配对、大小写不敏感配对

## [0.1.0-alpha.2] - 2026-05-08

第二个 alpha 预发布。在 alpha.1 上补齐多事件分段、交互式段编辑、设备识别确认 prompt，并修正两个出厂识别规则与 partial directory 评分阈值。

### Fixed

- **SONY ZVE10M2 默认识别规则收紧**：原来 `directories: [PRIVATE/SONY, DCIM]` 太宽——杂牌 U 盘只要有 `DCIM` 就会被部分命中误判（如 v0.1.0-alpha.1 上 G:\ 被打成 confidence 0.60）。改为 `[PRIVATE/M4ROOT, DCIM/100MSDCF]`，对应 ZVE10M2 实际卡布局（视频在 `M4ROOT/CLIP`、图片在 `DCIM/100MSDCF`）。
- **DJI Pocket3 默认识别规则修正**：原来 `directories: [DCIM/100MEDIA]` 完全对不上——Pocket 3 实际是 `DCIM/DJI_001/`（序号会滚动）+ `MISC/IDX/` + `MISC/THM/`。改为 `[MISC/THM, MISC/IDX]`，避开会变的 `DJI_NNN` 序号；file_patterns 加上 `DJI_*.JPG` 和 `DJI_*.WAV` 覆盖照片和音频。
- **`device.score` 部分命中阈值提高**：原来 `dirHits > 0` 就给 0.5+ 评分；改为至少命中 2 个目录才回落 partial 档，避免单个偶然命中触发 false positive。

### Note for existing users

已运行过 v0.1.0-alpha.1 的用户，`~/.config/ingest/devices.yaml` 已经写出，更新出厂默认不会自动回灌。要拿到新规则有两种办法：

1. 直接编辑 `~/.config/ingest/devices.yaml`，把 ZVE10M2 的 `directories` 改成 `["PRIVATE/M4ROOT", "DCIM/100MSDCF"]`
2. 删除 `~/.config/ingest/devices.yaml`，下次运行时会重新写出新版默认

### Added

- **多事件分段**：`period.Segments(files, gapDays)` 把文件按拍摄时间相邻聚成段；相邻文件日期间隔 ≤ `gap_days` 视为同段。
- **`config.yaml` 全局设置**：`internal/config` 包，路径同 `~/.config/ingest/`，首次运行写入内嵌默认；目前只有 `gap_days`，后续可加更多工具行为参数。
- **交互式 prompt**：`internal/prompt` 包提供设备确认（Y/n/list）、段编辑（范围 + 事件名）、卷选择；统一 `IO{In, Out}` 便于 mock。
- **设备交互式覆盖**：自动检测后 prompt 用户确认，错了可输 `list` 改选；`--device` 显式指定时跳过；`--yes` 自动接受。
- **多段交互式确认**：自动分段后逐段 prompt 起止日期 + 事件名；`--name` 仅适用于单段（多段时报错）。
- `--gap-days N` flag，临时覆盖 config 中的设置。
- `--config <path>` flag，覆盖默认 `config.yaml` 路径。
- `LICENSE` 文件（MIT），README/CONTRIBUTING 许可证节同步。

### Changed

- `runIngest` 主流程重写：load settings → mount detection → device confirm → segment files → per-segment prompt → loop copy。
- 单段 `--name` 行为保留向后兼容；多段 + `--name` 报错（不知道把名字给哪段）；多段 + `--yes` 报错（不允许盲发）。
- `internal/period` 重写 `Infer` 为 `Segments` 的退化版（强制单段）。
- README 安装节重写，明确推荐预编译二进制下载，列出 5 平台文件名表，说明 `devices.yaml` / `config.yaml` 由 `go:embed` 内嵌、首次运行自动写出。

## [0.1.0-alpha.1] - 2026-05-08

第一个 alpha 预发布。MVP 之上完成 Phase 1 ergonomics 三件套（YAML 设备配置、EXIF/QT 时间提取、自动挂载检测）以及跨平台 release 流水线。TUI 推迟到 v0.2.0。

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
- **EXIF / QuickTime 时间提取**：新增 `internal/timestamp` 包；图片（jpg/jpeg/arw/cr2/cr3/nef/dng/heic/tiff）走 EXIF `DateTimeOriginal`，视频（mp4/mov/m4v/mts/m2ts）解析 QuickTime `moov/mvhd` atom 拿 creation_time。`period.Infer` 优先用嵌入时间，提取失败才回退到 `fs.FileInfo.ModTime()`。verbose 模式打印 `(time source: exif=N quicktime=N mtime-fallback=N)`。
- `internal/timestamp` 与 `internal/period` 加最小单元测试覆盖 QT atom 解析、扩展名分发与 mtime 回退路径。
- 直接依赖 `github.com/dsoprea/go-exif/v3`，纯 Go 无 CGO，跨平台不影响 release 矩阵。
- **自动挂载检测**：新增 `internal/mount` 包，跨平台枚举可移动卷：
  - Linux 解析 `/proc/mounts`，过滤 `/media/<user>/`、`/run/media/<user>/`、`/mnt/` 前缀，丢弃内核虚拟 fstype；
  - macOS 读 `/Volumes/` 目录条目，排除系统盘（Macintosh HD / Macintosh HD - Data）；
  - Windows 通过 `golang.org/x/sys/windows` 调 `GetLogicalDrives` + `GetDriveType`，仅取 `DRIVE_REMOVABLE`，再用 `GetVolumeInformation` 拿卷标和 fstype。
- `--source` 缺省时自动调 `mount.List` + `device.Detect` 评分：0 候选报错、1 候选直采、N 候选交互式数字选择；`--yes` 时自动取置信度最高项。`--source` 显式指定时整段跳过。
- `internal/mount` 的 Linux 解析分离出 `parseProcMounts(io.Reader)` 便于测试；覆盖虚拟 fstype 过滤、`\040` 八进制反转义、重复挂载点去重。

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

[Unreleased]: https://github.com/hktkzyx/ingest/compare/v0.1.0-alpha.2...HEAD
[0.1.0-alpha.2]: https://github.com/hktkzyx/ingest/releases/tag/v0.1.0-alpha.2
[0.1.0-alpha.1]: https://github.com/hktkzyx/ingest/releases/tag/v0.1.0-alpha.1
[0.0.1]: https://github.com/hktkzyx/ingest/releases/tag/v0.0.1
