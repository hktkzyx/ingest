# ingest

> 智能素材导入工具 — 插卡 → 自动识别设备 → 推断时间段 → 模板归档 → 校验通过 → 安全完成。

`ingest` 是一个跨平台的命令行工具，用于把相机 SD 卡里的素材按结构归档到本地，并保证字节级校验、可重复执行。它面向多设备影像创作者（微单 + 无人机 + 运动相机），目标是把 `rsync` 的可靠性和 Kocard / Hedge 这类商业工具的易用性结合起来——但完全开源、可定制。

**当前状态**：已发布预编译二进制（Linux / macOS / Windows 共 5 平台）。功能对齐 PRD Phase 1：YAML 设备配置、EXIF/QuickTime 时间提取、自动挂载检测、多事件分段、交互式设备/事件确认、覆盖保护。TUI 推迟到 Phase 2。完整规格见 [PRD.md](./PRD.md)；版本与变更见 [Releases](https://github.com/hktkzyx/ingest/releases) 与 [CHANGELOG](./CHANGELOG.md)。

---

## 它做什么

- **自动识别设备**（出厂内置 Sony / DJI Pocket3，规则放在 `~/.config/ingest/devices.yaml`，可任意编辑增删；识别错了交互式改写）
- **自动挂载检测**：插卡后不必传 `--source`，跨平台枚举可移动卷自动选
- **推断拍摄时间段**（图片读 EXIF `DateTimeOriginal`，视频读 QuickTime `mvhd` creation_time，提取失败回退到文件 mtime）
- **多事件分段**：按"连续 N 天 = 一个事件"自动切分，逐段交互式确认日期范围 + 事件名（`gap_days` 在 `config.yaml` 里改）
- **按模板生成目标路径**
- **安全拷贝**：流式 xxHash64 + 临时文件 + 原子 rename + mtime/权限保留
- **幂等**：同一张卡重复插入只跳过已校验通过的文件（基于 SQLite 历史库）

---

## 安装

### 推荐：下载预编译二进制

到 [Releases](https://github.com/hktkzyx/ingest/releases) 选最新版本，下载对应平台的归档：

| 平台 | 文件名 |
|---|---|
| Linux x86_64 | `ingest_<版本>_linux_x86_64.tar.gz` |
| Linux ARM64 (Raspberry Pi 4/5、ARM 服务器) | `ingest_<版本>_linux_arm64.tar.gz` |
| macOS Intel | `ingest_<版本>_macos_x86_64.tar.gz` |
| macOS Apple Silicon (M 系列) | `ingest_<版本>_macos_arm64.tar.gz` |
| Windows x86_64 | `ingest_<版本>_windows_x86_64.zip` |

每个 release 都附 `checksums.txt`，建议下完先校验再解包。

**Linux / macOS**

```bash
# 自动取最新正式版（不含 pre-release），按平台改架构后缀（linux_arm64 / macos_x86_64 / macos_arm64）
TAG=$(curl -fsSL https://api.github.com/repos/hktkzyx/ingest/releases/latest | grep '"tag_name"' | head -1 | cut -d'"' -f4)
VER="${TAG#v}"
curl -fsSL "https://github.com/hktkzyx/ingest/releases/download/${TAG}/ingest_${VER}_linux_x86_64.tar.gz" | tar xz
sudo install -m 0755 ingest /usr/local/bin/
ingest version
```

**Windows**

下载 `.zip` 解压，把 `ingest.exe` 拖到一个在 `PATH` 里的目录（例如 `C:\Users\<你>\bin\` 并把它加到系统环境变量），或者直接 `cd` 到解压目录运行。

二进制是 self-contained 的：`devices.yaml` 和 `config.yaml` 都用 `go:embed` 打包在 binary 里，**首次运行会自动写到** `~/.config/ingest/` 下（Windows 是 `%USERPROFILE%\.config\ingest\`），无需手动放配置文件。

### 备选：从源码安装

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
ingest version          # → 当前版本号
ingest devices list     # → 当前配置的设备规则（首次运行自动生成 ~/.config/ingest/devices.yaml）
```

---

## 快速开始

### 推荐：零参数交互式向导

插好卡，直接：

```bash
ingest
```

工具会按 `[1/4] 选择源 → [2/4] 确认设备 → [3/4] 扫描素材并按事件分段 → [4/4] 确认目标并开始拷贝` 四步走，途中提示如下：

- 自动找到能匹配的可移动卷（多个时让你选）
- 自动识别设备并问 `接受? [Y=接受 / n=拒绝 / list=查看列表]:`
- 按 `gap_days`（默认 1）把文件分成若干个事件段，逐段询问日期范围 + `事件名称`
- 最后 `保存到哪里? [默认: ~/Backups]:` + 总览 `继续? [Y/n]:`

### 脚本化用法（带 flag）

```bash
# 单段 + 显式参数 + dry-run
ingest --source /Volumes/SONY_XYZ --target ~/Backups --name "周末骑行" --dry-run

# 自动检测 + 接受所有 prompt（非交互）
ingest --yes --target ~/Backups --name "测试"
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
| `-s`, `--source` | _（自动检测）_ | 源路径（挂载点 / 目录）。省略时枚举系统挂载点 + 设备规则匹配；1 个候选直采，多个候选交互式选 |
| `-t`, `--target` | `~/Backups` | 目标根目录 |
| `-n`, `--name` | _（每段交互式询问）_ | 事件名称。仅适用于自动检测出单段时；多段时需逐段交互输入 |
| `--device` | _（自动检测 + 询问）_ | 强制指定设备 ID（如 `zve10m2`），跳过检测 prompt |
| `--from` | _（自动）_ | 强制时间段起始 (`YYYY-MM-DD`)，会强制单段 |
| `--to` | _（自动）_ | 强制时间段结束 (`YYYY-MM-DD`)，会强制单段 |
| `--gap-days` | _（来自 config）_ | 自动分段时允许的日期间隔。覆盖 `config.yaml` 的 `gap_days` |
| `--template` | `{date_start}[_{date_end}]-{event_name}/origin-{device_name}` | 路径模板 |
| `--db` | `$XDG_DATA_HOME/ingest/ingest.db`，回退 `~/.local/share/ingest/ingest.db` | 历史数据库路径 |
| `--devices` | `$XDG_CONFIG_HOME/ingest/devices.yaml`，回退 `~/.config/ingest/devices.yaml` | 设备规则文件，缺失时自动写入出厂默认 |
| `--config` | `$XDG_CONFIG_HOME/ingest/config.yaml`，回退 `~/.config/ingest/config.yaml` | 全局设置文件，缺失时自动写入出厂默认 |
| `--dry-run` | `false` | 仅预览，不实际拷贝 |
| `-y`, `--yes` | `false` | 接受所有自动选择（多挂载选最高分、设备直接接受）；与多段不兼容 |
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
      directories:   ["PRIVATE/M4ROOT", "DCIM/100MSDCF"]    # 全部命中 → 0.80
      file_patterns: ["C*.MP4", "DSC*.ARW", "DSC*.JPG"]     # 根目录或下两层 glob → 0.70
```

> 提示：`directories` 应该列**这台设备特有的**目录组合（避免 `DCIM` 这种通用名）。partial directory 档要求至少 2 个目录命中，单个偶然命中不会触发误判。

多设备同时匹配取置信度最高者。要换文件位置：`ingest --devices /path/to/your.yaml ...`。

---

## 全局设置 `config.yaml`

工具自身的行为参数（不是设备规则）放在另一个文件里，路径同样在 `~/.config/ingest/`，首次运行自动写出。

```yaml
version: "1"
gap_days: 1   # 自动分段时允许的日期间隔；相邻文件拍摄日期相差 ≤ 该值视为同段。
              # 默认 1：今天 + 明天合一段，中间空一天就分段。
```

CLI 上的 `--gap-days N` 会临时覆盖这里的设置。

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
│   ├── mount/              # 跨平台枚举可移动挂载卷（linux/darwin/windows 各一个 build tag 文件）
│   ├── timestamp/          # EXIF / QuickTime 拍摄时间提取
│   ├── period/             # 时间段推断 + 多事件分段（timestamp 优先，mtime 兜底）
│   ├── prompt/             # 交互式 stdin 提示（设备确认、段编辑、卷选择）
│   ├── config/             # config.yaml 加载（gap_days 等全局设置）
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
- **未经明确指示不要做的事**：TUI、网络 I/O——这些都在路线图上（PRD §11）但还没接入。

---

## 路线图

里程碑目标见 [PRD §11](./PRD.md#11-里程碑规划)，详细 task 清单（含已完成 / 待做）见 [TODO.md](./TODO.md)。要点：

| 版本 | 重点 |
|---|---|
| **v0.1.x** | YAML 设备配置、EXIF/QT 时间提取、跨平台自动挂载检测、多事件分段、交互式设备 / 事件确认、覆盖保护、跨平台 release 流水线 |
| **v0.2.0** | TUI 交互（bubbletea）、多目标备份、`verify` / `history` 子命令、`devices.yaml`/`config.yaml` schema 校验 |
| **v0.3.0** | 代理文件生成（FFmpeg）、多卡队列、剪辑软件 XML 导出、云端归档 |
| **v1.0.0** | 测试覆盖率 >80%、包管理分发（Homebrew/Scoop） |

---

## 许可证

[MIT](./LICENSE)。Copyright © 2026 hktkzyx。
