package service

// reportHTMLTemplate is the standalone document served at
// /api/v1/runs/{id}/report.html. It carries its own styles so the file can be
// emailed, archived or printed without any external asset.
const reportHTMLTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · {{.Label}}</title>
<style>
  :root{
    --navy:#0b1b33; --ink:#101828; --muted:#5b6478; --line:#e3e7ee;
    --indigo:#4f46e5; --teal:#0f9d8f; --bg:#f5f7fa; --card:#ffffff;
  }
  *{box-sizing:border-box}
  body{margin:0;background:var(--bg);color:var(--ink);
    font:15px/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI","Noto Sans SC","PingFang SC","Microsoft YaHei",sans-serif;
    -webkit-font-smoothing:antialiased}
  .mono{font-family:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,monospace;font-size:.86em}
  .masthead{display:flex;justify-content:space-between;align-items:center;gap:24px;
    padding:22px 40px;background:var(--navy);color:#fff}
  .brand{display:flex;align-items:center;gap:12px}
  .mark{width:34px;height:34px;border-radius:9px;display:grid;place-items:center;
    background:linear-gradient(140deg,#4f46e5,#0f9d8f);font-weight:700;font-size:16px}
  .brand strong{display:block;font-size:15px;letter-spacing:.01em}
  .brand span{display:block;font-size:11px;text-transform:uppercase;letter-spacing:.12em;color:#9fb0c9}
  .masthead .meta{font-size:12px;color:#9fb0c9;text-align:right;line-height:1.8}
  main{max-width:1024px;margin:0 auto;padding:36px 24px 56px}
  .eyebrow{margin:0 0 6px;font-size:11px;letter-spacing:.16em;text-transform:uppercase;color:var(--muted)}
  h1{margin:0 0 14px;font-size:30px;line-height:1.25;letter-spacing:-.01em}
  .chips{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:26px}
  .chip{display:inline-flex;align-items:center;gap:6px;padding:4px 10px;border-radius:999px;
    border:1px solid var(--line);background:#fff;font-size:12px;color:#3c465c}
  .chip.ok{border-color:#bfe7df;background:#eefaf7;color:#0b6b60}
  .chip.warn{border-color:#f3d9a4;background:#fdf6e7;color:#8a5a12}
  .kpis{display:grid;grid-template-columns:repeat(4,1fr);gap:14px;margin:0 0 22px;padding:0}
  .kpi{background:var(--card);border:1px solid var(--line);border-radius:14px;padding:16px 18px}
  .kpi dt{font-size:12px;color:var(--muted);margin:0 0 8px}
  .kpi dd{margin:0;font-size:26px;font-weight:650;letter-spacing:-.02em}
  .kpi dd small{font-size:12px;font-weight:500;color:var(--muted);margin-left:4px}
  .card{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:22px 24px;margin-bottom:18px}
  .card h2{margin:0 0 4px;font-size:16px}
  .card .hint{margin:0 0 16px;font-size:12.5px;color:var(--muted)}
  .card .hint.spaced{margin-top:18px}
  dl.grid{display:grid;grid-template-columns:180px 1fr;gap:0;margin:0}
  dl.grid dt{padding:9px 0;border-bottom:1px dashed var(--line);color:var(--muted);font-size:13px}
  dl.grid dd{padding:9px 0;border-bottom:1px dashed var(--line);margin:0;font-size:13.5px;word-break:break-word}
  .genelist{display:flex;flex-wrap:wrap;gap:6px;margin:0;padding:0;list-style:none}
  .genelist li{padding:3px 9px;border:1px solid var(--line);border-radius:6px;
    font-family:ui-monospace,Menlo,Consolas,monospace;font-size:12px;background:#fbfcfe}
  .bars{display:grid;gap:10px;margin:0 0 4px}
  .bar{display:grid;grid-template-columns:minmax(180px,42%) 1fr 76px;align-items:center;gap:12px;font-size:12.5px}
  .bar .label{color:#25304a;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
  .bar .track{height:8px;border-radius:999px;background:#eef1f6;overflow:hidden}
  .bar .fill{display:block;height:100%;border-radius:999px;background:linear-gradient(90deg,#4f46e5,#6d63f0)}
  .bar .fill.alt{background:linear-gradient(90deg,#0f9d8f,#31b7a8)}
  .bar .val{color:var(--muted);text-align:right;font-family:ui-monospace,Menlo,Consolas,monospace;font-size:11.5px}
  table{width:100%;border-collapse:collapse;font-size:12.5px}
  thead th{text-align:left;padding:8px 10px;border-bottom:1px solid var(--line);color:var(--muted);
    font-weight:600;font-size:11.5px;letter-spacing:.04em;text-transform:uppercase;background:#fafbfd}
  tbody td{padding:9px 10px;border-bottom:1px solid #eef1f6;vertical-align:top}
  tbody tr:nth-child(even) td{background:#fcfdff}
  .term-name{font-weight:600;color:#1b2540}
  .term-id{color:var(--muted);font-family:ui-monospace,Menlo,Consolas,monospace;font-size:11px}
  .src{display:inline-block;padding:1px 7px;border-radius:5px;background:#eef1fb;color:#3f3bb0;
    font-size:10.5px;letter-spacing:.02em}
  .pval{font-family:ui-monospace,Menlo,Consolas,monospace;color:#12356b}
  .limits{margin:0;padding-left:18px;font-size:13px;color:#3c465c}
  .limits li{margin:6px 0}
  .cite{background:#f7f9fc;border-left:3px solid var(--teal);padding:12px 16px;border-radius:0 10px 10px 0;
    font-size:12.5px;color:#37415a;line-height:1.9}
  .cite code{font-size:11.5px;word-break:break-all}
  footer{max-width:1024px;margin:0 auto;padding:0 24px 40px;color:var(--muted);font-size:11.5px;
    display:flex;justify-content:space-between;gap:16px;flex-wrap:wrap}
  .demo{border:1px solid #f3d9a4;background:#fdf6e7;color:#7a4f10;border-radius:12px;
    padding:12px 16px;font-size:13px;margin-bottom:18px}
  @media (max-width:760px){
    .kpis{grid-template-columns:repeat(2,1fr)}
    dl.grid{grid-template-columns:120px 1fr}
    .bar{grid-template-columns:1fr;gap:4px}
    .bar .val{text-align:left}
    main{padding:24px 16px 40px}
    .masthead{padding:16px 20px}
    h1{font-size:23px}
  }
  @media print{
    body{background:#fff}
    .masthead{background:#fff;color:var(--ink);border-bottom:2px solid var(--navy);padding:0 0 12px}
    .masthead .meta,.brand span{color:var(--muted)}
    .card{break-inside:avoid;border-color:#d8dde7}
    footer{color:#666}
  }
</style>
</head>
<body>
<header class="masthead">
  <div class="brand">
    <span class="mark">G</span>
    <div><strong>GenePath Runner</strong><span>Enrichment Run Record</span></div>
  </div>
  <div class="meta">{{.Created}} 提交<br>{{.Status}}</div>
</header>

<main>
  <p class="eyebrow">Enrichment Report</p>
  <h1>{{.Label}}</h1>
  <div class="chips">
    <span class="chip">{{.Organism}}</span>
    <span class="chip mono">{{.RunID}}</span>
    <span class="chip ok">显著条目 {{.ResultCount}}</span>
    {{if .IsDemo}}<span class="chip warn">演示数据 · 非真实分析</span>{{end}}
  </div>

  {{if .IsDemo}}
  <div class="demo">该记录由 GenePath 内置样例数据集生成，仅用于界面演示，不代表真实的富集分析结果。</div>
  {{end}}

  <dl class="kpis">
    <div class="kpi"><dt>输入基因</dt><dd>{{.GeneCount}}<small>个符号</small></dd></div>
    <div class="kpi"><dt>上游可映射</dt><dd>{{.Mapped}}<small>个</small></dd></div>
    <div class="kpi"><dt>显著富集条目</dt><dd>{{.ResultCount}}<small>条</small></dd></div>
    <div class="kpi"><dt>总耗时</dt><dd>{{.Duration}}</dd></div>
  </dl>

  <section class="card">
    <h2>运行信息</h2>
    <p class="hint">以下字段与数据库中的运行记录一一对应，可用于日后复核。</p>
    <dl class="grid">
      <dt>运行 ID</dt><dd class="mono">{{.RunID}}</dd>
      <dt>状态</dt><dd>{{.Status}}{{if .Outcome}} · 结论 {{.Outcome}}{{end}}</dd>
      <dt>物种</dt><dd>{{.Organism}} ({{.OrganismRaw}})</dd>
      <dt>输入哈希 (sha256)</dt><dd class="mono">{{.InputHash}}</dd>
      <dt>提交时间</dt><dd>{{.Created}}</dd>
      <dt>完成时间</dt><dd>{{if .Finished}}{{.Finished}}{{else}}—{{end}}</dd>
      <dt>上游 HTTP 状态</dt><dd>{{if .HTTPStatus}}{{.HTTPStatus}}{{else}}—{{end}}</dd>
      <dt>数据源版本</dt><dd class="mono">{{.DataVersion}}</dd>
    </dl>
  </section>

  <section class="card">
    <h2>分析参数</h2>
    <p class="hint">参数按提交时的值原样记录，修改参数需要新建运行。</p>
    <dl class="grid">
      <dt>显著性阈值</dt><dd>{{.Params.Significance}}</dd>
      <dt>多重检验校正</dt><dd>{{.Params.Correction}}</dd>
      <dt>背景基因集</dt><dd>{{.Params.Background}}</dd>
      <dt>注释来源</dt><dd>{{join .Params.Sources ", "}}</dd>
    </dl>
  </section>

  <section class="card">
    <h2>输入基因与映射</h2>
    <p class="hint">{{.MappingText}}</p>
    <ul class="genelist">{{range .InputGenes}}<li>{{.}}</li>{{end}}</ul>
    {{if .Unmapped}}
    <p class="hint spaced">未映射到注释库的符号（上游返回，未做任何改写）：</p>
    <ul class="genelist">{{range .Unmapped}}<li>{{.}}</li>{{end}}</ul>
    {{end}}
    {{if .Duplicates}}
    <p class="hint spaced">上游识别出的重复符号：</p>
    <ul class="genelist">{{range .Duplicates}}<li>{{.}}</li>{{end}}</ul>
    {{end}}
  </section>

  {{if .HasTerms}}
  <section class="card">
    <h2>富集强度（前 10 条）</h2>
    <p class="hint">横条长度与 -log10(p) 成正比，仅用于阅读排序，数值以表格为准。</p>
    <div class="bars">
      {{range .Bars}}
      <div class="bar">
        <span class="label" title="{{.Name}}">{{.Name}}</span>
        <span class="track"><span class="fill{{if .Alt}} alt{{end}}" style="width:{{.Width}}%"></span></span>
        <span class="val">{{.PValue}}</span>
      </div>
      {{end}}
    </div>
  </section>

  <section class="card">
    <h2>富集结果明细</h2>
    <p class="hint">全部数值来自 g:Profiler 的返回值，GenePath 未重新计算任何 p 值。</p>
    <table>
      <thead>
        <tr><th style="width:36px">#</th><th>Term</th><th style="width:74px">来源</th><th style="width:84px">p 值</th><th style="width:88px">命中/查询</th></tr>
      </thead>
      <tbody>
        {{range .Terms}}
        <tr>
          <td>{{.Rank}}</td>
          <td><div class="term-name">{{.Name}}</div><div class="term-id">{{.TermID}}</div></td>
          <td><span class="src">{{.Source}}</span></td>
          <td class="pval">{{.PValue}}</td>
          <td>{{.Hits}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </section>
  {{else}}
  <section class="card">
    <h2>富集结果</h2>
    <p class="hint">本次运行没有达到显著性阈值的条目。这表示在当前阈值与注释来源下未发现显著富集，并不等于“没有生物学功能”。</p>
  </section>
  {{end}}

  <section class="card">
    <h2>科学局限</h2>
    <ul class="limits">
      <li>富集分析只反映“输入基因集”与“注释条目”的重叠程度，不能证明因果关系或通路被激活。</li>
      <li>结果高度依赖注释库版本（本次为 {{.DataVersion}}）、背景基因集与物种注释覆盖度。</li>
      <li>未映射的基因不参与统计，可能低估真实信号，请在实验设计层面确认符号正确。</li>
      <li>g:Profiler 数据源会随时间更新，重新执行同一基因集不保证得到完全相同的数值。</li>
      <li>本报告是历史存档的回看结果；重新执行会生成新的运行记录，两者应分别引用。</li>
    </ul>
  </section>

  <section class="card">
    <h2>数据来源与引用</h2>
    <div class="cite">
      Enrichment statistics: g:Profiler — Raudvere et al., “g:Profiler: a web server for functional enrichment analysis and conversions of gene lists”, Nucleic Acids Research, 2019.<br>
      接口端点：<code>{{.Endpoint}}</code><br>
      数据源版本：<code>{{.DataVersion}}</code><br>
      生成时间：{{.Generated}}
    </div>
  </section>
</main>

<footer>
  <span>GenePath Runner · 运行记录与报告导出</span>
  <span>报告由数据库中的运行记录生成，统计值未经二次计算。</span>
</footer>
</body>
</html>
`
