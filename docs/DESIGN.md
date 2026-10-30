# 设计说明

GenePath Runner 只做三件事：**把一次富集分析提交出去、把上游的答案原样存下来、让它在几个月后还能被复现**。富集统计全部由 g:Profiler 完成，本项目不实现也不改写任何统计算法。

## 分层

```text
handler (Gin)  →  service  →  repository  →  database (GORM + SQLite)
                     ↑
                executor (worker 池)
                     ↑
                provider (g:Profiler REST)
```

- `handler` 只负责绑定参数与映射错误，不写业务判断。
- `service` 不 import Gin，因此可以脱离 HTTP 直接测试；所有状态流转都在这里。
- `provider.Client` 是接口，测试用 httptest 或假实现替换，所以测试不依赖网络。
- 提交立即返回 `201`，真正调上游的是 `executor` 里的 worker，前端轮询 `/runs/:id`。

## 数据模型

| 表 | 内容 |
| --- | --- |
| `runs` | 一次提交的不可变记录：`input_hash`（规范化基因集的 SHA-256，顺序与大小写无关）、`organism`、参数快照 JSON、`source_version`、`status`、`outcome`、`gene_count`、`mapped_count`、`term_count`、耗时、上游 HTTP 状态与字节数、`is_demo` |
| `terms` | 上游条目逐字段保存：`p_value`、`term_id`、`native`、`name`、`source`、`intersection_size`、`query_size`、`term_size`、命中基因 |
| `run_events` | 生命周期事件：提交、入队、开始执行、调用上游、基因映射、完成、失败，驱动结果页的审计轨迹 |

## 状态机

```text
queued ──► running ──► succeeded
   │           │           └─ outcome: enriched / no_significant_terms / no_genes_mapped
   │           └─────────► failed
   └─────────► canceled
```

`succeeded` 不是"有结果"的同义词：请求成功但零显著条目、请求成功但一个基因都没匹配上，都是真实结局，必须分开呈现，不能都渲染成"没有显著结果"。

失败时 `error_code` 区分 `provider_error`、`provider_timeout`、`internal_error`，原始响应体若存在也会保存。

## g:Profiler 接入

请求按官方文档构造，不猜字段：

```json
{
  "organism": "hsapiens",
  "query": ["TP53", "BAX", "CDKN1A"],
  "sources": ["GO:BP", "KEGG"],
  "user_threshold": 0.05,
  "significance_threshold_method": "g_SCS",
  "background": "annotated",
  "domain_scope": "annotated",
  "no_evidences": false,
  "all_results": false
}
```

要点：

1. **超时**：`context` 与 `http.Client.Timeout` 双重约束，超时映射为 504 并落库。
2. **状态码**：非 2xx 一律失败，保存状态码与响应摘要，不把错误包装成"零结果"。
3. **数据源版本**：取响应的 `meta.version`（演示环境为 `e114_eg62_p19_9dc92ac8`），连同 `meta.timestamp` 一起落库。
4. **原始留存**：请求体与响应体原样保存，`/runs/:id/raw` 可下载；6 个基因的响应约 100 KB，不截断。
5. **统一结构**：上游 JSON 在 `service` 层转成内部 `Term`，前端只依赖内部结构。
6. **映射统计**：`genes_metadata` 的 `failed`、`duplicates`、`ambiguous`、`query.mapping` 用来展示"输入 N 个、映射 M 个、未映射 …"，未映射基因显式展示。
7. **限流**：公共接口有速率限制（响应头带 `X-RateLimit-*`），所以用有界队列与有限并发，不做无限制并发提交。

## 界面约定

- 设计令牌集中在 `web/static/assets/css/tokens.css`：深海军蓝侧边栏、靛蓝主操作色、青绿辅助色、浅灰白画布。
- 无框架、无构建步骤、无 CDN；静态资源 `go:embed` 进二进制，CSP 严格模式（无 inline script/style）。
- 加载 = 骨架屏，空 = 说明"为什么空"，错误 = 可重试提示；排队中的运行看到的是等待说明，而不是一张假的成绩单。
- 桌面优先，≤900px 侧边栏收为抽屉，表格在卡片内横向滚动而不是把单元格挤成竖线。

## 测试

```bash
go test ./... -count=1
go test ./... -count=1 -cover -p 1
go vet ./...
```

最近一次本地结果（Windows，Go 1.26.4，43 个测试函数）：

| 包 | 覆盖率 | 覆盖的行为 |
| --- | --- | --- |
| `internal/gene` | 97.9% | 分隔符、注释行、大小写去重、非法字符、上限、哈希顺序无关 |
| `internal/executor` | 91.4% | 队列满、状态推进、重试、超时、panic 兜底、优雅退出 |
| `internal/httpapi` | 74.5% | 路由契约、字段校验 422、404、CSP、静态资源、页面路由 |
| `internal/provider/gprofiler` | 74.1% | 请求字段、超时、非 2xx、坏 JSON、`meta.version`、映射统计 |
| `internal/service` | 62.8% | 三种 outcome、失败落库、重复哈希提示、报告渲染 |

## 诚实性约定

- 不重算 p 值，不修改上游统计结果，不在界面上给生物学结论。
- 演示数据默认关闭；开启时永远带「演示」标记，`is_demo` 落库，报告里注明。
- 上游失败就显示失败，不会把"请求失败"渲染成"没有显著结果"。
- 未映射、重复、无效的基因都显式展示，不悄悄丢弃。
- 不把大模型生成的生物学解释写进结果页——那正是这个项目要避免的"看起来可信"。

## 已知限制

- 单机 SQLite 写入串行，适合个人与小团队；多实例部署需要换 Postgres 并引入任务租约。
- GORM `AutoMigrate` 没有版本化迁移表，字段变更需要手工处理。
- 静态资源路径固定、无内容指纹，改前端后需要强制刷新（浏览器会缓存旧 JS）。
- 公共 g:Profiler 接口限流，批量提交会排队而不是并发轰炸。
