/* GenePath Runner — run history page. */
(function () {
  "use strict";

  const ui = window.GP.ui;
  const api = window.GP.api;
  const $ = (id) => document.getElementById(id);

  const PAGE_SIZE = 12;

  const state = {
    status: "",
    q: "",
    organism: "",
    offset: 0,
    total: 0,
  };

  const params = new URLSearchParams(window.location.search);
  state.status = params.get("status") || "";
  state.q = params.get("q") || "";

  function syncUrl() {
    const next = new URLSearchParams();
    if (state.status) next.set("status", state.status);
    if (state.q) next.set("q", state.q);
    const qs = next.toString();
    window.history.replaceState(null, "", "/history" + (qs ? "?" + qs : ""));
  }

  function emptyRow(title, message, action) {
    return (
      '<tr><td colspan="10"><div class="empty"><div class="empty__icon">' +
      ui.icon("flask", 21) +
      "</div><h4>" +
      ui.escapeHtml(title) +
      "</h4><p>" +
      ui.escapeHtml(message) +
      "</p>" +
      (action || "") +
      "</div></td></tr>"
    );
  }

  function renderRows(items) {
    const body = $("historyBody");
    if (!items.length) {
      body.innerHTML = emptyRow(
        "没有匹配的运行记录",
        "调整筛选条件，或提交一次新的分析。",
        '<a class="btn btn--primary btn--sm" href="/new">新建分析</a>'
      );
      return;
    }

    body.innerHTML = items
      .map(function (run) {
        return (
          '<tr class="is-clickable" data-href="/runs/' +
          encodeURIComponent(run.id) +
          '">' +
          '<td><div class="table__row-title"><span class="cell-primary truncate">' +
          ui.escapeHtml(run.label) +
          "</span><span class=\"mono\">" +
          ui.escapeHtml(run.id) +
          "</span></div></td>" +
          "<td>" +
          ui.statusBadge(run.status) +
          (run.isDemo ? ' <span class="badge badge--demo">演示</span>' : "") +
          "</td>" +
          "<td>" +
          ui.escapeHtml(ui.organismLabel(run.organism)) +
          "</td>" +
          '<td class="cell-num numeric">' +
          ui.fmtNumber(run.geneCount) +
          "</td>" +
          '<td class="cell-num numeric">' +
          (run.resultCount ? ui.fmtNumber(run.resultCount) : "—") +
          "</td>" +
          '<td class="numeric">' +
          ui.fmtDuration(run.durationMs) +
          "</td>" +
          '<td class="hash-cell truncate">' +
          ui.escapeHtml(run.dataVersion || "—") +
          "</td>" +
          '<td><span class="hash-cell" title="' +
          ui.escapeHtml(run.inputHash) +
          '">' +
          ui.shortHash(run.inputHash, 12) +
          "</span></td>" +
          '<td class="numeric muted" title="' +
          ui.escapeHtml(ui.fmtDateTime(run.createdAt)) +
          '">' +
          ui.fmtRelative(run.createdAt) +
          "</td>" +
          '<td><div class="row-actions">' +
          (run.status === "succeeded"
            ? '<a class="icon-btn" href="' +
              api.reportUrl(run.id) +
              '" title="下载报告" aria-label="下载报告" data-download>' +
              ui.icon("download", 15) +
              "</a>"
            : "") +
          '<a class="icon-btn" href="/runs/' +
          encodeURIComponent(run.id) +
          '" title="查看详情" aria-label="查看详情">' +
          ui.icon("chevronRight", 15) +
          "</a></div></td></tr>"
        );
      })
      .join("");

    Array.prototype.slice.call(body.querySelectorAll("tr[data-href]")).forEach(function (tr) {
      tr.addEventListener("click", function (e) {
        if (e.target.closest("a")) return;
        window.location.href = tr.getAttribute("data-href");
      });
    });
  }

  function renderPagination() {
    const box = $("pagination");
    if (state.total <= PAGE_SIZE) {
      box.hidden = true;
      return;
    }
    box.hidden = false;
    const from = state.offset + 1;
    const to = Math.min(state.offset + PAGE_SIZE, state.total);
    $("pageInfo").textContent =
      "第 " + from + "–" + to + " 条，共 " + ui.fmtNumber(state.total) + " 条";
    $("prevPage").disabled = state.offset === 0;
    $("nextPage").disabled = state.offset + PAGE_SIZE >= state.total;
  }

  async function load() {
    $("resultCount").textContent = "加载中…";
    try {
      const page = await api.listRuns({
        status: state.status,
        q: state.q,
        organism: state.organism,
        limit: PAGE_SIZE,
        offset: state.offset,
      });
      state.total = page.total;
      renderRows(page.items);
      renderPagination();
      $("resultCount").textContent =
        "共 " + ui.fmtNumber(page.total) + " 条记录";
      ui.syncNavCount();
    } catch (err) {
      $("historyBody").innerHTML = emptyRow(
        "无法加载运行记录",
        err.message,
        '<button class="btn btn--secondary btn--sm" id="retryInline">重试</button>'
      );
      $("resultCount").textContent = "加载失败";
      const retry = $("retryInline");
      if (retry) retry.addEventListener("click", load);
      ui.toast("error", "加载失败", err.message);
    }
  }

  function setStatusFilter(status) {
    state.status = status;
    state.offset = 0;
    syncUrl();
    Array.prototype.slice
      .call($("statusFilter").querySelectorAll("button"))
      .forEach(function (btn) {
        btn.classList.toggle(
          "is-active",
          btn.getAttribute("data-status") === status
        );
      });
    load();
  }

  function bind() {
    Array.prototype.slice
      .call($("statusFilter").querySelectorAll("button"))
      .forEach(function (btn) {
        btn.classList.toggle(
          "is-active",
          btn.getAttribute("data-status") === state.status
        );
        btn.addEventListener("click", function () {
          setStatusFilter(btn.getAttribute("data-status"));
        });
      });

    $("searchInput").value = state.q;
    let searchTimer = null;
    $("searchInput").addEventListener("input", function (e) {
      window.clearTimeout(searchTimer);
      const value = e.target.value.trim();
      searchTimer = window.setTimeout(function () {
        state.q = value;
        state.offset = 0;
        syncUrl();
        load();
      }, 260);
    });

    $("organismFilter").addEventListener("change", function (e) {
      state.organism = e.target.value;
      state.offset = 0;
      load();
    });

    $("refreshBtn").addEventListener("click", function () {
      ui.toast("info", "正在刷新", "重新读取运行记录");
      load();
    });

    $("prevPage").addEventListener("click", function () {
      state.offset = Math.max(0, state.offset - PAGE_SIZE);
      load();
    });
    $("nextPage").addEventListener("click", function () {
      state.offset += PAGE_SIZE;
      load();
    });
  }

  async function loadOrganisms() {
    try {
      const meta = await api.meta();
      const select = $("organismFilter");
      meta.organisms.forEach(function (o) {
        const opt = document.createElement("option");
        opt.value = o.code;
        opt.textContent = o.chinese + " · " + o.label;
        select.appendChild(opt);
      });
    } catch (err) {
      /* the filter degrades to "all organisms" */
    }
  }

  bind();
  loadOrganisms();
  load();
})();
