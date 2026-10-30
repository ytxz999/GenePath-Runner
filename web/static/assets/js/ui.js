/* GenePath Runner — shared UI helpers: icons, formatting, toasts, shell. */

(function (global) {
  "use strict";

  const ICONS = {
    dashboard:
      '<rect x="3" y="3" width="7" height="8" rx="1.6"/><rect x="14" y="3" width="7" height="5" rx="1.6"/><rect x="14" y="12" width="7" height="9" rx="1.6"/><rect x="3" y="15" width="7" height="6" rx="1.6"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    history:
      '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/>',
    chart: '<path d="M4 20V10M10 20V4M16 20v-7M22 20H2"/>',
    database:
      '<ellipse cx="12" cy="5.5" rx="8" ry="3"/><path d="M4 5.5v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/><path d="M4 11.5v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    download: '<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M4 20h16"/>',
    upload: '<path d="M12 16V4"/><path d="m7 9 5-5 5 5"/><path d="M4 20h16"/>',
    arrowRight: '<path d="M5 12h14"/><path d="m13 6 6 6-6 6"/>',
    arrowLeft: '<path d="M19 12H5"/><path d="m11 18-6-6 6-6"/>',
    chevronRight: '<path d="m9 6 6 6-6 6"/>',
    chevronDown: '<path d="m6 9 6 6 6-6"/>',
    check: '<path d="m4 12.5 5 5L20 6.5"/>',
    alert: '<path d="M12 8v5"/><path d="M12 17h.01"/><circle cx="12" cy="12" r="9"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 8h.01"/>',
    close: '<path d="M6 6l12 12M18 6 6 18"/>',
    copy: '<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V6a2 2 0 0 1 2-2h8"/>',
    refresh:
      '<path d="M20 11a8 8 0 1 0-2.3 5.6"/><path d="M20 5v6h-6"/>',
    menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
    download2: '<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M4 20h16"/>',
    flask:
      '<path d="M9 3h6"/><path d="M10 3v6.2L5.2 17A2.6 2.6 0 0 0 7.4 21h9.2a2.6 2.6 0 0 0 2.2-4L14 9.2V3"/><path d="M7.5 15h9"/>',
    gene: '<path d="M4 4c6 4 10 12 16 16"/><path d="M20 4C14 8 10 16 4 20"/><path d="M8 5h8M8 19h8"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    target:
      '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1.4"/>',
    layers:
      '<path d="m12 3 9 5-9 5-9-5 9-5Z"/><path d="m3 13 9 5 9-5"/>',
    external: '<path d="M14 4h6v6"/><path d="M20 4 11 13"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
    trash: '<path d="M4 7h16"/><path d="M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/><path d="M6 7l1 13h10l1-13"/>',
    spark: '<path d="m12 3 2.2 6.2L20 12l-5.8 2.8L12 21l-2.2-6.2L4 12l5.8-2.8L12 3Z"/>',
  };

  function icon(name, size) {
    const path = ICONS[name] || "";
    const s = size || 16;
    return (
      '<svg viewBox="0 0 24 24" width="' +
      s +
      '" height="' +
      s +
      '" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
      path +
      "</svg>"
    );
  }

  function hydrateIcons(root) {
    const scope = root || document;
    Array.prototype.slice
      .call(scope.querySelectorAll("[data-icon]"))
      .forEach(function (el) {
        const size = parseInt(el.getAttribute("data-size") || "16", 10);
        el.outerHTML = icon(el.getAttribute("data-icon"), size);
      });
  }

  function escapeHtml(value) {
    return String(value === undefined || value === null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  const STATUS = {
    queued: { label: "排队中", cls: "badge--queued" },
    running: { label: "运行中", cls: "badge--running" },
    succeeded: { label: "已完成", cls: "badge--succeeded" },
    failed: { label: "失败", cls: "badge--failed" },
    canceled: { label: "已取消", cls: "badge--canceled" },
    interrupted: { label: "已中断", cls: "badge--interrupted" },
  };

  function statusBadge(status) {
    const meta = STATUS[status] || { label: status, cls: "badge--neutral" };
    return (
      '<span class="badge ' +
      meta.cls +
      '">' +
      escapeHtml(meta.label) +
      "</span>"
    );
  }

  function statusLabel(status) {
    return (STATUS[status] && STATUS[status].label) || status;
  }

  const ORGANISMS = {
    hsapiens: "人",
    mmusculus: "小鼠",
    rnorvegicus: "大鼠",
    dmelanogaster: "果蝇",
    scevisiae: "酿酒酵母",
    athaliana: "拟南芥",
    drerio: "斑马鱼",
  };

  function organismLabel(code) {
    return ORGANISMS[code] || code;
  }

  function fmtNumber(n) {
    if (n === undefined || n === null || Number.isNaN(n)) return "—";
    return new Intl.NumberFormat("zh-CN").format(n);
  }

  function fmtBytes(n) {
    if (n === undefined || n === null || Number.isNaN(n) || n <= 0) return "—";
    if (n < 1024) return n + " B";
    if (n < 1024 * 1024) return (n / 1024).toFixed(n < 10240 ? 1 : 0) + " KB";
    return (n / (1024 * 1024)).toFixed(1) + " MB";
  }

  function fmtDuration(ms) {
    if (!ms || ms <= 0) return "—";
    if (ms < 1000) return Math.round(ms) + " ms";
    if (ms < 60000) return (ms / 1000).toFixed(ms < 10000 ? 2 : 1) + " s";
    const m = Math.floor(ms / 60000);
    const s = Math.round((ms % 60000) / 1000);
    return m + " 分 " + s + " 秒";
  }

  function fmtDateTime(value) {
    if (!value) return "—";
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return "—";
    const pad = (x) => String(x).padStart(2, "0");
    return (
      d.getFullYear() +
      "-" +
      pad(d.getMonth() + 1) +
      "-" +
      pad(d.getDate()) +
      " " +
      pad(d.getHours()) +
      ":" +
      pad(d.getMinutes())
    );
  }

  function fmtRelative(value) {
    if (!value) return "—";
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return "—";
    const diff = Date.now() - d.getTime();
    if (diff < 0) return "刚刚";
    const min = Math.floor(diff / 60000);
    if (min < 1) return "刚刚";
    if (min < 60) return min + " 分钟前";
    const hour = Math.floor(min / 60);
    if (hour < 24) return hour + " 小时前";
    const day = Math.floor(hour / 24);
    if (day < 8) return day + " 天前";
    return fmtDateTime(value);
  }

  function fmtP(p) {
    if (p === undefined || p === null) return "—";
    if (p === 0) return "0";
    if (p < 1e-3) return p.toExponential(1);
    return p.toFixed(4);
  }

  function shortHash(hash, len) {
    if (!hash) return "—";
    const n = len || 10;
    return hash.length <= n ? hash : hash.slice(0, n) + "…";
  }

  /* ------------------------------- toasts -------------------------------- */

  function toast(type, title, message) {
    let stack = document.querySelector(".toast-stack");
    if (!stack) {
      stack = document.createElement("div");
      stack.className = "toast-stack";
      document.body.appendChild(stack);
    }
    const el = document.createElement("div");
    el.className = "toast toast--" + (type || "info");
    el.innerHTML =
      '<span class="toast__icon">' +
      icon(type === "error" ? "alert" : type === "success" ? "check" : "info", 17) +
      "</span>" +
      "<div><strong>" +
      escapeHtml(title) +
      "</strong>" +
      (message ? "<span>" + escapeHtml(message) + "</span>" : "") +
      "</div>";
    stack.appendChild(el);
    window.setTimeout(function () {
      el.style.opacity = "0";
      el.style.transform = "translateY(6px)";
      window.setTimeout(function () {
        el.remove();
      }, 200);
    }, type === "error" ? 6000 : 3800);
  }

  /* -------------------------------- shell -------------------------------- */

  function initShell() {
    hydrateIcons();
    syncNavCount();

    const sidebar = document.getElementById("sidebar");
    const scrim = document.getElementById("sidebarScrim");
    const toggle = document.getElementById("menuToggle");

    function closeSidebar() {
      if (!sidebar) return;
      sidebar.classList.remove("is-open");
      if (scrim) scrim.classList.remove("is-open");
    }

    if (toggle && sidebar) {
      toggle.addEventListener("click", function () {
        const open = sidebar.classList.toggle("is-open");
        if (scrim) scrim.classList.toggle("is-open", open);
      });
    }
    if (scrim) scrim.addEventListener("click", closeSidebar);

    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape") closeSidebar();
    });

    // Copy buttons for hashes, declared in markup as data-copy="value".
    document.addEventListener("click", function (e) {
      const btn = e.target.closest("[data-copy]");
      if (!btn) return;
      const value = btn.getAttribute("data-copy");
      if (!value) return;
      navigator.clipboard
        .writeText(value)
        .then(function () {
          toast("success", "已复制", "内容已写入剪贴板");
        })
        .catch(function () {
          toast("error", "复制失败", "浏览器拒绝了剪贴板权限");
        });
    });
  }

  /**
   * Sidebar run counter. Every page shows the same "运行历史" badge, so they all
   * read it from the same endpoint instead of each page inventing a number.
   */
  function syncNavCount() {
    const el = document.getElementById("navRunCount");
    const client = global.GP && global.GP.api;
    if (!el || !client || !client.dashboard) {
      return;
    }
    client
      .dashboard()
      .then(function (res) {
        const total = res && res.stats ? res.stats.total : res && res.total;
        el.textContent = fmtNumber(total || 0);
      })
      .catch(function () {
        el.textContent = "—";
      });
  }

  global.GP = global.GP || {};
  global.GP.ui = {
    icon: icon,
    hydrateIcons: hydrateIcons,
    escapeHtml: escapeHtml,
    statusBadge: statusBadge,
    statusLabel: statusLabel,
    organismLabel: organismLabel,
    fmtNumber: fmtNumber,
    fmtBytes: fmtBytes,
    fmtDuration: fmtDuration,
    fmtDateTime: fmtDateTime,
    fmtRelative: fmtRelative,
    fmtP: fmtP,
    shortHash: shortHash,
    toast: toast,
    syncNavCount: syncNavCount,
    initShell: initShell,
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initShell);
  } else {
    initShell();
  }
})(window);
