# Ingest — 智能素材导入工具

## 需求规格书 (PRD)

**版本**: v1.0.0-draft  
**日期**: 2026-04-27  
**作者**: brooksyuan  
**状态**: 草案

---

## 1. 项目概述

### 1.1 背景

作为多设备影像创作者，日常拷卡流程极其繁琐：

- **设备多**: Sony ZVE10M2（微单）+ DJI Pocket3（口袋云台），文件命名规则不同，存储结构不同
- **命名乱**: 索尼是 `C0001.MP4`，Pocket3 是 `DJI_20250427120001_0001_D.MP4`，手动整理极易出错
- **整理累**: 需要按拍摄日期/事件分文件夹，还要区分原始素材和设备来源
- **不放心**: 拷卡过程中断、遗漏、损坏无法感知，没有校验机制
- **重复拷**: 同一张卡二次插入时无法识别哪些已拷过

现有商业方案（如 Kocard）收费且封闭，无法满足自定义需求。

### 1.2 目标

开发一个**开源、跨平台、可配置**的智能素材导入工具，让拷卡从"手动苦力活"变成"一键自动化"。

### 1.3 核心价值主张

> 插卡 → 自动识别设备 → 推断时间段 → 按模板归档 → 校验通过 → 安全弹出

---

## 2. 用户画像

### 2.1 主要用户: 独立影像创作者

- 拥有 2-3 台拍摄设备（相机、无人机、运动相机等）
- 每周产生 50-200GB 素材
- 习惯按项目/事件/日期管理素材
- 对数据安全有极高要求（不可丢失原始素材）
- 偏好命令行工具，但也需要友好的交互引导

### 2.2 使用场景

| 场景 | 频率 | 描述 |
|------|------|------|
| 日常拷卡 | 每天 | 当天拍摄完，回家插卡一键导入 |
| 项目归档 | 每周 | 周末集中整理一周的素材到一个项目文件夹 |
| 旅行/假期 | 不定期 | 多天素材合并到一个事件文件夹 |
| 双备份 | 每次 | 同时拷贝到本地 SSD + 外置硬盘 |

---

## 3. 术语表

| 术语 | 定义 |
|------|------|
| **源设备** (Source Device) | 产生素材的物理设备，如 ZVE10M2、Pocket3 |
| **存储卷** (Volume) | 挂载到系统的存储介质，如 SD 卡、TF 卡、机身存储 |
| **摄取** (Ingest) | 将素材从存储卷安全复制到目标目录的完整流程 |
| **模板路径** (Template Path) | 用户定义的目标目录命名规则，如 `YYYYMMDD-名称/origin-设备名` |
| **时间段** (Period) | 一组素材的日期范围，用于文件夹命名 |
| **校验和** (Checksum) | 用于验证文件完整性的哈希值（xxHash64） |
| **增量** (Incremental) | 已存在于目标目录且校验通过的文件，跳过不重复拷贝 |

---

## 4. 功能需求

### 4.1 需求分级

- **P0** (Must Have): MVP 必须实现，缺失则产品不可用
- **P1** (Should Have): 重要功能，显著提升体验
- **P2** (Nice to Have): 锦上添花，可后续迭代

### 4.2 P0: 核心功能

#### FR-001: 自动设备识别

**描述**: 插入存储卡后，工具应自动识别其来源设备，无需用户手动指定。

**识别策略（优先级排序）**:

1. **卷标匹配**: 检查挂载点卷标（如 `SONY_XYZ`, `DJI_POCKET3`）
2. **目录结构指纹**: 检查预定义的目录模式
   - Sony: `PRIVATE/SONY/` + `DCIM/`
   - Pocket3: `DCIM/100MEDIA/DJI_*.MP4`
3. **文件命名模式**: 检查根目录/DCIM 下的文件名前缀
   - Sony: `C*.MP4`, `DSC*.ARW`
   - Pocket3: `DJI_*.MP4`
4. **用户确认**: 若以上均不匹配，TUI 弹窗询问，并将用户选择持久化到配置

**输出**: 设备标识符（如 `zve10m2`, `pocket3`），用于后续模板渲染。

#### FR-002: 元数据完整保留

**描述**: 拷贝过程必须 100% 保留原始文件的元数据，不得有任何修改或丢失。

**保留项**:
- 文件修改时间 (mtime)
- 文件创建时间 (birthtime，若文件系统支持)
- 访问权限 (Unix mode bits)
- 扩展属性 (xattr，如 macOS 的 `_kMDItemContentCreationDate`)
- 内部 EXIF/XMP/QuickTime metadata（通过逐字节复制保证）

**禁止行为**:
- 不得通过图像/视频处理库"重写"文件（会丢失或改变元数据）
- 不得修改文件任何字节

#### FR-003: 模板化路径生成

**描述**: 用户通过模板定义目标目录结构，工具根据模板动态生成最终路径。

**默认模板**:
```
{date_start}[_{date_end}]-{event_name}/origin-{device_id}
```

**模板变量**:

| 变量 | 示例 | 说明 |
|------|------|------|
| `{date_start}` | `20250427` | 素材最早日期，格式 YYYYMMDD |
| `{date_end}` | `20250503` | 素材最晚日期，格式 YYYYMMDD |
| `{event_name}` | `五一假期` | 用户给定的事件名称 |
| `{device_id}` | `zve10m2` | 设备标识符（小写，无空格） |
| `{device_name}` | `ZVE10M2` | 设备友好名称 |

**日期范围简化规则**:
- 单天素材: `{date_start}` 即可，省略 `_{date_end}`
- 多天素材: `{date_start}_{date_end}`

**示例输出**:
```
20250427-周末骑行/origin-zve10m2/
20250427_20250503-五一假期/origin-pocket3/
```

#### FR-004: 时间段自动推断

**描述**: 扫描存储卷中所有媒体文件的 EXIF/QuickTime creation date，自动推断拍摄时间段。

**算法**:
1. 遍历存储卷所有媒体文件（`.MP4`, `.MOV`, `.JPG`, `.ARW`, `.RAW` 等）
2. 提取每个文件的创建时间（优先 EXIF `DateTimeOriginal`，次选文件系统 mtime）
3. 计算 `min(date)` → `date_start`, `max(date)` → `date_end`
4. 向用户展示推断结果，允许修改

**交互**: TUI 弹窗
```
检测到素材时间范围: 2025-04-27 至 2025-04-27 (共 1 天)
请输入事件名称: [周末骑行]
确认时间段? [Enter 确认 / e 编辑 / a 追加到现有文件夹]
```

#### FR-005: 安全拷贝协议

**描述**: 所有文件必须通过安全的拷贝流程，确保数据完整性。

**协议步骤**:

1. **路径计算**: 根据模板确定目标路径
2. **增量检查**: 若目标文件已存在且大小相同，进入校验流程；若校验通过，跳过
3. **临时写入**: 拷贝到目标路径的同级临时文件（`.ingest.tmp.{random}`）
4. **流式校验**: 拷贝同时计算源文件和目标临时文件的 xxHash64
5. **校验比对**: 源 hash == 目标 hash？
   - ✅ 通过: `rename(临时文件 → 最终文件)`，原子操作
   - ❌ 失败: 删除临时文件，标记为失败，记录日志，可选重试（最多 3 次）
6. **元数据恢复**: 将原始文件的 mtime/权限应用到新文件
7. **完成记录**: 写入内部数据库（源路径 → 目标路径 → hash → 时间戳）

**失败处理**:
- 单文件失败不影响其他文件继续拷贝
- 最终输出失败清单，用户可单独重试

#### FR-006: 增量识别

**描述**: 同一张卡多次插入，或目标目录已存在部分素材时，只拷贝新增或变更的文件。

**判断逻辑**:

```
if 目标文件不存在:
    → 全新拷贝
elif 目标文件存在 AND 大小相同:
    if 目标文件在数据库中有记录 AND 记录 hash == 当前源文件 hash:
        → 跳过（快速路径，无需重算 hash）
    else:
        → 重算双方 xxHash64
        if hash 相同:
            → 跳过，更新数据库
        else:
            → 重新拷贝（覆盖）
elif 目标文件存在 AND 大小不同:
    → 重新拷贝（覆盖）
```

#### FR-007: 多设备文件夹隔离

**描述**: 即使同一事件/时间段涉及多台设备，素材也必须在不同的设备子目录中隔离存放。

**结构示例**:
```
~/Backups/
└── 20250427-周末骑行/
    ├── origin-zve10m2/
    │   ├── C0001.MP4
    │   ├── C0002.MP4
    │   └── DSC00001.ARW
    └── origin-pocket3/
        ├── DJI_20250427120001_0001_D.MP4
        └── DJI_20250427130001_0002_D.MP4
```

#### FR-008: 跨平台支持

**描述**: 工具必须在 macOS、Linux、Windows 上可编译运行。

**平台差异处理**:
- **macOS**: 使用 FSEvents 或轮询检测卷挂载；保留 HFS+/APFS 扩展属性
- **Linux**: 使用 udev/inotify 或轮询；保留 ext4/xattr
- **Windows**: 使用 WMI/RegisterDeviceNotification 或轮询；NTFS 替代数据流（ADS）尽力保留

**最低要求**: 即使高级功能（自动挂载检测）在某些平台受限，核心的 CLI 手动指定路径功能必须全平台可用。

### 4.3 P1: 重要功能

#### FR-009: 多卡队列

**描述**: 同时插入多张存储卡时，按插入顺序排队依次处理，或并发处理（若目标目录不同）。

#### FR-010: 双备份目标

**描述**: 支持一次拷贝到两个目标路径（如本地 SSD + 外置硬盘），各自独立执行安全拷贝协议。

```bash
ingest --targets ~/Backups,/Volumes/BackupDrive
```

#### FR-011: 进度与报告

**描述**: 拷贝过程展示实时进度（已拷/总数/速度/ETA），完成后生成结构化报告。

**报告内容**:
- 处理文件总数、成功数、跳过数、失败数
- 总数据量、耗时、平均速度
- 失败文件清单（路径 + 错误原因）
- 校验和日志（可选输出到文件）

#### FR-012: 设备配置扩展

**描述**: 设备识别规则不硬编码，而是通过配置文件（`devices.yaml`）定义，方便用户自行添加新设备。

```yaml
devices:
  - id: zve10m2
    name: "Sony ZV-E10M2"
    detect:
      volume_label_contains: ["SONY"]
      directories: ["PRIVATE/SONY", "DCIM"]
      file_patterns: ["C*.MP4", "DSC*.ARW"]

  - id: pocket3
    name: "DJI Pocket3"
    detect:
      volume_label_contains: ["DJI"]
      directories: ["DCIM/100MEDIA"]
      file_patterns: ["DJI_*.MP4"]
```

#### FR-013: 配置文件管理

**描述**: 支持全局配置（`~/.config/ingest/config.yaml`）和项目级配置（`./.ingestrc.yaml`）。

**配置项**:
- `default_target`: 默认目标根目录
- `default_template`: 默认路径模板
- `devices_config_path`: 设备配置文件路径
- `hash_algorithm`: 校验算法（默认 xxhash64）
- `preserve_metadata`: 是否保留元数据（默认 true）
- `auto_eject`: 拷贝完成后是否自动弹出（默认 false）

### 4.4 P2: 进阶功能

#### FR-014: 代理文件自动生成

**描述**: 拷贝完成后，后台自动转码低码率代理文件，方便剪辑软件流畅预览。

- 输入: 4K H.265 / 4K/120fps
- 输出: 1080p H.264，同名加 `.proxy.mp4` 后缀
- 存放: `proxy-{device_id}/` 子目录，或同级目录

#### FR-015: 缩略图生成

**描述**: 为视频生成封面缩略图（JPG），为照片生成低分辨率预览，便于快速浏览。

#### FR-016: 剪辑软件集成

**描述**: 生成剪辑软件的素材库索引文件：
- DaVinci Resolve: `.drp` 或文件夹结构
- Premiere Pro: `.xml` 项目文件
- Final Cut Pro: `.fcpxml`

#### FR-017: 云端归档

**描述**: 本地拷贝完成后，可选自动上传到云存储（S3、OSS、Backblaze B2）作为异地备份。

#### FR-018: WebDAV/NAS 支持

**描述**: 目标路径支持远程存储，通过 WebDAV/SMB/NFS 挂载点访问。

---

## 5. 非功能需求

### 5.1 性能

- **拷贝速度**: 不低于存储介质理论读取速度的 80%（即 SD 卡读 95MB/s 时，ingest 应达到 75MB/s+）
- **内存占用**: 峰值内存不超过 500MB（即使处理 10万+ 文件列表）
- **启动时间**: CLI 冷启动 < 200ms

### 5.2 可靠性

- **数据零丢失**: 在任何异常中断（断电、拔卡、磁盘满、进程被杀）情况下，已完成的文件必须是完整且校验通过的；未完成的临时文件必须可安全丢弃
- **原子性**: 单个文件的最终呈现必须是原子的（通过临时文件 + rename 实现）
- **幂等性**: 同一操作执行多次，结果与执行一次相同

### 5.3 可维护性

- **日志**: 结构化日志（JSON Lines），包含时间戳、级别、操作、文件路径、hash、错误信息
- **调试**: `--verbose` / `--debug` 模式输出详细诊断信息
- **数据库**: 使用 SQLite 记录拷贝历史（源设备、文件路径、hash、时间戳），便于增量判断和审计

### 5.4 用户体验

- **TUI**: 首次配置、设备首次识别、时间段确认等场景使用交互式 TUI（基于 Bubble Tea 或类似框架）
- **CLI**: 日常操作完全可通过命令行 flag 完成，适合脚本和 alias
- **错误信息**: 友好、可操作的错误提示（如"磁盘空间不足，需要 50GB，剩余 30GB"而非"write error"）

### 5.5 安全性

- **无网络依赖**: 核心功能完全离线运行，不依赖任何云服务
- **无权限滥用**: 仅请求必要的文件系统权限（读源卷、写目标目录）
- **开源可审计**: 全部源代码公开，校验算法可复现

---

## 6. 系统架构

### 6.1 技术栈

| 组件 | 选择 | 理由 |
|------|------|------|
| 语言 | Go 1.24+ | 跨平台单二进制、并发优秀、标准库强大 |
| CLI 框架 | Cobra | 命令结构清晰、自动生成 help/补全 |
| TUI 框架 | Bubble Tea + Lipgloss | 生态成熟、响应式、美观 |
| 配置 | Viper | 支持 YAML/JSON/环境变量 |
| 数据库 | SQLite (modernc.org/sqlite) | 零配置、内嵌、纯 Go |
| Hash | xxHash (cespare/xxhash) | 极快，适合大文件流式计算 |
| EXIF 读取 | dsoprea/go-exif 或 rs/tminfo | 提取拍摄时间 |
| 日志 | slog (标准库) | 结构化、高性能 |

### 6.2 模块架构

```
┌─────────────────────────────────────────────┐
│                 cmd/ingest                    │
│              (Cobra 命令入口)                  │
└─────────────────────────────────────────────┘
                      │
    ┌─────────────────┼─────────────────┐
    ▼                 ▼                 ▼
┌─────────┐    ┌─────────────┐    ┌──────────┐
│  config │    │    TUI      │    │  logger  │
│ (Viper) │    │ (BubbleTea) │    │  (slog)  │
└────┬────┘    └──────┬──────┘    └────┬─────┘
     │                │                │
     └────────────────┼────────────────┘
                      ▼
┌─────────────────────────────────────────────┐
│              internal/core                    │
│  ┌─────────┐  ┌─────────┐  ┌─────────────┐  │
│  │ scanner │  │ device  │  │   period    │  │
│  │ (扫描源) │  │(设备识别)│  │(时间段推断) │  │
│  └────┬────┘  └────┬────┘  └──────┬──────┘  │
│       │            │              │          │
│       └────────────┼──────────────┘          │
│                    ▼                         │
│  ┌───────────────────────────────────────┐   │
│  │            template engine             │   │
│  │      (解析模板 → 生成目标路径)          │   │
│  └──────────────────┬────────────────────┘   │
│                     │                        │
│  ┌──────────────────┼────────────────────┐   │
│  │                  ▼                    │   │
│  │  ┌─────────┐  ┌─────────┐  ┌──────┐  │   │
│  │  │ copier  │  │ verify  │  │ db   │  │   │
│  │  │(安全拷贝)│  │(校验和) │  │(SQLite)│  │   │
│  │  └─────────┘  └─────────┘  └──────┘  │   │
│  └────────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

---

## 7. 核心模块详细设计

### 7.1 设备识别模块 (`internal/device`)

**接口**:
```go
type Device struct {
    ID          string // "zve10m2"
    Name        string // "Sony ZV-E10M2"
    Type        string // "camera", "drone", "action_cam"
}

type Detector interface {
    Detect(volumePath string) (*Device, float64, error)
    // 返回: 设备信息, 置信度(0-1), 错误
}
```

**检测器链**:
1. `VolumeLabelDetector`: 卷标关键词匹配（置信度 0.9）
2. `DirectoryStructureDetector`: 目录指纹匹配（置信度 0.8）
3. `FilePatternDetector`: 文件名模式匹配（置信度 0.7）
4. `FallbackDetector`: 用户交互确认（置信度 1.0，需人工）

**规则按置信度排序，第一个 >= 0.8 的直接采用；否则展示候选列表让用户选。**

### 7.2 模板引擎模块 (`internal/template`)

**语法**:
- 变量用 `{}` 包裹: `{date_start}`, `{event_name}`
- 条件段用 `[]` 包裹，内部变量若为空则整段省略: `[_{date_end}]`

**解析流程**:
1. Tokenize 模板字符串
2. 从上下文（扫描结果 + 用户输入）提取变量值
3. 渲染最终路径
4. 清理非法字符（`\/:*?"<>|` 等）

**示例**:
```go
template := "{date_start}[_{date_end}]-{event_name}/origin-{device_id}"
context := TemplateContext{
    DateStart: "20250427",
    DateEnd:   "20250503",  // 若为空字符串，条件段省略
    EventName: "五一假期",
    DeviceID:  "pocket3",
}
// 结果: "20250427_20250503-五一假期/origin-pocket3"
```

### 7.3 安全拷贝模块 (`internal/copier`)

**核心算法**:
```go
func SafeCopy(src, dst string, db *Database) error {
    // 1. 增量检查
    if existing := db.Get(dst); existing != nil {
        if existing.Size == srcSize && existing.Hash == srcHash {
            return ErrSkipped // 快速跳过
        }
    }

    // 2. 临时文件路径
    tmp := dst + ".ingest.tmp." + randomSuffix()

    // 3. 流式拷贝 + 计算 hash
    hasher := xxhash.New()
    writer := io.MultiWriter(file, hasher)
    if _, err := io.Copy(writer, srcReader); err != nil {
        os.Remove(tmp)
        return fmt.Errorf("copy failed: %w", err)
    }

    // 4. 校验
    srcHash := hasher.Sum64()
    dstHash := hashFile(tmp)
    if srcHash != dstHash {
        os.Remove(tmp)
        return fmt.Errorf("checksum mismatch: src=%x dst=%x", srcHash, dstHash)
    }

    // 5. 原子重命名
    if err := os.Rename(tmp, dst); err != nil {
        os.Remove(tmp)
        return fmt.Errorf("rename failed: %w", err)
    }

    // 6. 恢复元数据
    preserveMetadata(src, dst)

    // 7. 记录数据库
    db.Put(dst, FileRecord{Hash: srcHash, Size: srcSize, Time: time.Now()})

    return nil
}
```

### 7.4 时间段推断模块 (`internal/period`)

**算法**:
```go
func InferPeriod(files []MediaFile) Period {
    var minDate, maxDate time.Time
    for _, f := range files {
        date := f.CreationDate() // 优先 EXIF, 次选 mtime
        if minDate.IsZero() || date.Before(minDate) {
            minDate = date
        }
        if date.After(maxDate) {
            maxDate = date
        }
    }
    return Period{Start: minDate, End: maxDate}
}
```

**交互决策树**:
```
if 数据库中存在完全相同的日期范围 + 事件名称:
    → 提示"追加到现有文件夹?"
    if 用户确认:
        → 使用现有 event_name，不创建新文件夹
    else:
        → 要求输入新的 event_name
else:
    → TUI 展示推断结果，让用户确认或编辑
```

---

## 8. 数据流 / 工作流程

### 8.1 标准摄取流程（TUI 模式）

```
1. [系统事件] 检测到新卷挂载
        │
        ▼
2. [scanner]  扫描卷内所有媒体文件（递归 DCIM/等目录）
        │
        ▼
3. [device]   识别设备（zve10m2 / pocket3 / unknown）
        │
        ▼
4. [period]   提取所有文件日期，推断时间范围
        │
        ▼
5. [TUI]      展示推断结果，请求用户输入 event_name
        │
        ▼
6. [template] 生成目标路径: 20250427-周末骑行/origin-zve10m2/
        │
        ▼
7. [copier]   对每个文件执行安全拷贝协议
   ├─ 增量检查 → 跳过/覆盖
   ├─ 临时文件写入
   ├─ 校验和比对
   ├─ 原子重命名
   └─ 元数据恢复
        │
        ▼
8. [report]   输出摘要报告
        │
        ▼
9. [optional] 自动弹出存储卷
```

### 8.2 无人值守流程（CLI 模式）

```bash
# 全部通过 flag 指定，无交互
ingest \
  --source /Volumes/SONY_XYZ \
  --target ~/Backups \
  --name "周末骑行" \
  --device zve10m2 \
  --from 2025-04-27 \
  --to 2025-04-27
```

### 8.3 数据库 Schema

```sql
CREATE TABLE ingest_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_path TEXT NOT NULL,       -- 源文件绝对路径
    target_path TEXT NOT NULL,       -- 目标文件绝对路径
    device_id TEXT NOT NULL,         -- 设备标识
    volume_id TEXT,                  -- 存储卷 UUID/序列号
    size_bytes INTEGER NOT NULL,
    xxhash64 TEXT NOT NULL,          -- 校验和（16进制字符串）
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(target_path)
);

CREATE INDEX idx_target ON ingest_history(target_path);
CREATE INDEX idx_hash ON ingest_history(xxhash64);
```

---

## 9. CLI 接口设计

### 9.1 命令结构

```
ingest [command] [flags]

Commands:
  ingest              执行摄取（默认命令）
  config              交互式配置向导
  verify              校验已拷贝的目录
  history             查看拷贝历史
  devices             管理设备识别规则
  version             显示版本

Flags:
  -s, --source        源路径（存储卷挂载点）
  -t, --target        目标根目录
  -n, --name          事件名称
      --from          时间段起始（YYYY-MM-DD）
      --to            时间段结束（YYYY-MM-DD）
      --device        强制指定设备 ID
      --template      自定义路径模板
      --dry-run       预览模式，不实际拷贝
      --targets       多目标备份（逗号分隔）
  -y, --yes           跳过所有确认
  -v, --verbose       详细输出
      --debug         调试模式
```

### 9.2 使用示例

```bash
# ===== 日常用法 =====

# 一键拷卡（TUI 交互确认事件名）
ingest

# 指定事件名，全自动无交互
ingest --name "五一假期"

# 预览模式（先看会生成什么路径）
ingest --dry-run --name "周末骑行"

# 手动指定源和目标（适合脚本）
ingest --source /Volumes/SONY_XYZ --target ~/Backups --name "测试"

# 双备份
ingest --targets ~/Backups,/Volumes/BackupDrive --name "重要项目"

# ===== 管理用法 =====

# 配置向导
ingest config

# 校验历史文件夹是否完好
ingest verify ~/Backups/20250427-周末骑行

# 查看最近 20 条拷贝记录
ingest history --limit 20

# 列出已知设备
ingest devices list

# 添加自定义设备
ingest devices add --id osmo-action4 --name "Osmo Action 4"
```

---

## 10. 配置系统

### 10.1 全局配置 (`~/.config/ingest/config.yaml`)

```yaml
version: "1"

# 默认设置
defaults:
  target_root: "~/Backups"
  template: "{date_start}[_{date_end}]-{event_name}/origin-{device_id}"
  hash_algorithm: "xxhash64"
  auto_eject: false
  preserve_metadata: true

# 多目标备份（可选）
backup_targets:
  - "~/Backups"
  - "/Volumes/BackupDrive"

# 设备配置路径
devices_config: "~/.config/ingest/devices.yaml"

# 数据库路径
database_path: "~/.local/share/ingest/ingest.db"

# 日志设置
logging:
  level: "info"  # debug, info, warn, error
  format: "json" # json, text
  file: "~/.local/share/ingest/ingest.log"

# TUI 设置
ui:
  theme: "auto"  # auto, dark, light
  language: "zh-CN"
```

### 10.2 设备配置 (`~/.config/ingest/devices.yaml`)

```yaml
version: "1"

devices:
  - id: zve10m2
    name: "Sony ZV-E10M2"
    manufacturer: "Sony"
    type: "mirrorless"
    detect:
      volume_labels:
        - contains: "SONY"
      directories:
        - "PRIVATE/SONY"
        - "DCIM"
      file_patterns:
        - "C*.MP4"
        - "C*.MTS"
        - "DSC*.ARW"
        - "DSC*.JPG"
    # 该设备的特殊处理
    options:
      subdir_name: "origin-zve10m2"

  - id: pocket3
    name: "DJI Pocket3"
    manufacturer: "DJI"
    type: "pocket_gimbal"
    detect:
      volume_labels:
        - contains: "DJI"
      directories:
        - "DCIM/100MEDIA"
      file_patterns:
        - "DJI_*.MP4"
    options:
      subdir_name: "origin-pocket3"
```

### 10.3 项目级配置 (`./.ingestrc.yaml`)

在项目根目录（如 `~/Backups`）可放置覆盖配置：

```yaml
# 本项目使用不同的模板
template: "{date_start}-{event_name}/raw/{device_id}"

# 强制校验模式
verify_after_copy: true
```

---

## 11. 里程碑规划

### Phase 1: MVP (v0.1.0) — 4 周

**目标**: 解决核心痛点，可用且安全

- [ ] 项目骨架 + CLI 框架搭建
- [ ] 设备识别（硬编码 Sony + Pocket3）
- [ ] EXIF 日期提取 + 时间段推断
- [ ] 模板引擎（基础变量）
- [ ] 安全拷贝协议（临时文件 + xxHash + 原子 rename）
- [ ] 增量识别（基于文件大小 + hash）
- [ ] SQLite 数据库记录
- [ ] 基础 TUI（Bubble Tea）：事件名输入、进度展示
- [ ] 跨平台编译验证（macOS/Linux）

### Phase 2: 完善 (v0.2.0) — 3 周

**目标**: 提升易用性和鲁棒性

- [ ] 配置文件系统（YAML 全局配置 + 设备配置）
- [ ] 设备配置热加载（不重启增删设备规则）
- [ ] 多目标备份（`--targets`）
- [ ] 自动挂载检测（macOS FSEvents + Linux udev）
- [ ] 详细报告生成（JSON/Text）
- [ ] `verify` 子命令
- [ ] `history` 子命令
- [ ] Windows 支持验证

### Phase 3: 进阶 (v0.3.0) — 4 周

**目标**: 面向团队/专业工作流

- [ ] 代理文件自动生成（FFmpeg 集成）
- [ ] 缩略图生成
- [ ] 多卡队列
- [ ] 剪辑软件 XML 导出
- [ ] WebDAV/NAS 目标支持
- [ ] 云端归档插件（S3/OSS）
- [ ] 性能优化（并发拷贝、内存池）

### Phase 4: 稳定 (v1.0.0) — 持续

- [ ] 完整测试覆盖（单元测试 >80%，集成测试）
- [ ] 文档网站
- [ ] Homebrew/Scoop/Chocolatey 分发
- [ ] 社区设备规则贡献流程

---

## 12. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 不同文件系统元数据支持差异大 | 高 | 逐字节拷贝保证内部数据；元数据尽力而为，失败仅警告不阻断 |
| 大文件（如 128GB 单文件）拷贝中断 | 高 | 临时文件机制保证原子性；支持断点续传（未来版本） |
| 用户误删数据库导致增量失效 | 中 | 提供 `verify` 命令重建数据库；定期自动备份 db |
| 存储卷在拷贝中被拔出 | 高 | 捕获 IO 错误优雅退出；已完成的文件不受影响 |
| EXIF 日期提取库性能差 | 中 | 仅提取必需字段；对大文件只读前 1MB；可缓存结果 |

---

## 13. 附录

### A. 文件名冲突处理

同一张卡多次使用导致文件名重复（如 Sony 的 `C0001.MP4` 每次格式化后重置）：

**策略**: 文件名本身不冲突，因为目标路径已按日期+事件隔离。若同一事件+设备出现同名文件（极少见），比较文件大小和 hash：
- 若完全相同 → 跳过（增量）
- 若不同 → 追加序号：`C0001.MP4` → `C0001_1.MP4`

### B. 支持的媒体格式

| 类型 | 扩展名 |
|------|--------|
| 视频 | `.MP4`, `.MOV`, `.M4V`, `.AVI`, `.MKV`, `.MTS`, `.M2TS` |
| 照片 | `.JPG`, `.JPEG`, `.ARW`, `.RAW`, `.CR2`, `.CR3`, `.NEF`, `.DNG`, `.HEIC`, `.PNG` |
| 音频 | `.WAV`, `.MP3`, `.AAC`, `.BWF` |
| 其他 | `.LRC` (歌词/字幕), `.SRT`, `.XML` (项目文件) |

**注意**: 即使是不认识的扩展名，若在同一目录下也应一并拷贝（如 `.THM` 缩略图、`.XML` 项目元数据）。

### C. 参考与竞品分析

| 工具 | 优点 | 缺点 |
|------|------|------|
| Kocard | 功能全面、UI 美观 | 收费、封闭源码、无法自定义 |
| Hedge | 专业校验、多备份 | 收费、macOS only |
| ShotPut Pro | 影视行业标准 | 极贵、仅 macOS |
| rsync | 免费、强大 | 无设备识别、无 EXIF 处理、无模板 |
| 手动拷贝 | "免费" | 极易出错、耗时、无校验 |

**Ingest 定位**: rsync 的可靠性 + Kocard 的易用性 + 完全开源可定制。

---

**文档结束**
