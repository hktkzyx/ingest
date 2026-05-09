# TODO

按优先级倒序，靠上越紧急。

## v0.2.0 候选

### 1. 交互模式的输入容错（高优）

当前任意一步输错（最常见：在「日期范围」处输了事件名，或在「事件名称」处回车留空）→ `prompt` 函数返回 error → `runIngest` 整个退出，前面所有进度（设备确认、段切分、目标选择）作废。

期望：解析失败时打印中文错误提示并**重新询问当前问题**，连续 N 次（建议 3 次）失败再终止。`Ctrl+C` 仍立即退出。

涉及：`internal/prompt/prompt.go` 各 prompt 函数引入 `retryUntil` 包装；`cmd/ingest/main.go` 在调用点决定哪些是可重试 / 哪些必须立即终止（如 `os.Stat` 失败的源路径）。

### 2. 0 文件不报错（中优）

`scanner.Scan` 返回空切片时（卡是空的、或全被孤立 sidecar 过滤掉了）现在会落到下游 `period.Segments` 报错退出。

期望：阶段 `[3/4]` 检测到 0 文件直接打印「未发现可拷贝的文件，源: <path>」并 `return nil`（exit 0），整个交互流程不进入段编辑。

### 3. 运行日志（中优）

当前错误只在 stdout/stderr 一闪而过；用户事后排查（"上次为什么少拷了一个 .xml？"）没有可查的记录。

期望：每次运行附带一条日志到 `~/.local/share/ingest/logs/run-<timestamp>.log`（Windows: `%LOCALAPPDATA%\ingest\logs\`），含：
- 设备识别结果与 reason / confidence
- 扫描总数、过滤掉的孤立 sidecar 列表
- 每个文件的 outcome（copied / skipped / conflict-{overwritten,kept} / failed）
- 总用时与吞吐

实现可考虑 `slog` + `os.OpenFile` 双写：终端打人类可读简化版，日志文件打结构化。`--log-file <path>` flag 覆盖默认位置；`--no-log` 禁用。
