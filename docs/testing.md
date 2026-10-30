# 测试与验证记录

本文记录"哪些行为被自动化测试覆盖、哪些是手动验证的、哪些没有验证"。
所有数字都来自本仓库的实际执行，不是估算。

- 代码版本：v0.4.0
- 环境：Windows 10.0.26200 / Go 1.26.4 windows-amd64 / Chrome 137.0.7151.69
- 上游：`https://biit.cs.ut.ee/gprofiler/api/gost/profile/`（真实网络）

---

## 1. 自动化测试

运行方式：

```bash
go test ./... -count=1
```

最近一次结果（全部通过）：

```text
ok  genepath/internal/executor          1.4s
ok  genepath/internal/gene              1.3s
ok  genepath/internal/httpapi           5.3s
ok  genepath/internal/provider/gprofiler 2.1s
ok  genepath/internal/repository        1.0s
ok  genepath/internal/service           5.0s
```

覆盖率（`go test ./... -cover`）：

| 包 | 语句覆盖率 |
| --- | --- |
| `internal/gene` | 98.0% |
| `internal/executor` | 91.4% |
| `internal/httpapi` | 78.3% |
| `internal/provider/gprofiler` | 74.1% |
| `internal/service` | 66.4% |
| `internal/repository` | 21.1% |
| 全部 | 65.8% |

`config`、`catalog`、`database`、`model`、`cmd/server` 没有独立测试，它们要么是常量/枚举，要么在集成测试里被间接覆盖。

---

## 2. 功能测试矩阵

| # | 场景 | 预期 | 验证方式 | 结果 |
| --- | --- | --- | --- | --- |
| 1 | 空输入 | 400，字段错误指向 `genes` | `TestValidationRejectsBadInput` | 通过 |
| 2 | 非法字符（`BAX!`） | 400，字段错误指向 `genes` | `TestValidationRejectsBadInput`、`TestValidRejectsProviderBreakingSymbols` | 通过 |
| 3 | 未知物种（`klingon`） | 400，字段错误指向 `organism` | `TestValidationRejectsBadInput` | 通过 |
| 4 | 未选来源 / 未知来源 | 400，指向 `sources` | `TestValidationRejectsBadInput` | 通过 |
| 5 | 非法阈值 / 校正 / 背景 | 400，指向对应字段 | `TestValidationRejectsBadInput` | 通过 |
| 6 | 超过 2000 个基因 | 400 | `TestGeneLimitIsEnforced` | 通过 |
| 7 | 大小写重复、逗号/换行混排 | 去重且保留首次拼写 | `TestParseHandlesCommonSeparators`、`TestParseDeduplicatesCaseInsensitivelyButKeepsFirstSpelling` | 通过 |
| 8 | 输入含 `#` 注释行、空行 | 跳过 | `TestParseSkipsCommentsAndBlankLines` | 通过 |
| 9 | 同一组基因不同顺序/大小写/提交时间 | 输入哈希相同 | `TestInputHashIgnoresOrderCaseAndSubmissionTime`、`TestHashIsStableAcrossCalls` | 通过 |
| 10 | 换物种 | 哈希变化 | `TestHashIsOrderAndCaseIndependentOnlyWithinOrganism` | 通过 |
| 11 | 升级前的旧哈希 | 仍然匹配，不被"修复"破坏语义 | `TestHashKeepsTheLegacyValueForUpperCaseSymbols`、`TestRehashRepairsLegacyHashes` | 通过 |
| 12 | 重复提交同一基因集 | 返回 `duplicate_of`，仍创建新记录 | `TestDuplicateSubmissionIsFlagged` | 通过 |
| 13 | 上游 5xx | 重试一次后判定失败，运行记录保留 | `TestProfileRetriesOnceOnProviderSideFailure`、`TestProviderFailureIsRecordedWithoutLosingTheRun` | 通过 |
| 14 | 上游超时 | 归类为超时，错误信息明确 | `TestProfileTimeoutIsClassified`、`TestPerRunTimeoutIsApplied` | 通过 |
| 15 | 上游返回坏 JSON | 归类为"响应无法解析" | `TestProfileInvalidJSONIsClassified` | 通过 |
| 16 | 上游错误体（如 `Invalid organism`） | 转成可读错误 | `TestProfileMapsProviderErrorBody` | 通过 |
| 17 | 空基因集不发请求 | 本地拒绝 | `TestProfileRejectsEmptyQueryWithoutCallingUpstream` | 通过 |
| 18 | 请求线格式 | 与官方文档字段一致（`organism`/`query`/`sources`/`user_threshold`/`significance_threshold_method`） | `TestProfileSendsDocumentedWireFormat` | 通过 |
| 19 | 有结果 | 条目入库、`outcome=enriched` | `TestEndToEndSuccessStoresRealTermsAndAudit` | 通过 |
| 20 | 成功但无显著条目 | `outcome=no_significant_terms` | `TestNoSignificantTermsOutcome` | 通过 |
| 21 | 没有基因被映射 | `outcome=no_genes_mapped` | `TestNoGenesMappedOutcome` | 通过 |
| 22 | 队列恰好执行一次 | 每条提交只被执行一次 | `TestQueueExecutesEverySubmittedRunExactlyOnce` | 通过 |
| 23 | 队列满 | 503 `queue_full` | `TestQueueRejectsWhenBufferIsFull` | 通过 |
| 24 | worker panic | 进程存活，任务标记失败 | `TestWorkerSurvivesAPanickingRun` | 通过 |
| 25 | 执行器未装配 | `/readyz` 503，提交返回 503 | `TestReadyzFailsWithoutExecutor`、`TestCreateRunAnswers503WhenExecutorIsMissing` | 通过 |
| 26 | 关闭服务 | 停止接收新请求，在途任务完成 | `TestShutdownStopsIntakeButLetsInflightFinish` | 通过 |
| 27 | 重开数据库 | 运行、条目、产物仍在 | `TestRecordsSurviveAReopen` | 通过 |
| 28 | 产物齐全 | 5 个文件 + 5 条 artifact 记录 | `TestSucceededRunWritesCompleteArtifactBundle` | 通过 |
| 29 | 归档文件被删 | 下载时按数据库内容重建 | `TestArtifactBytesRegeneratesADeletedFile` | 通过 |
| 30 | 产物清单 / 下载 | kind 白名单，未知 kind 404 | `TestArtifactListReturnsHashedFiles`、`TestUnknownArtifactKindIsRejected`、`TestGenericArtifactDownloadAndUnknownKind` | 通过 |
| 31 | manifest 内容 | 含 run_id / input_hash / organism / input_genes / parameters / data_source / data_version / created_at / 请求与响应快照 | `TestManifestDescribesInputsAndSnapshots` | 通过 |
| 32 | manifest 确定性 | 同一运行两次生成字节一致 | `TestManifestIsDeterministicJSON` | 通过 |
| 33 | manifest 下载 | `Content-Disposition: attachment`，JSON | `TestManifestEndpointIsDownloadableJSON` | 通过 |
| 34 | HTML 报告 | 自包含、严格 CSP、用户输入被转义 | `TestHTMLReportIsServedInlineWithATightCSP`、`TestReportHTMLIsSelfContainedAndEscapesInput` | 通过 |
| 35 | 重跑 | 新建第二条记录，旧记录字段不变 | `TestRerunEndpointCreatesASecondRun`、`TestRerunKeepsTheOriginalRecordIntact` | 通过 |
| 36 | 未知运行 / 未知路由 | 404 且是 JSON 错误体 | `TestUnknownRunAndUnknownRouteAreJSON` | 通过 |
| 37 | 安全响应头 | CSP / X-Content-Type-Options / Referrer-Policy 等 | `TestSecurityHeadersAreSet` | 通过 |
| 38 | 健康检查 | `/healthz` 报告数据库状态 | `TestHealthzReportsDatabase` | 通过 |
| 39 | 页面可访问 | 四个页面 + meta 正常 | `TestPagesAreServed`、`TestMetaDescribesCatalogue` | 通过 |
| 40 | 列表筛选与分页 | status / q / organism / limit | `TestListRunsFiltersAndPaginates` | 通过 |
| 41 | 空数据库工作台 | 统计全为 0 且结构合法 | `TestEmptyDashboardIsWellFormed` | 通过 |

---

## 3. 真实链路验证（手动）

用真实网络调用 g:Profiler，记录如下（本机，2026-10-30）：

| 项 | 值 |
| --- | --- |
| 运行 ID | `run_5028502f1ddc` |
| 输入 | RAD51, BRCA1, ATM, CHEK2, TP53, MDM2（6 个） |
| 物种 / 参数 | `hsapiens`，GO:BP + KEGG，阈值 0.05，g:SCS，annotated |
| 上游状态 | HTTP 200 |
| 数据源版本 | `e114_eg62_p19_9dc92ac8` |
| 返回条目 | 148 条（全部显著） |
| 基因映射 | 6/6（无未映射） |
| 上游耗时 / 总耗时 | 2425 ms / 2.45 s |
| 请求 / 响应体积 | 233 B / 98 KB |
| 产物 | `request.json` 233 B、`response.json` 100232 B、`manifest.json` 155138 B、`report.html` 56257 B、`report.md` 14444 B |

另有一条**包含未映射基因**的真实运行：`run_71676eba30c6`，输入 `TP53, BAX, CDKN1A, MDM2, BBC3, PUMA` → 映射 5/6，`PUMA` 在报告的"未映射基因"一节单独列出。

再有一条**成功但无显著条目**的真实运行：`run_9f8e2b8c48b1`，输入 `MCM2` → 0 条显著，页面显示"分析确实执行了，但这批基因没有达到阈值的条目"。

手动步骤：

1. 网页输入基因 → 提交 → 前端显示"提交中"并禁用按钮；
2. 结果页轮询到 `succeeded` 后渲染真实条目；
3. 关闭服务进程，重新启动，历史与详情仍可打开；
4. 下载 HTML / Markdown / manifest，与页面内容比对一致。

---

## 4. 界面验证（手动 + 截图）

- 桌面 1440×900（Chrome headless，`docs/screens/p4-*.png`）；
- 移动端 390×844（真实 viewport 模拟，`docs/screens/p4-*-mobile.png`），四个页面 `scrollWidth == 390`，无横向溢出；
- 浏览器控制台：四个页面均无 `error` / `warning` 级别日志（通过 CDP 监听 `Runtime.consoleAPICalled` 与 `ExceptionThrown` 采集）；
- 空状态：空数据库下工作台、历史页均显示设计过的空状态，统计数字为 0 而不是编造值。

未做的界面验证：Safari / Firefox 的视觉回归、屏幕阅读器实测、200% 缩放下的布局检查。

---

## 5. 验证范围（诚实说明）

| 能力 | 状态 |
| --- | --- |
| `go build` / `go vet` / `gofmt` | 本机通过 |
| `go test ./...` | 本机通过 |
| 真实 g:Profiler 调用 | 本机通过（见上表） |
| 进程重启后的持久化 | 本机通过 |
| Docker 镜像构建 | **未在本机验证**（本机 Docker daemon 不可用），已配置在 CI 的 `docker` job |
| 容器冒烟测试（healthz / 首页 / meta / 404） | **未在本机验证**，同上 |
| GitHub Actions | 推送后由 GitHub 执行；结果以仓库 Actions 页面为准 |
| 并发压力测试 | 未做（只有队列语义与"恰好一次"的单元测试） |
| 长时间运行 / 内存泄漏 | 未做 |
