# 贡献指南

感谢对 ingest 的兴趣。本文涵盖参与开发所需的全部内容：环境搭建、项目结构、分支策略、提交规范、代码风格、发布流程。

---

## 开发环境

### 前置要求

- **Go 1.24+** — 构建会自动升级到 `go.mod` 中固定的 toolchain 版本（当前 1.25.0）
- **git 2.x**，遵循 gitflow 习惯（见 [分支策略](#分支策略-gitflow)）
- 可选：`golangci-lint`、`make`

### 初始化

```bash
git clone https://github.com/hktkzyx/ingest.git
cd ingest
go mod download
go build ./...
go vet ./...
```

### 不安装直接运行

```bash
go run ./cmd/ingest --source /tmp/fake-card --target /tmp/out --name test --dry-run
```

### 本地安装自用

```bash
go install ./cmd/ingest
# 二进制位置：$(go env GOBIN)，默认 ~/go/bin
```

---

## 项目结构

| 路径 | 职责 |
|---|---|
| `cmd/ingest/` | CLI 入口——只做 flag 解析与流程串联，**不放业务逻辑** |
| `internal/scanner/` | 遍历源卷，区分媒体文件与 sidecar |
| `internal/device/` | 设备识别规则与匹配器；新增内置设备从这里加 |
| `internal/period/` | 时间段推断；当前基于 mtime，预留 EXIF 接入位 |
| `internal/template/` | 路径模板解析与渲染 |
| `internal/copier/` | 安全拷贝协议——**关键路径，谨慎修改** |
| `internal/db/` | SQLite 历史库；Schema 在 `db.go` 的 `schema` 常量里 |
| `PRD.md` | 权威产品规格 |

**依赖规则**：`cmd/ingest` 可以引用任何 `internal/*`；`internal/*` 之间应保持独立。当前唯一刻意的跨包依赖是 `copier` → `db`（安全拷贝协议要内联写入历史）。

---

## 分支策略 gitflow

仓库遵循标准的 [gitflow](https://nvie.com/posts/a-successful-git-branching-model/) 模型。

| 分支 | 来源 | 合入 | 用途 |
|---|---|---|---|
| `main` | — | — | 生产环境。仅打 tag 的提交，不直接编辑 |
| `develop` | `main`（初始） | `main`（经 `release/*`） | 集成分支。日常开发的默认基准 |
| `feature/<名称>` | `develop` | `develop` | 新功能、重构、非紧急修复 |
| `release/<版本>` | `develop` | `main` + 回流到 `develop` | 发布稳定化、改版本号、整理 changelog |
| `hotfix/<版本>` | `main` | `main` + 回流到 `develop` | 紧急生产修复 |

### 工作流

**功能开发**

```bash
git checkout develop && git pull
git checkout -b feature/exif-date-extraction
# ... 写代码、提交 ...
git push -u origin feature/exif-date-extraction
# 提 PR：feature/exif-date-extraction → develop
```

**发布**

```bash
git checkout develop && git pull
git checkout -b release/0.1.0
# 改 cmd/ingest/main.go 里的 version、补 CHANGELOG.md、做最后修缮
git push -u origin release/0.1.0
# PR 1：release/0.1.0 → main，merge commit 方式合入并打 v0.1.0 tag
# PR 2：release/0.1.0 → develop（或 main → develop），把发布期间的修缮回流到 develop
```

**热修**

```bash
git checkout main && git pull
git checkout -b hotfix/0.1.1
# 修复
git push -u origin hotfix/0.1.1
# PR 1：hotfix/0.1.1 → main，merge commit + 打 v0.1.1 tag
# PR 2：hotfix/0.1.1 → develop，让 develop 不落后于 main
```

### 硬性规则

- **绝不**直接提交到 `main` 或 `develop`
- **绝不**强推 `main` 或 `develop`
- 合入 `main` / `develop` 必须经过 PR 且至少一人 review
- feature PR 合入 `develop` 用 **squash merge**，保持线性历史
- release / hotfix PR 合入 `main` 用 **merge commit**，让合并点在 tag 处可见
- `main` 上每个提交都打 `vX.Y.Z` tag

---

## 提交信息

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
<type>(<scope>): <主题>

<正文>

<脚注>
```

**type**：`feat`、`fix`、`refactor`、`perf`、`test`、`docs`、`chore`、`build`、`ci`。

**scope** 与包名对应：`scanner`、`device`、`period`、`template`、`copier`、`db`、`cli`。跨包的用 `repo`。

**示例**

```
feat(device): 添加 Osmo Action 4 识别规则
fix(copier): src stat 失败时清理临时文件
refactor(template): 抽取段渲染逻辑到 helper
docs(readme): 补充 --template 语法
```

主题行用祈使句、≤72 字符、结尾不加句号。

---

## 代码风格

- 提交前跑 `gofmt -s`，CI 不通过会被打回
- `go vet ./...` 必须通过
- 优先标准库。当前直接依赖刻意保持精简：
  - `github.com/spf13/cobra` — CLI 框架
  - `github.com/cespare/xxhash/v2` — 哈希
  - `modernc.org/sqlite` — 纯 Go SQLite（无 CGO，跨编译更省事）

  新增直接依赖需要在 PR 描述里说明理由。
- **注释**：写"为什么"，不写"是什么"。代码本身能说明意图就别加注释；只有当不变量、绕坑、非显而易见的决策需要解释时才写。
- **错误处理**：调用方可能想 `errors.Is/As` 时用 `fmt.Errorf("context: %w", err)`，否则普通 `fmt.Errorf` 就够。错误一律向上传，不要 log-and-return。
- **不引入新全局变量**，除了 `cmd/ingest/main.go` 里现有的 cobra flag 绑定。依赖通过显式参数传入。

---

## 增加内置设备规则

编辑 `internal/device/device.go`，往 `BuiltinRules` 末尾追加：

```go
{
    ID: "osmoaction4", Name: "DJI Osmo Action 4", Manufacturer: "DJI",
    VolumeLabels: []string{"OSMO"},
    Directories:  []string{"DCIM"},
    FilePatterns: []string{"DJI_*.MP4"},
},
```

然后验证：

```bash
go build ./... && ingest devices list
```

置信度排序逻辑（卷标 → 目录结构 → 文件名 → 部分目录命中）在 `score()` 函数里。加新字段前，先确认现有匹配维度是否已经够用。

YAML 外置设备规则（`devices.yaml`）属于 PRD FR-012 的范畴。

---

## 修改 `copier.SafeCopy`

这是仓库里安全相关性最高的代码。修改前先做：

1. 完整重读 PRD §FR-005。
2. 保留以下不变量：
   - 目标文件**只能**通过 `os.Rename(tmp, dst)` 才以最终路径出现。
   - 写入历史库的 hash 必须既等于**源字节**的 hash、又等于**目标磁盘文件**的 hash。
   - 任何失败路径都要清掉临时文件。
   - 一次拷贝过程中，源文件只读一次（流式同时喂目标 writer 和源端 hasher，靠 `io.MultiWriter`）。
3. 拿不准的时候，**先写测试再改函数**。

---

## 测试

测试套件计划在 PRD Phase 4 补齐；当前仓库没有单元测试。新增测试时优先级：

1. `copier.SafeCopy`：黄金路径、hash 不匹配、增量跳过、入库正确、拷贝中断（kill 进程后 `dst` 不应有半成品）。
2. `template.Render`：变量为空时段省略、转义处理、未知变量报错。
3. `device.Detect`：置信度排序、多规则重叠、无匹配场景。

执行：

```bash
go test ./...
go test -race ./...
```

---

## 发布

仅维护者操作。

1. 从 `develop` 切 `release/<版本>`
2. 改 `cmd/ingest/main.go` 里的 `version`
3. 更新 `CHANGELOG.md`（Keep a Changelog 格式）
4. PR → `main`，merge commit 合入
5. 打签名 annotated tag：`git tag -s v<版本> -m "v<版本>"`，再 `git push --tags`
6. 把 `main` 回流到 `develop`（或直接把 release 分支合入 `develop`）
7. 构建二进制产物：`goreleaser`（规划中）或按平台 `go build`

---

## 报 Bug

到 <https://github.com/hktkzyx/ingest/issues> 开 issue，附上：

- 系统平台 + Go 版本（`go version`）
- 复现步骤——给出最小源目录结构最有帮助
- `--verbose` 输出和报错信息

如果怀疑数据损坏，请把 `~/.local/share/ingest/ingest.db` 中相关行附上：

```bash
sqlite3 ~/.local/share/ingest/ingest.db \
  "SELECT * FROM ingest_history WHERE target_path LIKE '%<文件名>%';"
```

---

## 许可证

随项目（待定，倾向 MIT 或 Apache-2.0）。
