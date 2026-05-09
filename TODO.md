# TODO & Roadmap

短期施工清单 + 长期里程碑。每个 Phase 的目标见 [PRD §11](./PRD.md#11-里程碑规划)。本文件是**唯一**的具体待办 source of truth；PRD 不再列 task。

---

## Phase 1 — MVP (v0.1.x)

**目标**：解决核心痛点，可用且安全。

### 已完成

- [x] 项目骨架 + CLI 框架（cobra + gitflow + Conventional Commits）
- [x] 设备识别 + YAML 外置（首次运行写 `~/.config/ingest/devices.yaml`，可任意编辑）
- [x] EXIF / QuickTime 时间提取 + mtime 兜底
- [x] 模板引擎（`{date_start}` / `{event_name}` / `{device_name}` / `{device_id}` 等）
- [x] 安全拷贝协议（临时文件 + xxHash64 双向校验 + 原子 rename + db 快路径）
- [x] 增量识别（同 size + hash 跳过；SQLite history 库记录）
- [x] 跨平台编译（Linux x86_64/arm64、macOS x86_64/arm64、Windows x86_64）
- [x] 跨平台 release 流水线（goreleaser + GitHub Actions，tag push 自动构建 + Pre-release 标记）
- [x] 自动挂载检测（`mount.List` 跨平台枚举可移动卷）
- [x] 多事件分段（`period.Segments` 按连续 N 天聚合 + 交互式段编辑）
- [x] 零参数交互式向导（中文 `[1/4]…[4/4]` 流程：源 / 设备 / 扫描 / 目标）
- [x] 孤立 sidecar 过滤（`scanner` 两遍扫描，避免 SONY `THMBNL/CTRL_INF.xml` 等内部状态文件污染备份）
- [x] 覆盖保护 + `.ingest-trash` 备份（默认 prompt 确认；`--overwrite` 跳过 prompt；`--yes` 默认保守跳过冲突）

### 推迟

- [ ] 基础 TUI（Bubble Tea）— 推迟到 Phase 2，目前用 stdio prompt 替代，体验已可接受

---

## Phase 2 — 完善 (v0.2.x)

**目标**：从能用提升到顺手 — 可配置、可恢复、可审计。

### 高优

- [ ] 交互模式输入容错（详见下文）
- [ ] 扫描到 0 文件时友好退出（详见下文）
- [ ] 持久运行日志（详见下文）

### 中优

- [ ] 设备配置热加载（编辑 `devices.yaml` 后不重启 ingest 也生效）
- [ ] 多目标备份（`--targets path1,path2,...`，并行写入多份）
- [ ] 详细报告生成（每次运行末尾打印 / 写出 JSON 或文本报告）
- [ ] `verify` 子命令（对 history db 里全部 target_path 重算 hash，对比记录）
- [ ] `history` 子命令（按设备 / 时间 / 事件名查询过往拷贝记录）
- [ ] `devices.yaml` / `config.yaml` schema 校验（启动时报错点出错的字段而不是 silent ignore）
- [ ] `ingest devices add` 子命令（交互式新增设备规则，避免手编 YAML）

### 低优

- [ ] TUI 交互（bubbletea）— 替代 stdio prompt，提供进度条 / 多列布局

### 高优详细说明

#### 交互模式输入容错

当前任意一步输错（最常见：在「日期范围」处输了事件名，或在「事件名称」处回车留空）→ `prompt` 函数返回 error → `runIngest` 整个退出，前面所有进度（设备确认、段切分、目标选择）作废。

期望：解析失败时打印中文错误提示并**重新询问当前问题**，连续 N 次（建议 3 次）失败再终止。`Ctrl+C` 仍立即退出。

涉及：`internal/prompt/prompt.go` 各 prompt 函数引入 `retryUntil` 包装；`cmd/ingest/main.go` 在调用点决定哪些是可重试 / 哪些必须立即终止（如 `os.Stat` 失败的源路径）。

#### 扫描到 0 文件时友好退出

`scanner.Scan` 返回空切片时（卡是空的、或全被孤立 sidecar 过滤掉了）现在会落到下游 `period.Segments` 报错退出。

期望：阶段 `[3/4]` 检测到 0 文件直接打印「未发现可拷贝的文件，源: <path>」并 `return nil`（exit 0），整个交互流程不进入段编辑。

#### 持久运行日志

当前错误只在 stdout/stderr 一闪而过；用户事后排查（"上次为什么少拷了一个 .xml？"）没有可查的记录。

期望：每次运行附带一条日志到 `~/.local/share/ingest/logs/run-<timestamp>.log`（Windows: `%LOCALAPPDATA%\ingest\logs\`），含：
- 设备识别结果与 reason / confidence
- 扫描总数、过滤掉的孤立 sidecar 列表
- 每个文件的 outcome（copied / skipped / conflict-{overwritten,kept} / failed）
- 总用时与吞吐

实现可考虑 `slog` + `os.OpenFile` 双写：终端打人类可读简化版，日志文件打结构化。`--log-file <path>` flag 覆盖默认位置；`--no-log` 禁用。

---

## Phase 3 — 进阶 (v0.3.x)

**目标**：面向团队 / 专业工作流。

- [ ] 代理文件自动生成（FFmpeg shell out，按目标分辨率 / 码率生成 proxy）
- [ ] 缩略图生成
- [ ] 多卡队列（队列里多张卡按顺序导，不必等一张拷完再插下一张）
- [ ] 剪辑软件 XML 导出（FCPXML / Premiere XML）
- [ ] WebDAV / NAS 目标支持
- [ ] 云端归档插件（S3 / OSS / 七牛）
- [ ] 性能优化（并发拷贝、内存池、零拷贝 sendfile）

---

## Phase 4 — 稳定 (v1.0.0+)

- [ ] 单元测试覆盖率 >80%（当前覆盖：scanner / copier / prompt / device / period / mount / timestamp 共 7 包；待补：db / template / config / cmd 层）
- [ ] 集成测试（e2e 跑完整向导流程）
- [ ] 文档网站（VitePress 或 mdBook）
- [ ] 包管理分发（Homebrew tap / Scoop bucket / Chocolatey）
- [ ] 社区设备规则贡献流程（PR 模板 + 自动校验 + 入选合到 builtin）

---

具体实现讨论 / 设计权衡请放在对应 PR 或 GitHub Issue 里，不要堆在这里。
