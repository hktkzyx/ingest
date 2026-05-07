# ingest

> 智能素材导入工具 — 插卡 → 自动识别设备 → 推断时间段 → 模板归档 → 校验通过 → 安全完成。

`ingest` 是一个跨平台的命令行工具，用于把相机 SD 卡里的素材按结构归档到本地，并保证字节级校验、可重复执行。它面向多设备影像创作者（微单 + 无人机 + 运动相机），目标是把 `rsync` 的可靠性和 Kocard / Hedge 这类商业工具的易用性结合起来——但完全开源、可定制。

**当前状态**：`v0.0.1`（测试版本，Phase 1 MVP）已发布；`develop` 分支已陆续合入 YAML 设备配置、EXIF/QuickTime 时间提取、跨平台 release 流水线，将在 v0.1.0 一并发布。TUI、自动挂载检测仍在路线图上。完整规格见 [PRD.md](./PRD.md)。

---

## 它做什么

- **自动识别设备**（出厂内置 Sony / DJI Pocket3，规则放在 `~/.config/ingest/devices.yaml`，可任意编辑增删）
- **推断拍摄时间段**（图片读 EXIF `DateTimeOriginal`，视频读 QuickTime `mvhd` creation_time，提取失败回退到文件 mtime）
- **按模板生成目标路径**
- **安全拷贝**：流式 xxHash64 + 临时文件 + 原子 rename + mtime/权限保留
- **幂等**：同一张卡重复插入只跳过已校验通过的文件（基于 SQLite 历史库）

---

## 安装

### 预编译二进制（推荐）

到 [Releases](https://github.com/hktkzyx/ingest/releases) 下载对应平台的归档（Linux / macOS / Windows，amd64 + arm64），解包后把 `ingest` 放到 `PATH` 即可。每个 release 附 `checksums.txt` 用于校验。

```bash
# 示例：Linux x86_64
curl -L https://github.com/hktkzyx/ingest/releases/latest/download/ingest_<版本>_linux_x86_64.tar.gz | tar xz
sudo mv ingest /usr/local/bin/
```

> v0.0.1 是手工发布、暂未提供二进制；自 v0.1.0 起由 goreleaser 自动构建。

### 从源码安装

需要 Go **1.24+**（构建会自动升到 `go.mod` 中固定的 toolchain 版本）。

```bash
git clone https://github.com/hktkzyx/ingest.git
cd ingest
go install ./cmd/ingest
```

二进制会装到 `$(go env GOBIN)`（默认 `~/go/bin`）。把它加到 `PATH`：

```bash
export PATH="$HOME/go/bin:$PATH"   # 写到 ~/.zshrc 或 ~/.bashrc 持久化
```

### 验证

```bash
ingest version          # → ingest 0.0.1-dev
ingest devices list     # → 当前配置的设备规则（首次运行自动生成 ~/.config/ingest/devices.yaml）
```

---

## 快速开始

```bash
# 仅预览，不实际拷贝
ingest --source /Volumes/SONY_XYZ --target ~/Backups --name "周末骑行" --dry-run

# 真实运行 + 详细日志
ingest --source /Volumes/SONY_XYZ --target ~/Backups --name "周末骑行" -v
```

输出目录结构：

```
~/Backups/
└── 20260427-周末骑行/
    └── origin-SONY_ZVE10M2/
        ├── C0001.MP4
        ├── C0002.MP4
        └── DSC00001.ARW
```

再次运行同一条命令，每个文件都会在毫秒级内被跳过——`~/.local/share/ingest/ingest.db` 这个 SQLite 历史库记得每次拷贝过的内容。

> 提示：`--source`、`--target`、`--db`、`--devices` 都支持 `~`、`$VAR`、相对路径、绝对路径，Windows 上原生处理 `\` 与盘符。

---

## 命令

| 命令 | 说明 |
|---|---|
| `ingest [flags]` | 执行一次摄取（默认命令） |
| `ingest version` | 显示版本 |
| `ingest devices list` | 列出当前配置的设备识别规则（同时显示规则文件路径） |

### 参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-s`, `--source` | _（必填）_ | 源路径（挂载点 / 目录） |
| `-t`, `--target` | `~/Backups` | 目标根目录 |
| `-n`, `--name` | _（必填）_ | 事件名称，对应 `{event_name}` |
| `--device` | _（自动）_ | 强制指定设备 ID，如 `zve10m2`、`pocket3` |
| `--from` | _（按 mtime）_ | 时间段起始 (`YYYY-MM-DD`) |
| `--to` | _（按 mtime）_ | 时间段结束 (`YYYY-MM-DD`) |
| `--template` | `{date_start}[_{date_end}]-{event_name}/origin-{device_name}` | 路径模板 |
| `--db` | `$XDG_DATA_HOME/ingest/ingest.db`，回退 `~/.local/share/ingest/ingest.db` | 历史数据库路径 |
| `--devices` | `$XDG_CONFIG_HOME/ingest/devices.yaml`，回退 `~/.config/ingest/devices.yaml` | 设备规则文件，缺失时自动写入出厂默认 |
| `--dry-run` | `false` | 仅预览，不实际拷贝 |
| `-v`, `--verbose` | `false` | 详细输出 |

### 模板语法

- `{var}` — 变量替换（变量未定义会报错）
- `[ ... ]` — 可选段，段内任一变量为空则整段省略

| 变量 | 示例 |
|---|---|
| `date_start` | `20260427` |
| `date_end` | `20260503`（单天为空） |
| `event_name` | `周末骑行` |
| `device_id` | `zve10m2` |
| `device_name` | `SONY_ZVE10M2`（来自 `devices.yaml` 中的 `name` 字段，空格自动替换为 `_`） |

单天：`{date_start}[_{date_end}]-{event_name}/origin-{device_name}` → `20260427-周末骑行/origin-SONY_ZVE10M2`
跨天：同模板 → `20260427_20260503-五一假期/origin-DJI_Pocket3`

---

## 设备规则 `devices.yaml`

首次运行时会把内嵌的出厂默认写到 `~/.config/ingest/devices.yaml`（若设了 `XDG_CONFIG_HOME` 则在那下面），之后每次运行都重新读取该文件——增删条目不需要重编。

```yaml
version: "1"
devices:
  - id: zve10m2          # 必填，CLI 用 --device <id> 强指定
    name: "SONY ZVE10M2" # {device_name} 即此值，空格自动替换为 _
    manufacturer: "SONY" # 仅展示用
    detect:
      volume_labels: ["SONY"]                               # 卷标包含任一 → 0.90
      directories:   ["PRIVATE/SONY", "DCIM"]               # 全部命中 → 0.80
      file_patterns: ["C*.MP4", "C*.MTS", "DSC*.ARW"]       # 根目录或下两层 glob → 0.70
```

多设备同时匹配取置信度最高者。要换文件位置：`ingest --devices /path/to/your.yaml ...`。

---

## 安全拷贝是怎么做的

每个文件都按 [§FR-005 协议](./PRD.md#fr-005-安全拷贝协议) 处理：

1. **增量检查** — 若目标文件已存在且大小相同，且历史库里有匹配记录，仅重算源文件 hash；命中即跳过。
2. **流式拷贝** — 源文件只读一次；`io.MultiWriter` 同时把字节写到临时文件 (`*.ingest.tmp.<rand>`) 并喂给一个源端 xxHash64 实例。
3. **回读校验** — 从磁盘上重新读取临时文件、再算一遍 xxHash64。不匹配就删临时文件、记失败。
4. **原子 rename** — `os.Rename(tmp, dst)`；校验通过后文件才会以最终路径出现。
5. **元数据恢复** — 把源文件的 mtime 和权限位应用到新文件。
6. **写入历史** — `{源, 目标, 设备, 大小, hash}` 落库到 SQLite。

任何中断（断电、拔卡、磁盘满）之后，要么留下一个会被下次自动重试的临时文件，要么留下一个已经验证通过的最终文件——不会有"半完成"状态。

---

## 项目结构

```
.
├── PRD.md                  # 权威产品规格说明，设计意图都在这里
├── README.md               # 本文件
├── CONTRIBUTING.md         # 开发环境、分支策略、代码规范
├── cmd/ingest/main.go      # CLI 入口：仅做参数解析与流程串联
├── internal/
│   ├── scanner/            # 扫描源卷，区分媒体文件与 sidecar
│   ├── device/             # 设备识别规则与匹配器
│   │   ├── config.go       # devices.yaml 加载 + 首次运行写出默认
│   │   └── default.yaml    # 内嵌的出厂默认（go:embed）
│   ├── timestamp/          # EXIF / QuickTime 拍摄时间提取
│   ├── period/             # 时间段推断（timestamp 优先，mtime 兜底）
│   ├── template/           # 路径模板解析与渲染
│   ├── copier/             # 安全拷贝协议——核心逻辑
│   └── db/                 # SQLite 历史库（modernc.org/sqlite，纯 Go）
├── go.mod / go.sum
```

---

## 给 AI agent

如果你是在这个仓库里做修改的 AI 编码助手，先看这一节：

- **权威规格**：[`PRD.md`](./PRD.md)。本 README 是摘要，冲突时以 PRD 为准。
- **暂无对外 Go API**：`internal/` 故意不可被外部包导入。当前唯一稳定接口是 CLI。
- **分支策略**：本仓库使用 **gitflow**。绝不直接提交到 `main` 或 `develop`；新功能从 `develop` 切 `feature/<名称>`。完整规则见 [`CONTRIBUTING.md`](./CONTRIBUTING.md)。
- **完工前必跑**：
  ```bash
  go vet ./...
  go build ./...
  go test ./...        # 测试套件计划在 Phase 4 补齐，目前为空
  ```
- **黄金路径冒烟测试**（不需要真 SD 卡）：
  ```bash
  mkdir -p /tmp/fake-sony/PRIVATE/SONY /tmp/fake-sony/DCIM/100MSDCF
  echo hello > /tmp/fake-sony/DCIM/100MSDCF/C0001.MP4
  go run ./cmd/ingest --source /tmp/fake-sony --target /tmp/out --name test -v
  ```
  期望：识别为 `zve10m2`、文件被拷、hash 入库、退出码 0。
- **改动前要先读的文件**：
  - `internal/copier/copier.go` — 安全拷贝不变量；不要放松校验步骤
  - `internal/db/db.go` — Schema 是 `UNIQUE(target_path)`；尚无迁移机制
  - `cmd/ingest/main.go` — 全部 CLI 表面
- **未经明确指示不要做的事**：TUI、自动挂载检测、网络 I/O——这些都在路线图上（PRD §11）但还没接入。

---

## 路线图

完整里程碑见 [PRD.md §11](./PRD.md#11-里程碑规划)，要点：

| 版本 | 重点 |
|---|---|
| **v0.1.0** | TUI 交互、自动挂载检测（EXIF/QT 提取、跨平台发布、YAML 设备配置已在 develop 完成） |
| **v0.2.0** | 多目标备份、`verify` / `history` 子命令、`devices.yaml` schema 校验 |
| **v0.3.0** | 代理文件生成（FFmpeg）、多卡队列、剪辑软件 XML 导出、云端归档 |
| **v1.0.0** | 测试覆盖率 >80%、包管理分发（Homebrew/Scoop） |

---

## 许可证

待定，倾向 MIT 或 Apache-2.0。
