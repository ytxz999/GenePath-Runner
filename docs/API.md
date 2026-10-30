# API 参考

公开命名空间：`/api/v1`（`/v1` 作为旧链接别名保留）。所有响应为 JSON，错误体统一为：

```json
{ "code": "validation_failed", "message": "genes: 请至少输入 1 个基因符号", "details": { "genes": "…" } }
```

每个响应都带 `X-Request-Id`，`-dev` 模式下 4xx/5xx 会把它写进日志。

## 端点

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 存活 + 数据库连通性，返回 `version`、`uptimeMs` |
| GET | `/readyz` | 就绪检查，执行器未接入时 503 |
| GET | `/api/v1/meta` | 物种、注释来源、阈值与校正选项、上游名称与地址 |
| GET | `/api/v1/dashboard` | 统计、14 天活动、状态分布、来源分布、平均耗时、成功率、最近运行 |
| GET | `/api/v1/runs` | 分页列表，支持 `status` / `organism` / `q` / `limit` / `offset` |
| POST | `/api/v1/runs` | 提交分析，返回 `201` 与运行详情 |
| POST | `/api/v1/runs/validate` | 只校验不落库，供前端实时统计 |
| GET | `/api/v1/runs/:id` | 运行详情：参数、条目、事件、基因、未映射基因、上游元数据 |
| GET | `/api/v1/runs/:id/results` | 富集条目 + `provider` 溯源 + 每个来源的阈值 |
| GET | `/api/v1/runs/:id/raw` | 保存的原始请求体与原始响应体 |
| GET | `/api/v1/runs/:id/report.md` | 服务端渲染的 Markdown 报告 |
| DELETE | `/api/v1/demo` | 删除标注过的演示数据（真实运行不受影响） |

## 提交一次分析

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{
    "label": "Apoptosis panel",
    "genes": "TP53, BAX, CDKN1A, MDM2, BBC3, PUMA",
    "organism": "hsapiens",
    "sources": ["GO:BP", "KEGG"],
    "significance": "0.05",
    "correction": "g_SCS",
    "background": "annotated"
  }'
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `genes` | 是 | 换行 / 逗号 / 分号 / 空格分隔，自动去重、大小写不敏感，上限 2000 |
| `organism` | 是 | 见 `/api/v1/meta`，例如 `hsapiens` |
| `sources` | 否 | 注释来源列表；省略时用默认来源，传空数组会返回 422 |
| `significance` | 否 | 0–1 之间，默认 `0.05` |
| `correction` | 否 | `g_SCS`（默认）/ `bonferroni` / `fdr` |
| `background` | 否 | `annotated`（默认）/ `genome` |
| `label` | 否 | 任务名称，省略时按物种与基因数自动生成 |

## 查询任务

```bash
curl -s http://127.0.0.1:8080/api/v1/runs/run_71676eba30c6
```

关键字段：`status`（`queued` / `running` / `succeeded` / `failed` / `canceled`）、`outcome`（`enriched` / `no_significant_terms` / `no_genes_mapped`）、`geneCount`、`mappedCount`、`resultCount`、`dataVersion`、`httpStatus`、`requestBytes`、`responseBytes`、`unmapped`、`duplicates`、`events`。

## 错误码

| HTTP | `code` | 场景 |
| --- | --- | --- |
| 400 | `bad_request` | 请求体不是合法 JSON |
| 404 | `not_found` | 运行或页面不存在 |
| 409 | `queue_full` | 队列已满，稍后重试 |
| 422 | `validation_failed` | 字段校验失败，`details` 为 `字段 → 提示` |
| 502 | `provider_error` | 上游非 2xx 或响应无法解析 |
| 503 | `executor_not_ready` | 服务以无 worker 模式启动 |
| 504 | `provider_timeout` | 上游超时 |

## 结果结构

`/results` 的 `terms` 逐条对应 g:Profiler 的原始条目，字段保持上游语义，不做统计变换：

```json
{
  "source": "GO:BP",
  "termId": "GO:0072332",
  "name": "intrinsic apoptotic signaling pathway by p53 class mediator",
  "pValue": 7.0276599181867473E-10,
  "querySize": 5,
  "intersection": 5,
  "significant": true,
  "genes": ["TP53", "BAX", "CDKN1A", "MDM2", "BBC3"],
  "rank": 1
}
```

`provider` 部分给出 `name`、`endpoint`、`dataVersion`（如 `e114_eg62_p19_9dc92ac8`）、`configured`、`ready`；`thresholds` 给出每个来源实际的 `domainSize`、`numberOfTerms` 与阈值。
