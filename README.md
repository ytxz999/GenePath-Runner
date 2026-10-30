# GenePath Runner

> 把一组基因变成一份可复现的富集分析记录。
> 输入基因 → 调用 g:Profiler → 保存原始请求/响应、结果、数据源版本 → 导出 HTML / Markdown / manifest 报告。

GenePath Runner 是一个**围绕 g:Profiler 的运行记录与可复现性平台**：它不重新实现富集算法，只负责把"谁、用什么输入、什么参数、在哪个数据源版本下、得到了什么"完整地留下来，并且能被第三方复核。

![工作台](docs/screens/p4-dashboard.png)

---

## 1. 它解决什么问题

科研里常见的情况是：三个月前跑过一次富集分析，图放进论文了，但现在说不清当时用的是哪个注释版本、哪些基因没被映射到、参数是什么。

GenePath Runner 把每次分析都当成一次**带证据的运行**：

- 输入的基因集规范化后算 SHA-256，同样一组基因（顺序、大小写不同）得到同一个哈希；
- 语义相同的重复提交会被识别出来，提示"与已有运行 run_xxx 的输入哈希一致"；
- 原始 HTTP 请求体、原始响应 JSON 原样落盘，并单独记录 SHA-256；
- 记录 g:Profiler 返回的数据源版本（如 `e114_eg62_p19_9dc92ac8`），报告里明确写出"数据源更新后不保证重跑得到完全相同的数值"；
- 一键导出 HTML 报告 / Markdown 报告 / manifest.json，也可以回看原始请求与响应。

**科学边界**：GenePath 不重新计算 p 值，不修改上游统计量，不生成生物学结论。所有富集数字都来自 g:Profiler 的返回值。

---

## 2. 功能一览

### 已实现

| 能力 | 说明 |
| --- | --- |
| 基因输入 | 换行 / 逗号 / 分号 / 空格分隔，去重、去空白、大小写归一，上限 2000 个；实时显示有效基因数、重复数、非法符号数 |
| 物种与参数 | 人 / 小鼠 / 大鼠 / 斑马鱼，GO:BP、GO:CC、GO:MF、KEGG、Reactome、WikiPathways；显著性阈值、多重检验校正（g:SCS 等）、背景基因集 |
| 真实上游调用 | 后台 worker 调用 g:Profiler REST，记录 HTTP 状态、耗时、请求/响应体积，区分成功有结果 / 成功无显著结果 / 无基因映射 / 上游失败 / 超时 |
| 运行记录 | 状态机（排队中 → 运行中 → 已完成 / 失败）、任务日志、开始与结束时间、错误原因、输入哈希 |
| 结果展示 | 富集强度条形图、结果表格（来源筛选、关键词搜索、p 值排序）、命中基因、运行参数、溯源信息、审计轨迹 |
| 归档产物 | 每次成功运行生成 5 个文件：`request.json`、`response.json`、`manifest.json`、`report.html`、`report.md`，均记录 SHA-256 |
| 报告导出 | HTML（自包含、可打印）、Markdown、manifest.json，全部可从页面下载 |
| 重跑 | "重新运行"用同一输入与参数新建一次运行，旧记录保持不变；明确区分"回看存档"与"重新执行" |
| 历史与工作台 | 真实统计（总数/成功/失败/平均耗时/14 天趋势/来源分布）、搜索与筛选、空状态设计 |
| 工程化 | 单元与集成测试、GitHub Actions CI、Dockerfile、docker-compose、Makefile、严格 CSP、结构化错误响应 |

### 未实现（明确不做，或留待以后）

- 用户体系与鉴权（单用户本地工具，绑定 `127.0.0.1`）；
- 任务取消 / 真正的中断恢复（状态字段保留，但没有 cancel 接口）；
- 多机部署与外部数据库（依赖本地 SQLite，写入串行）；
- 结果的可视化富集气泡图、通路网络图；
- g:Profiler 之外的注释源（如 DAVID、Enrichr）；
- 统计学二次计算（刻意不做）。

---

## 3. 快速开始（本地）

需要 Go 1.26+（`go version` 查看）。**不需要** Node、数据库或 Docker。

```bash
git clone https://github.com/ytxz999/GenePath-Runner.git
cd GenePath-Runner

go build -o bin/genepath ./cmd/server        # Linux / macOS
go build -o bin/genepath.exe ./cmd/server    # Windows

./bin/genepath -addr 127.0.0.1:8080 -db data/genepath.db
```

打开 <http://127.0.0.1:8080>，在"新建分析"里粘贴几个基因（例如 `TP53, BAX, CDKN1A, MDM2, BBC3`），提交后几秒内会看到真实结果。

首次启动会自动建库（`data/genepath.db`）并创建 `data/artifacts/`。若想先看看界面，可加 `-seed=true` 写入 42 条**明确标记为「演示数据」**的示例记录（页面会显示「演示数据」徽章，不会伪装成真实分析）。

常用命令：

```bash
go test ./... -count=1                                   # 全部测试
go test ./internal/... -count=1 -cover                   # 覆盖率
gofmt -l . && go vet ./...                               # 格式与静态检查
```

---

## 4. 用 Docker 运行

```bash
docker compose up --build      # 构建并启动，监听 http://localhost:8080
docker compose down            # 停止，数据保留在 named volume
docker compose down -v         # 连同数据一起删除
```

或者手动：

```bash
docker build -t genepath-runner:0.4.0 .
docker run --rm -p 8080:8080 -v genepath-data:/data genepath-runner:0.4.0
```

镜像基于 Alpine，`CGO_ENABLED=0` 静态编译（SQLite 驱动是纯 Go 实现），以非 root 用户运行，带 `HEALTHCHECK`，数据落在 `/data`。

> 说明：本机开发环境的 Docker daemon 不可用，因此镜像构建与容器冒烟由 `.github/workflows/ci.yml` 的 `docker` job 在 CI 上执行；见 [docs/testing.md](docs/testing.md) 的「验证范围」一节。

---

## 5. 配置

命令行参数优先，其次环境变量，最后是默认值。

| 参数 | 环境变量 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `-addr` | `GENEPATH_ADDR` | `127.0.0.1:8080` | HTTP 监听地址 |
| `-db` | `GENEPATH_DB` | `data/genepath.db` | SQLite 文件路径 |
| `-artifacts` | `GENEPATH_ARTIFACTS` | 数据库同级的 `artifacts/` | 报告与清单文件目录 |
| `-seed` | `GENEPATH_SEED` | `false` | 首次启动写入演示数据（带标记） |
| `-workers` | `GENEPATH_WORKERS` | `2` | 并行 worker 数 |
| `-provider-timeout` | `GENEPATH_PROVIDER_TIMEOUT` | `90s` | 单次上游调用超时 |
| `-provider-endpoint` | `GPROFILER_ENDPOINT` | `https://biit.cs.ut.ee/gprofiler/api/gost/profile/` | 上游接口地址 |
| `-version` | — | `0.4.0` | 显示在页脚与 `/healthz` |
| `-dev` | — | `false` | 开发模式：更松的 CSP 与详细日志 |

---

## 6. 架构

```text
浏览器 (原生 HTML/CSS/JS, 无框架)
   │  fetch JSON
   ▼
Gin 路由 / middleware            internal/httpapi
   │                             ├─ 请求 ID、panic 恢复、访问日志
   │                             ├─ 安全响应头 + 严格 CSP
   ▼
RunService（业务编排）            internal/service
   ├─ Validate / Create          ├─ 规范化、校验、哈希、查重
   ├─ ExecuteRun                 ├─ 调上游、写结果、记审计
   ├─ Report / Manifest          ├─ HTML/Markdown/manifest 生成
   └─ Dashboard / List           └─ 统计与分页
   │                    │
   ▼                    ▼
Repository          Executor（队列 + worker）
GORM / SQLite       internal/executor
internal/repository  ├─ 有界队列（满则 503）
                     ├─ 每任务独立超时
                     └─ panic 不影响其他任务
                          │
                          ▼
                    g:Profiler REST 客户端
                    internal/provider/gprofiler
```

分层规则：handler 只做参数绑定与状态码，service 承载业务，repository 只碰数据库，provider 只碰 HTTP。`/api/v1` 与 `/v1` 是同一组接口的两个挂载点（后者保留兼容）。

---

## 7. 数据模型

四张表，够用即可，没有为了"看起来复杂"而加表。

| 表 | 关键字段 | 用途 |
| --- | --- | --- |
| `runs` | `id`、`status`、`outcome`、`organism`、`genes`、`input_hash`、`params`(JSON)、`data_version`、`raw_request`、`raw_response`、`http_status`、`upstream_ms`、`started_at`、`finished_at`、`error_*` | 一次分析的全部元数据与原始交换 |
| `run_terms` | `run_id`、`source`、`term_id`、`name`、`p_value`、`term_size`、`query_size`、`intersection_size`、`genes`、`significant` | g:Profiler 返回的每一条富集结果，字段与上游一一对应 |
| `run_events` | `run_id`、`type`、`message`、`created_at` | 追加式审计轨迹（已提交 → 进入队列 → 开始执行 → 调用上游 → 基因映射 → 已完成） |
| `artifacts` | `run_id`、`kind`、`path`、`sha256`、`bytes`、`created_at` | 归档文件索引；`(run_id, kind)` 唯一 |

`params` 存的就是提交时的参数快照，`raw_response` 存的是上游返回的完整 JSON。

---

## 8. API

所有响应都是 JSON；出错时返回统一结构：

```json
{
  "error": { "code": "validation_failed", "message": "输入有误，请检查标出的字段" },
  "fields": [{ "field": "genes", "message": "至少需要一个有效基因符号" }],
  "requestId": "req_..."
}
```

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 存活 + 数据库探活 |
| GET | `/readyz` | 就绪（执行器 + 队列） |
| GET | `/api/v1/meta` | 支持的物种、来源、参数选项、上游信息 |
| GET | `/api/v1/dashboard` | 统计、14 天趋势、状态分布、来源分布、最近运行 |
| GET | `/api/v1/runs?status=&q=&organism=&limit=&offset=` | 运行列表（分页） |
| POST | `/api/v1/runs` | 提交分析，返回 `201` 与运行详情 |
| POST | `/api/v1/runs/validate` | 只校验不提交（前端实时校验用） |
| GET | `/api/v1/runs/{id}` | 运行详情（含审计事件、映射信息） |
| GET | `/api/v1/runs/{id}/results` | 结果条目（含显著性与排序） |
| GET | `/api/v1/runs/{id}/raw` | 原始请求体与响应体 |
| GET | `/api/v1/runs/{id}/artifacts` | 产物清单（kind / 大小 / SHA-256） |
| GET | `/api/v1/runs/{id}/artifacts/{kind}` | 下载任一产物 |
| GET | `/api/v1/runs/{id}/report.html` | HTML 报告（内联展示） |
| GET | `/api/v1/runs/{id}/report.md` | Markdown 报告（附件下载） |
| GET | `/api/v1/runs/{id}/manifest.json` | manifest（附件下载） |
| POST | `/api/v1/runs/{id}/rerun` | 用相同输入与参数新建一次运行 |
| DELETE | `/api/v1/demo` | 删除演示数据 |

示例：

```bash
# 提交
curl -s -X POST http://127.0.0.1:8080/api/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{"label":"p53 panel","genes":"TP53, BAX, CDKN1A, MDM2, BBC3","organism":"hsapiens","sources":["GO:BP","KEGG"],"significance":0.05,"correction":"g_SCS","background":"annotated"}'

# 查询
curl -s http://127.0.0.1:8080/api/v1/runs/run_xxxx | jq .run.status

# 下载报告
curl -sO -J http://127.0.0.1:8080/api/v1/runs/run_xxxx/report.md
curl -s http://127.0.0.1:8080/api/v1/runs/run_xxxx/manifest.json | jq .data_version
```

---

## 9. 可复现性设计

**输入哈希**：对规范化后的基因集合（排序、小写、去重）加上物种做 SHA-256，不带时间戳、不带参数。因此"同一组基因、不同顺序、不同大小写"得到同一个 32 字节哈希；换物种会变。旧的、大小写未归一的哈希值保持兼容（不会因为升级而让历史记录"对不上"）。

**manifest.json** 至少包含：

```json
{
  "run_id": "run_5028502f1ddc",
  "input_hash": "bab8f9850a06f38345...",
  "organism": "hsapiens",
  "input_genes": ["RAD51", "BRCA1", "ATM", "CHEK2", "TP53", "MDM2"],
  "parameters": { "sources": ["GO:BP","KEGG"], "significance": 0.05, "correction": "g_SCS", "background": "annotated" },
  "data_source": "g:Profiler REST",
  "data_version": "e114_eg62_p19_9dc92ac8",
  "created_at": "2026-10-30T21:15:00+08:00",
  "request_snapshot": { "organism": "hsapiens", "query": ["RAD51", "..."], "sources": ["GO:BP"], "user_threshold": 0.05 },
  "response_snapshot": { "meta": { "version": "e114_eg62_p19_9dc92ac8" }, "result": ["..."] },
  "artifacts": [ { "kind": "report_html", "path": "run_.../report.html", "sha256": "...", "bytes": 45212 } ]
}
```

`request_snapshot` / `response_snapshot` 是**原样快照**。响应过大（> 2 MB）时不会内联，而是写上 `{"artifact": "response.json", "sha256": "...", "bytes": ...}` 指向归档文件，manifest 里写明原因。

**回看 vs 重跑**：打开一条历史运行看到的是当时存档（数据库 + 归档文件），不会重新请求上游；"重新运行"会生成一条**新**记录，使用当时的输入与参数，但结果取决于上游当前的数据版本。报告里对此有明确说明。

---

## 10. 目录结构

```text
cmd/server/                 入口：参数、装配、优雅停机
internal/
  catalog/                  物种、来源、参数枚举（与前端共享的"字典"）
  config/                   默认配置与 env 覆盖
  database/                 SQLite 打开、迁移、演示数据
  dto/                      请求/响应结构（API 契约）
  executor/                 有界队列 + worker
  gene/                     解析、规范化、校验、哈希
  model/                    GORM 模型：runs / run_terms / run_events / artifacts
  provider/gprofiler/       g:Profiler REST 客户端（超时、重试、错误分类）
  repository/               数据访问
  service/                  业务编排：校验、执行、报告、统计
  httpapi/                  路由、handler、错误映射、安全头
web/
  embed.go                  go:embed 整个前端
  static/                   index.html / new.html / results.html / history.html
    assets/css/             tokens / app / pages / polish
    assets/js/              api / ui / 各页面脚本（原生 ES2020）
docs/
  API.md  DESIGN.md  testing.md  screens/
Dockerfile  docker-compose.yml  Makefile  .github/workflows/ci.yml
```

---

## 11. 测试

```bash
go test ./... -count=1
```

覆盖的内容包括：输入解析与哈希、参数校验边界、上游客户端（用 `httptest` 模拟 5xx、超时、坏 JSON、错误体）、队列语义（恰好一次、队列满、worker panic 隔离）、报告与 manifest 生成、HTML 转义、附件下载、重跑、404 语义、安全响应头、优雅停机、SQLite 重开后的持久化。

最近一次本地测量的语句覆盖率（`go test ./... -cover`）：

| 包 | 覆盖率 |
| --- | --- |
| `internal/gene` | 98.0% |
| `internal/executor` | 91.4% |
| `internal/httpapi` | 78.3% |
| `internal/provider/gprofiler` | 74.1% |
| `internal/service` | 66.4% |
| `internal/repository` | 21.1% |
| 全部语句 | **65.8%** |

功能测试矩阵、手动验证步骤与"哪些没验证"的说明见 [docs/testing.md](docs/testing.md)。

---

## 12. 设计取舍

- **Go + SQLite，而不是 Python + FastAPI**：项目卖点是"运行记录与工程可靠性"，Go 的静态二进制、单文件部署、并发与超时控制更直接；g:Profiler 是 HTTP 服务，用什么语言调用都一样，不需要 Python 生态。
- **不重算 p 值**：重算会引入"和上游不一致"的风险，也会让结论失去可追溯性。GenePath 只搬运与保存。
- **前端不用框架**：四个页面、一套设计变量，原生 JS 足够；避免构建链与依赖膨胀，也让 `go build` 之后二进制自带界面。
- **SQLite 单机**：瓶颈在外部 HTTP 调用而不是数据库；单机串行写入换来"一个文件就是全部状态"，便于复现与备份。
- **轮询而不是 WebSocket**：任务通常 1–3 秒完成，轮询实现与调试成本更低，也不会多一套连接状态要维护。
- **产物落盘 + 数据库索引**：报告是可被外部引用的文件，不应该只能从数据库重新渲染。

---

## 13. 已知限制

- 没有鉴权，默认只监听 `127.0.0.1`；不要直接暴露到公网。
- 单机 SQLite，多个实例不能共享同一个数据库文件。
- Docker 镜像构建与容器冒烟只配置在 CI 中，本机 Docker daemon 不可用，因此**未在本地实测**。
- 上游可用性依赖 g:Profiler；其数据版本会随时间变化，历史记录只保证"当时是什么"，不保证"重跑一模一样"。
- 没有实现任务取消；"中断"状态只在进程重启回收在途任务时出现。
- 前端未做系统性的无障碍审计（键盘可达、屏幕阅读器标签只覆盖了主要控件）。
- 界面只有中文。

---

## 14. 数据来源与引用

本项目调用 [g:Profiler](https://biit.cs.ut.ee/gprofiler/) 的公开 REST API，不缓存、不改写其统计结果。若在论文或报告中使用了分析结果，请引用：

- Raudvere U, Kolberg L, Kuzmin I, Arak T, Adler P, Peterson H, Vilo J. **g:Profiler: a web server for functional enrichment analysis and conversions of gene lists (2019 update)**. *Nucleic Acids Research*, 2019. doi:10.1093/nar/gkz369

GenePath Runner 自身只负责编排、记录与导出。

---

## 15. 许可证

MIT，见 [LICENSE](LICENSE)（如仓库中尚未包含该文件，以仓库实际声明为准）。
