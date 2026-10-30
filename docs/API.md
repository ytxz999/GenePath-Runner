# API 参考

公开命名空间：`/api/v1`（`/v1` 作为旧链接别名保留）。除报告、快照与页面外，响应均为 JSON，错误体统一为：

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
| GET | `/api/v1/runs/:id` | 运行详情：参数、条目、事件、基因、未映射基因、归档文件、重跑来源 |
| GET | `/api/v1/runs/:id/results` | 富集条目 + `provider` 溯源 + 每个来源的阈值 |
| GET | `/api/v1/runs/:id/raw` | 保存的原始请求体与原始响应体 |
| GET | `/api/v1/runs/:id/report.html` | 自包含 HTML 报告（`text/html`，strict CSP，可离线打开） |
| GET | `/api/v1/runs/:id/report.md` | Markdown 报告（`text/markdown`） |
| GET | `/api/v1/runs/:id/manifest.json` | 复现清单（`application/json; charset=utf-8`，带 `Content-Disposition`） |
| GET | `/api/v1/runs/:id/artifacts` | 归档列表：`kind`、`bytes`、`sha256`、`url`、`createdAt` |
| GET | `/api/v1/runs/:id/artifacts/:kind` | 下载单个归档文件 |
| POST | `/api/v1/runs/:id/rerun` | 用同一组输入与参数**新建**一次运行，返回 `201` |
| DELETE | `/api/v1/demo` | 删除标注过的演示数据（真实运行不受影响） |

`kind` 取值：`report_html`、`report_md`、`manifest`、`request_json`、`response_json`。`manifest` 不会把自身列进清单。

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
| `significance` | 否 | 0–1 之间，默认 `0.05`；字符串 `"0.05"` 与数字 `0.05` 都接受 |
| `correction` | 否 | `g_SCS`（默认）/ `bonferroni` / `fdr` |
| `background` | 否 | `annotated`（默认）/ `genome` |
| `label` | 否 | 任务名称，省略时按物种与基因数自动生成 |

提交成功返回 `201`，`status` 初始为 `queued`，随后由 worker 推进为 `running` → `succeeded` / `failed`。

## 查询任务

```bash
curl -s http://127.0.0.1:8080/api/v1/runs/run_71676eba30c6
```

关键字段：`status`（`queued` / `running` / `succeeded` / `failed` / `canceled`）、`outcome`（`enriched` / `no_significant_terms` / `no_genes_mapped`）、`geneCount`、`mappedCount`、`resultCount`、`dataVersion`、`httpStatus`、`requestBytes`、`responseBytes`、`unmapped`、`duplicates`、`events`、`artifacts`、`rerunFrom`。

## 归档与下载

一次成功的运行会在 `-artifacts` 目录（默认 `artifacts/`）下生成：

```text
artifacts/run_71676eba30c6/
  request.json      上游请求体
  response.json     上游原始响应
  manifest.json     复现清单
  report.md         Markdown 报告
  report.html       自包含 HTML 报告
```

每个文件的 SHA-256 与字节数写入 `artifacts` 表，`GET /runs/:id/artifacts` 返回：

```json
[
  {
    "kind": "manifest",
    "label": "运行清单",
    "bytes": 155138,
    "sha256": "81338055f8…",
    "href": "/api/v1/runs/run_71676eba30c6/artifacts/manifest",
    "createdAt": "2026-10-30T19:42:19+08:00"
  }
]
```

下载接口在文件缺失时会按数据库内容重新生成并回写，因此删除 `artifacts/` 目录不会让历史失效（哈希校验见 `TestArtifactBytesRegeneratesADeletedFile`）。

## manifest.json

字段与 `dto.Manifest` 一一对应（snake_case）：

```json
{
  "project": "GenePath Runner",
  "schema_version": "1.0",
  "run_id": "run_71676eba30c6",
  "label": "Apoptosis panel — TP53 通路复现",
  "status": "succeeded",
  "outcome": "enriched",
  "input_hash": "460c9c480c9f91c941111cbce01d4439a56b21a4b20b2ac4dbb642593256cb0c",
  "hash_algorithm": "sha256",
  "organism": "hsapiens",
  "organism_label": "Homo sapiens",
  "input_genes": ["TP53", "BAX", "CDKN1A", "MDM2", "BBC3", "PUMA"],
  "input_gene_count": 6,
  "parameters": {
    "significance": "0.05",
    "correction": "g_SCS",
    "background": "annotated",
    "sources": ["GO:BP", "KEGG"]
  },
  "data_source": "g:Profiler",
  "data_source_endpoint": "https://biit.cs.ut.ee/gprofiler/api/gost/profile/",
  "data_version": "e114_eg62_p19_9dc92ac8",
  "created_at": "2026-10-30T19:42:17+08:00",
  "started_at": "2026-10-30T19:42:17+08:00",
  "finished_at": "2026-10-30T19:42:19+08:00",
  "duration_ms": 2453,
  "upstream_http_status": 200,
  "request_snapshot": {
    "organism": "hsapiens",
    "query": ["TP53", "BAX", "CDKN1A", "MDM2", "BBC3", "PUMA"],
    "user_threshold": 0.05,
    "significance_threshold_method": "g_SCS"
  },
  "response_snapshot": {
    "result": ["…"],
    "meta": { "version": "e114_eg62_p19_9dc92ac8", "timestamp": "2026-10-30T11:42:17+00:00" }
  },
  "result_count": 148,
  "mapping": {
    "submitted": 6,
    "mapped": 5,
    "unmapped": ["PUMA"],
    "duplicates": [],
    "message": "映射 5/6 个基因到注释库；未映射：PUMA"
  },
  "artifacts": [
    {
      "kind": "response_json",
      "label": "原始响应",
      "bytes": 100232,
      "sha256": "8e4a431a…",
      "href": "/api/v1/runs/run_71676eba30c6/artifacts/response",
      "createdAt": "2026-10-30T19:42:19+08:00"
    }
  ],
  "notes": [
    "input_hash 只覆盖规范化后的基因符号与物种，不含时间戳，因此同一基因集重复粘贴会得到相同哈希。",
    "本文件是历史存档：重新打开是回看当时的结果；重新执行会生成新的运行记录。",
    "g:Profiler 的数据版本会更新，数据源更新后不保证得到完全相同的统计结果。",
    "所有 p 值、Term 规模等统计量均来自 g:Profiler 返回值，GenePath 未重新计算。"
  ]
}
```

`request_snapshot` / `response_snapshot` 是解析后的原始 JSON。若原始响应超过 512 KiB，`response_snapshot` 退化为 `{ "embedded": false, "artifact": "response.json", "sha256": "…", "bytes": …, "note": "…" }`，完整字节始终在同目录的原始文件里。

## 错误码

| HTTP | `code` | 场景 |
| --- | --- | --- |
| 400 | `bad_request` | 请求体不是合法 JSON |
| 404 | `not_found` | 运行、页面或归档文件不存在 |
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
