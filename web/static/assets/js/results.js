/* GenePath Runner — analysis result page. */
(function () {
  "use strict";

  const ui = window.GP.ui;
  const api = window.GP.api;
  const $ = (id) => document.getElementById(id);

  const runId = decodeURIComponent(
    window.location.pathname.replace(/^\/runs\//, "").replace(/\/$/, "")
  );

  const state = { source: "ALL", q: "", sort: "p-asc" };
  let terms = [];
  let run = null;
  let data = null;
  let pollTimer = null;
  let polls = 0;

  const KIND_LABEL = {
    submitted: "已提交",
    queued: "进入队列",
    started: "开始执行",
    provider_call: "调用上游",
    mapping: "基因映射",
    completed: "已完成",
    failed: "失败",
    canceled: "已取消",
    interrupted: "已中断",
  };

  // A run is finished once the worker has stored a terminal state.
  function isActive() {
    return run && (run.status === "queued" || run.status === "running");
  }

  function sourceClass(source) {
    if (source === "KEGG") return "term-chart__fill--KEGG";
    if (source === "REAC") return "term-chart__fill--REAC";
    if (source.indexOf("GO:CC") === 0) return "term-chart__fill--GOCC";
    return "";
  }

  function metaPill(icon, label) {
    return '<span class="meta-pill">' + ui.icon(icon, 13) + label + "</span>";
  }

  function renderHead() {
    $("runTitle").textContent = run.label || "未命名分析";
    $("crumbId").textContent = run.id;
    $("runStatus").innerHTML = ui.statusBadge(run.status);
    $("runDemo").hidden = !run.isDemo;
    document.title = (run.label || "分析结果") + " · GenePath Runner";

    let duration = "未运行";
    if (run.status === "running") duration = "运行中…";
    else if (run.durationMs) duration = ui.fmtDuration(run.durationMs);

    const parts = [
      metaPill("flask", ui.escapeHtml(run.id)),
      metaPill("gene", ui.escapeHtml(ui.organismLabel(run.organism)) + " · " + run.geneCount + " 个基因"),
      metaPill("clock", duration),
      metaPill("database", ui.escapeHtml(run.dataVersion || "数据集版本未记录")),
      metaPill("target", ui.fmtDateTime(run.createdAt)),
    ];
    if (run.httpStatus) {
      parts.push(metaPill("external", "上游 HTTP " + run.httpStatus));
    }
    if (run.isDemo) {
      parts.push(
        '<span class="meta-pill">' + ui.icon("info", 13) + "示例结果，非真实分析</span>"
      );
    }
    $("runMeta").innerHTML = parts.join("");

    const download = $("downloadBtn");
    download.href = api.reportUrl(run.id);
    download.hidden = false;
  }

  function renderStats() {
    const list = terms;
    $("statTerms").textContent = ui.fmtNumber(list.length);
    if (!list.length) {
      $("statBestP").textContent = "—";
      $("statMaxHit").textContent = "—";
      return;
    }
    const best = list.reduce((a, b) => (a.pValue <= b.pValue ? a : b));
    $("statBestP").textContent = ui.fmtP(best.pValue);
    const maxHit = list.reduce((a, b) => Math.max(a, b.intersection), 0);
    $("statMaxHit").textContent = maxHit + " 个";
  }

  function spinnerBlock(title, sub) {
    return (
      '<div class="empty empty--loading"><div class="spinner" aria-hidden="true"></div>' +
      "<h4>" + ui.escapeHtml(title) + "</h4><p>" + ui.escapeHtml(sub) + "</p></div>"
    );
  }

  function failedBlock() {
    return (
      '<div class="empty"><div class="empty__icon">' + ui.icon("alert", 21) + "</div>" +
      "<h4>这次分析没有完成</h4><p>" + ui.escapeHtml(run.errorMessage || "上游没有返回可用结果。") + "</p>" +
      '<p class="text-xs muted">错误码：' + ui.escapeHtml(run.errorCode || "unknown") + "</p>" +
      '<a class="btn btn--secondary btn--sm" href="/new">重新提交</a></div>'
    );
  }

  function renderChart() {
    const box = $("termChart");
    if (!terms.length) {
      if (isActive()) {
        box.innerHTML = spinnerBlock(
          run.status === "queued" ? "排队中，等待 worker 接管" : "正在调用 g:Profiler…",
          "结果写入数据库后本页会自动刷新。可以离开页面，任务会继续执行。"
        );
        return;
      }
      if (run.status === "failed" || run.status === "interrupted") {
        box.innerHTML = failedBlock();
        return;
      }
      slotEmpty(box);
      return;
    }

    const top = terms.slice().sort((a, b) => a.pValue - b.pValue).slice(0, 10);
    const negLogs = top.map((t) => -Math.log10(Math.max(t.pValue, 1e-300)));
    const max = Math.max.apply(null, negLogs);

    box.innerHTML = top
      .map(function (t, i) {
        const pct = Math.max(4, (negLogs[i] / max) * 100);
        return (
          '<div class="term-chart__row">' +
          '<div class="term-chart__item">' +
          '<div class="term-chart__label">' +
          '<span class="term-chart__name" title="' +
          ui.escapeHtml(t.name) +
          '">' +
          ui.escapeHtml(t.name) +
          "</span>" +
          '<span class="badge badge--source">' +
          ui.escapeHtml(t.source) +
          "</span></div>" +
          '<div class="term-chart__track"><div class="term-chart__fill ' +
          sourceClass(t.source) +
          '" data-w="' +
          pct +
          '"></div></div></div>' +
          '<div class="term-chart__value">' +
          ui.fmtP(t.pValue) +
          "</div></div>"
        );
      })
      .join("");

    Array.prototype.slice
      .call(box.querySelectorAll(".term-chart__fill"))
      .forEach(function (el) {
        el.style.width = el.getAttribute("data-w") + "%";
      });
  }

  function slotEmpty(box) {
    const mapped = run.mappedCount === 0;
    box.innerHTML =
      '<div class="empty"><div class="empty__icon">' + ui.icon("chart", 21) + "</div>" +
      "<h4>" +
      (mapped ? "没有基因被映射到注释库" : "没有条目通过显著性阈值") +
      "</h4><p>" +
      (mapped
        ? "上游没有识别这次输入中的任何基因符号，请检查物种是否选对、符号是否为官方 HGNC/ENSG 名称。"
        : "分析确实执行了，但这批基因在所选注释来源下没有达到阈值的条目。可以放宽阈值或增加注释来源后重试。") +
      "</p></div>";
  }

  function renderSourceChips() {
    const box = $("sourceChips");
    const counts = {};
    terms.forEach(function (t) {
      counts[t.source] = (counts[t.source] || 0) + 1;
    });
    const keys = Object.keys(counts).sort();

    box.innerHTML = "";
    const all = document.createElement("button");
    all.type = "button";
    all.className = "chip" + (state.source === "ALL" ? " chip--active" : "");
    all.textContent = "全部 " + terms.length;
    all.addEventListener("click", function () {
      state.source = "ALL";
      renderSourceChips();
      renderTable();
    });
    box.appendChild(all);

    keys.forEach(function (key) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "chip" + (state.source === key ? " chip--active" : "");
      btn.innerHTML =
        ui.escapeHtml(key) + ' <span class="chip__count">' + counts[key] + "</span>";
      btn.addEventListener("click", function () {
        state.source = key;
        renderSourceChips();
        renderTable();
      });
      box.appendChild(btn);
    });
  }

  function visibleTerms() {
    let list = terms.slice();
    if (state.source !== "ALL") {
      list = list.filter((t) => t.source === state.source);
    }
    if (state.q) {
      const q = state.q.toLowerCase();
      list = list.filter(function (t) {
        return (
          t.name.toLowerCase().indexOf(q) >= 0 ||
          t.termId.toLowerCase().indexOf(q) >= 0 ||
          t.genes.join(" ").toLowerCase().indexOf(q) >= 0
        );
      });
    }
    if (state.sort === "p-asc") list.sort((a, b) => a.pValue - b.pValue);
    if (state.sort === "p-desc") list.sort((a, b) => b.pValue - a.pValue);
    if (state.sort === "hits-desc") list.sort((a, b) => b.intersection - a.intersection);
    if (state.sort === "name-asc") list.sort((a, b) => a.name.localeCompare(b.name));
    return list;
  }

  function renderTable() {
    const body = $("termTableBody");
    const list = visibleTerms();

    $("termCount").textContent = list.length + " / " + terms.length + " 条";

    if (!terms.length) {
      let msg = "这次运行没有可以展示的显著条目。";
      if (isActive()) msg = "任务仍在执行，结果写入后会自动出现在这里。";
      if (run.status === "failed") msg = "运行失败：" + (run.errorMessage || "上游没有返回结果。");
      body.innerHTML =
        '<tr><td colspan="6"><div class="empty"><div class="empty__icon">' +
        ui.icon("database", 21) +
        "</div><h4>没有已存储的富集条目</h4><p>" +
        ui.escapeHtml(msg) +
        "</p></div></td></tr>";
      return;
    }

    if (!list.length) {
      body.innerHTML =
        '<tr><td colspan="6"><div class="empty"><p>当前筛选条件下没有匹配的条目。</p></div></td></tr>';
      return;
    }

    body.innerHTML = list
      .map(function (t, i) {
        const genes = t.genes || [];
        const shown = genes.slice(0, 6);
        const more = genes.length - shown.length;
        const tags =
          shown
            .map(function (g) {
              return '<span class="gene-tag">' + ui.escapeHtml(g) + "</span>";
            })
            .join("") +
          (more > 0
            ? '<span class="gene-tag gene-tag--more">+' + more + "</span>"
            : "");

        return (
          "<tr>" +
          '<td class="numeric muted">' +
          (i + 1) +
          "</td>" +
          '<td><div class="term-name"><strong>' +
          ui.escapeHtml(t.name) +
          "</strong><code>" +
          ui.escapeHtml(t.termId) +
          "</code></div></td>" +
          '<td><span class="badge badge--source">' +
          ui.escapeHtml(t.source) +
          "</span></td>" +
          '<td class="cell-num"><span class="pvalue' +
          (t.pValue < 1e-6 ? " pvalue--strong" : "") +
          '">' +
          ui.fmtP(t.pValue) +
          "</span></td>" +
          "<td>" +
          '<div class="hit-bar"><div class="hit-bar__track"><div class="hit-bar__fill" data-w="' +
          Math.round((t.intersection / Math.max(1, t.querySize)) * 100) +
          '"></div></div><span>' +
          t.intersection +
          " / " +
          t.querySize +
          "</span></div></td>" +
          '<td><div class="gene-tags">' +
          (tags || '<span class="muted text-xs">未返回</span>') +
          "</div></td></tr>"
        );
      })
      .join("");

    Array.prototype.slice
      .call(body.querySelectorAll(".hit-bar__fill"))
      .forEach(function (el) {
        el.style.width = Math.max(3, Number(el.getAttribute("data-w"))) + "%";
      });
  }

  function renderAside() {
    const params = run.params || {};
    $("paramList").innerHTML =
      row("显著性阈值", params.significance) +
      row("多重检验校正", params.correction) +
      row("背景基因集", params.background) +
      row("注释来源", (params.sources || []).join(", ") || "—");

    $("traceList").innerHTML =
      row("运行 ID", run.id, true) +
      row("数据源版本", run.dataVersion || "—") +
      row("上游服务", "g:Profiler REST") +
      row("上游状态码", run.httpStatus ? "HTTP " + run.httpStatus : "—") +
      row("上游耗时", run.providerMs ? run.providerMs + " ms" : "—") +
      row(
        "请求 / 响应体积",
        run.requestBytes || run.responseBytes
          ? ui.fmtBytes(run.requestBytes) + " / " + ui.fmtBytes(run.responseBytes)
          : "—"
      ) +
      row("开始时间", ui.fmtDateTime(run.startedAt)) +
      row("结束时间", ui.fmtDateTime(run.finishedAt)) +
      row("总耗时", ui.fmtDuration(run.durationMs));

    const rawLinks = $("rawLinks");
    const rawBtn = $("rawBtn");
    if (rawLinks && rawBtn) {
      const hasRaw = run.requestBytes > 0 || run.responseBytes > 0;
      rawLinks.hidden = !hasRaw;
      rawBtn.href = api.rawUrl(run.id);
    }

    $("traceHash").textContent = run.inputHash || "—";
    $("copyHash").setAttribute("data-copy", run.inputHash || "");

    const timeline = $("eventTimeline");
    if (!run.events || !run.events.length) {
      timeline.innerHTML = '<p class="text-sm muted">没有记录到事件。</p>';
    } else {
      timeline.innerHTML = run.events
        .map(function (e) {
          let cls = "";
          if (e.kind === "completed") cls = " timeline__item--success";
          if (e.kind === "failed" || e.kind === "interrupted" || e.kind === "canceled")
            cls = " timeline__item--error";
          return (
            '<div class="timeline__item' +
            cls +
            '"><div class="timeline__kind">' +
            ui.escapeHtml(KIND_LABEL[e.kind] || e.kind) +
            '</div><div class="timeline__msg">' +
            ui.escapeHtml(e.message) +
            '</div><div class="timeline__time">' +
            ui.fmtDateTime(e.at) +
            "</div></div>"
          );
        })
        .join("");
    }

    const genes = (data && data.genes) || run.genes || [];
    $("geneCountLabel").textContent = "共 " + genes.length + " 个符号";
    $("geneList").innerHTML = genes
      .slice(0, 60)
      .map(function (g) {
        return '<span class="gene-tag">' + ui.escapeHtml(g) + "</span>";
      })
      .join("") +
      (genes.length > 60
        ? '<span class="gene-tag gene-tag--more">+' + (genes.length - 60) + "</span>"
        : "");
  }

  function row(key, value, mono) {
    return (
      '<div class="summary-list__row"><dt>' +
      ui.escapeHtml(key) +
      "</dt><dd" +
      (mono ? ' class="mono"' : "") +
      ">" +
      ui.escapeHtml(value || "—") +
      "</dd></div>"
    );
  }

  function renderError(err) {
    $("runTitle").textContent =
      err.status === 404 ? "任务不存在" : "加载失败";
    $("pageError").innerHTML =
      '<div class="card"><div class="empty"><div class="empty__icon">' +
      ui.icon("alert", 21) +
      "</div><h4>" +
      (err.status === 404 ? "找不到这条运行记录" : "无法加载运行详情") +
      "</h4><p>" +
      ui.escapeHtml(err.message) +
      '</p><a class="btn btn--secondary btn--sm" href="/history">返回运行历史</a></div></div>';
    $("resultGrid").hidden = true;
    document.querySelector(".stat-strip").hidden = true;
    ui.toast("error", "加载失败", err.message);
  }

  function bind() {
    $("termSearch").addEventListener("input", function (e) {
      state.q = e.target.value.trim();
      renderTable();
    });
    $("sortSelect").addEventListener("change", function (e) {
      state.sort = e.target.value;
      renderTable();
    });
  }

  async function load() {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
    try {
      data = await api.results(runId);
      run = data.run;
      run.events = data.events || [];
      terms = (data.terms || []).slice().sort((a, b) => a.pValue - b.pValue);
      renderHead();
      renderStats();
      renderChart();
      renderSourceChips();
      renderTable();
      renderAside();
      $("navRunCount").textContent = "—";
      if (isActive() && polls < 240) {
        polls += 1;
        pollTimer = setTimeout(load, 1600);
      }
    } catch (err) {
      renderError(err);
    }
  }

  bind();
  load();
})();
