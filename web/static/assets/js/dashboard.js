/* GenePath Runner — dashboard page. */
(function () {
  "use strict";

  const ui = window.GP.ui;
  const api = window.GP.api;

  const STATUS_COLORS = {
    succeeded: "#10a3a0",
    failed: "#c25a4e",
    interrupted: "#b9c1d2",
    canceled: "#d5dae4",
    running: "#3150dd",
    queued: "#e0a34a",
  };

  const $ = (id) => document.getElementById(id);

  function greeting() {
    const h = new Date().getHours();
    if (h < 6) return "夜深了，注意休息";
    if (h < 12) return "早上好，欢迎回到工作台";
    if (h < 18) return "下午好，继续你的分析";
    return "晚上好，看看今天的进展";
  }

  function renderStats(data) {
    const s = data.stats;

    $("kpiTotal").textContent = ui.fmtNumber(s.total);
    const latest = data.recentRuns && data.recentRuns[0];
    $("kpiTotalFoot").innerHTML = latest
      ? "<span>最近一次提交 <strong>" + ui.escapeHtml(ui.fmtRelative(latest.createdAt)) + "</strong></span>"
      : "<span>还没有提交过分析</span>";

    $("kpiSuccess").textContent = ui.fmtNumber(s.succeeded);
    $("kpiSuccessFoot").innerHTML = s.total
      ? "<span>成功率 <strong>" +
        (data.successRate * 100).toFixed(0) +
        "%</strong></span>"
      : "<span>还没有可统计的运行</span>";

    $("kpiFailed").textContent = ui.fmtNumber(s.failed);
    const others =
      data.statuses
        .filter((x) => x.status === "canceled" || x.status === "interrupted")
        .reduce((a, b) => a + b.count, 0) + s.failed;
    $("kpiFailedFoot").innerHTML =
      "<span>失败 + 中断/取消共 <strong>" + ui.fmtNumber(others) + "</strong> 次</span>";

    if (data.avgDurationMs) {
      const seconds = data.avgDurationMs >= 1000;
      $("kpiDuration").innerHTML =
        ui.escapeHtml(
          ui.fmtDuration(data.avgDurationMs).replace(seconds ? " s" : " ms", "")
        ) + "<small>" + (seconds ? "s" : "ms") + "</small>";
      $("kpiDurationFoot").innerHTML = "<span>仅统计成功任务</span>";
    } else {
      $("kpiDuration").textContent = "—";
      $("kpiDurationFoot").innerHTML = "<span>暂无成功任务可供统计</span>";
    }

    if (data.demoDataPresent) {
      $("demoChip").hidden = false;
      const clear = $("clearDemoBtn");
      if (clear) {
        clear.hidden = false;
        clear.addEventListener("click", async function () {
          if (!window.confirm("删除所有标记为演示的运行记录？真实分析记录不受影响。")) return;
          clear.disabled = true;
          try {
            const res = await api.clearDemo();
            ui.toast("success", "演示数据已清除", "删除了 " + res.deleted + " 条记录");
            window.location.reload();
          } catch (err) {
            clear.disabled = false;
            ui.toast("error", "清除失败", err.message);
          }
        });
      }
    }
    $("navRunCount").textContent = ui.fmtNumber(s.total);
  }

  function renderActivity(activity) {
    const chart = $("activityChart");
    const axis = $("activityAxis");
    chart.innerHTML = "";
    axis.innerHTML = "";

    const values = activity.map((d) => d.count);
    const max = Math.max(1, ...values);
    const peak = Math.max(...values);
    $("activityPeak").textContent = "峰值 " + peak + " 次/天";

    const todayKey = new Date().toISOString().slice(0, 10);

    activity.forEach(function (day, idx) {
      const wrap = document.createElement("div");
      wrap.className = "activity__bar";
      if (day.count === 0) wrap.classList.add("activity__bar--empty");
      if (day.day === todayKey || idx === activity.length - 1) {
        wrap.classList.add("activity__bar--today");
      }
      wrap.title = day.day + " · " + day.count + " 次运行";

      const fill = document.createElement("div");
      fill.className = "activity__fill";
      const pct = max === 0 ? 0 : (day.count / max) * 100;
      fill.style.height = Math.max(day.count === 0 ? 2 : 6, pct) + "%";
      wrap.appendChild(fill);
      chart.appendChild(wrap);

      const label = document.createElement("span");
      if (idx % 2 === 0 || idx === activity.length - 1) {
        label.textContent = day.day.slice(5).replace("-", "/");
      } else {
        label.textContent = "";
      }
      axis.appendChild(label);
    });
  }

  function renderRecent(runs) {
    const tbody = $("recentRuns");
    tbody.innerHTML = "";

    if (!runs.length) {
      const tr = document.createElement("tr");
      const td = document.createElement("td");
      td.colSpan = 6;
      td.innerHTML =
        '<div class="empty"><div class="empty__icon">' +
        ui.icon("flask", 21) +
        "</div><h4>还没有运行记录</h4><p>提交第一组基因后，运行记录会出现在这里。</p>" +
        '<a class="btn btn--primary btn--sm" href="/new">新建分析</a></div>';
      tr.appendChild(td);
      tbody.appendChild(tr);
      return;
    }

    runs.forEach(function (run) {
      const tr = document.createElement("tr");
      tr.className = "is-clickable";
      tr.addEventListener("click", function () {
        window.location.href = "/runs/" + encodeURIComponent(run.id);
      });
      tr.innerHTML =
        '<td><div class="table__row-title"><span class="cell-primary truncate">' +
        ui.escapeHtml(run.label) +
        "</span><span>" +
        ui.escapeHtml(ui.organismLabel(run.organism)) +
        " · " +
        ui.escapeHtml(run.id.slice(0, 12)) +
        "</span></div></td>" +
        "<td>" +
        ui.statusBadge(run.status) +
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
        '<td class="numeric muted">' +
        ui.fmtRelative(run.createdAt) +
        "</td>";
      tbody.appendChild(tr);
    });
  }

  function renderDonut(data) {
    const svg = $("statusDonut");
    const total = data.stats.total;
    $("donutTotal").textContent = ui.fmtNumber(total);

    // Remove previous slices but keep the background ring.
    while (svg.childNodes.length > 1) {
      svg.removeChild(svg.lastChild);
    }

    const C = 2 * Math.PI * 48;
    let offset = 0;
    data.statuses.forEach(function (item) {
      if (!item.count) return;
      const len = (item.count / total) * C;
      const circle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      circle.setAttribute("cx", "60");
      circle.setAttribute("cy", "60");
      circle.setAttribute("r", "48");
      circle.setAttribute("fill", "none");
      circle.setAttribute("stroke", STATUS_COLORS[item.status] || "#c9cfdc");
      circle.setAttribute("stroke-width", "13");
      circle.setAttribute("stroke-dasharray", len + " " + (C - len));
      circle.setAttribute("stroke-dashoffset", String(-offset));
      circle.setAttribute("stroke-linecap", "butt");
      svg.appendChild(circle);
      offset += len;
    });

    const list = $("statusList");
    list.innerHTML = "";
    if (!total) {
      list.innerHTML =
        '<p class="text-sm muted">还没有运行记录，提交第一次分析后这里会出现状态分布。</p>';
      return;
    }
    data.statuses.forEach(function (item) {
      if (!item.count) return;
      const row = document.createElement("div");
      row.className = "status-list__row";

      const swatch = document.createElement("i");
      swatch.className = "swatch";
      swatch.style.background = STATUS_COLORS[item.status] || "#c9cfdc";

      const name = document.createElement("span");
      name.className = "name";
      name.textContent = ui.statusLabel(item.status);

      const count = document.createElement("span");
      count.className = "count";
      count.textContent = ui.fmtNumber(item.count);

      const pct = document.createElement("span");
      pct.className = "pct";
      pct.textContent = ((item.count / total) * 100).toFixed(0) + "%";

      row.appendChild(swatch);
      row.appendChild(name);
      row.appendChild(count);
      row.appendChild(pct);
      list.appendChild(row);
    });
  }

  function renderSources(sources) {
    const box = $("sourceList");
    box.innerHTML = "";
    if (!sources.length) {
      box.innerHTML =
        '<p class="text-sm muted">暂无已存储的显著条目。</p>';
      return;
    }
    const max = Math.max.apply(
      null,
      sources.map((s) => s.count)
    );
    sources.slice(0, 6).forEach(function (src) {
      const row = document.createElement("div");
      row.className = "source-row";

      const badge = document.createElement("span");
      badge.className = "badge badge--source";
      badge.textContent = src.source;

      const bar = document.createElement("div");
      bar.className = "source-row__bar";
      const fill = document.createElement("i");
      fill.style.width = Math.max(6, (src.count / max) * 100) + "%";
      bar.appendChild(fill);

      const count = document.createElement("span");
      count.className = "count";
      count.textContent = src.count + " 条";

      row.appendChild(badge);
      row.appendChild(bar);
      row.appendChild(count);
      box.appendChild(row);
    });
  }

  function renderProvider(provider) {
    $("footerEndpoint").textContent = provider.endpoint
      ? provider.endpoint.replace(/^https?:\/\//, "")
      : "未配置";
    $("sidebarVersion").textContent = "数据集 由上游实时返回";
  }

  function renderError(err) {
    const tbody = $("recentRuns");
    tbody.innerHTML =
      '<tr><td colspan="6"><div class="empty"><div class="empty__icon">' +
      ui.icon("alert", 21) +
      "</div><h4>数据加载失败</h4><p>" +
      ui.escapeHtml(err.message) +
      "</p><button class=\"btn btn--secondary btn--sm\" id=\"retryBtn\">重新加载</button></div></td></tr>";
    const retry = $("retryBtn");
    if (retry) retry.addEventListener("click", load);
    ui.toast("error", "无法加载工作台数据", err.message);
  }

  async function load() {
    try {
      const data = await api.dashboard();
      $("greeting").textContent = greeting();
      renderStats(data);
      renderActivity(data.activity);
      renderRecent(data.recentRuns);
      renderDonut(data);
      renderSources(data.sources);
      renderProvider(data.provider);
    } catch (err) {
      renderError(err);
    }
  }

  load();
})();
