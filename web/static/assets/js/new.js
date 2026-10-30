/* GenePath Runner — new analysis page.
   Client-side parsing mirrors internal/gene so the counter reacts instantly;
   the server still re-validates everything, and the hash shown here comes from
   the server so it is guaranteed to match what gets stored. */
(function () {
  "use strict";

  const ui = window.GP.ui;
  const api = window.GP.api;
  const $ = (id) => document.getElementById(id);

  const SAMPLES = {
    "cell-cycle": [
      "CDK1", "CDK2", "CCNB1", "CCNB2", "CCNE1", "CCNA2", "MCM2", "MCM3",
      "MCM4", "MCM7", "PCNA", "RFC2", "AURKA", "AURKB", "PLK1", "BUB1",
      "BUB1B", "MAD2L1", "TOP2A", "CENPE", "KIF11", "TP53", "CDKN1A", "E2F1",
    ],
    apoptosis: [
      "TP53", "BAX", "BAK1", "BCL2", "BCL2L1", "CASP3", "CASP8", "CASP9",
      "APAF1", "CYCS", "DIABLO", "XIAP", "FAS", "FADD", "BID", "PMAIP1",
      "BBC3", "CDKN1A",
    ],
    immune: [
      "CD3D", "CD3E", "CD28", "ICOS", "IL2", "IL2RA", "IL2RB", "IL7R",
      "JAK1", "JAK3", "STAT5A", "STAT5B", "LCK", "ZAP70", "LAT", "PLCG1",
      "PRKCQ", "CARD11", "NFKB1", "RELA", "NFATC1", "NFATC2", "CCR7", "SELL",
      "LTB", "TNFRSF4",
    ],
  };

  let maxGenes = 2000;
  let validateTimer = null;
  let validateSeq = 0;

  const SYMBOL = /^[\p{L}\p{N}][\p{L}\p{N}._-]*$/u;

  function parseRaw(text) {
    return text
      .split(/[\s,;]+/)
      .map((t) => t.trim())
      .filter((t) => t.length > 0 && t.charAt(0) !== "#");
  }

  function normalize(tokens) {
    const seen = new Set();
    const out = [];
    tokens.forEach(function (t) {
      const key = t.toLowerCase();
      if (seen.has(key)) return;
      seen.add(key);
      out.push(t);
    });
    return out;
  }

  function isSymbol(token) {
    return token.length <= 32 && SYMBOL.test(token);
  }

  function fieldOf(el) {
    return el.closest(".field");
  }

  function setError(el, message) {
    const field = fieldOf(el);
    if (!field) return;
    field.classList.toggle("has-error", Boolean(message));
    const box = field.querySelector(".field__error");
    if (box) box.textContent = message || "";
  }

  function clearErrors() {
    document.querySelectorAll(".field.has-error").forEach(function (f) {
      f.classList.remove("has-error");
    });
  }

  function currentSources() {
    return Array.prototype.slice
      .call(document.querySelectorAll("#sourcesBox input:checked"))
      .map((i) => i.value);
  }

  function updateSummary(state) {
    $("sumGenes").textContent = ui.fmtNumber(state.accepted.length);
    $("sumDuplicates").textContent = ui.fmtNumber(state.duplicates);
    $("sumRejected").textContent = ui.fmtNumber(state.rejectedCount);
    $("sumOrganism").textContent = ui.organismLabel($("organism").value);
    $("sumSources").textContent = currentSources().length + " 个";
    $("sumParams").textContent =
      $("significance").value + " · " + $("correction").value;
    $("sumHash").textContent = state.hash || "等待输入…";

    $("geneValid").textContent = state.accepted.length;
    $("geneRejected").textContent = state.rejectedCount;
    $("geneCounter").classList.toggle("has-rejected", state.rejectedCount > 0);

    const notice = $("duplicateNotice");
    if (state.duplicateOf) {
      notice.hidden = false;
      $("duplicateText").innerHTML =
        "该基因集与已有运行 <code class=\"mono\">" +
        ui.escapeHtml(state.duplicateOf) +
        "</code> 的输入哈希一致。仍可再次提交，两条记录会并列保存。";
    } else {
      notice.hidden = true;
    }
  }

  function localState() {
    const raw = parseRaw($("genes").value);
    const normalized = normalize(raw);
    const accepted = [];
    const rejected = [];
    normalized.forEach(function (t) {
      if (isSymbol(t)) accepted.push(t);
      else rejected.push(t);
    });
    return {
      accepted: accepted,
      rejected: rejected,
      rejectedCount: rejected.length,
      duplicates: raw.length - normalized.length,
      hash: "",
      duplicateOf: "",
    };
  }

  function refresh() {
    const local = localState();
    updateSummary(local);

    if (local.accepted.length === 0 || local.accepted.length > maxGenes) {
      $("sumHash").textContent = "等待输入…";
      return;
    }

    window.clearTimeout(validateTimer);
    validateTimer = window.setTimeout(function () {
      const seq = ++validateSeq;
      api
        .validateRun({
          genes: $("genes").value,
          organism: $("organism").value,
        })
        .then(function (res) {
          if (seq !== validateSeq) return;
          updateSummary({
            accepted: res.accepted,
            rejected: res.rejected,
            rejectedCount: res.rejectedCount,
            duplicates: res.duplicates,
            hash: res.inputHash,
            duplicateOf: res.duplicateOf || "",
          });
        })
        .catch(function () {
          /* The live hash is a convenience; failing it must not block input. */
        });
    }, 320);
  }

  function applyValidation(details) {
    clearErrors();
    if (!details) return;
    if (details.genes) setError($("genes"), details.genes);
    if (details.organism) setError($("organism"), details.organism);
    if (details.sources) {
      const box = $("sourcesBox").closest(".field");
      if (box) {
        box.classList.add("has-error");
        const err = box.querySelector(".field__error");
        if (err) err.textContent = details.sources;
      }
    }
    if (details.significance) setError($("significance"), details.significance);
  }

  function buildCheckboxes(meta) {
    const box = $("sourcesBox");
    box.innerHTML = "";
    meta.sources.forEach(function (src, idx) {
      const label = document.createElement("label");
      label.className = "check";

      const input = document.createElement("input");
      input.type = "checkbox";
      input.value = src.code;
      input.checked = idx === 0;
      input.addEventListener("change", refresh);

      const text = document.createElement("span");
      text.textContent = src.label;

      const code = document.createElement("code");
      code.textContent = src.code;

      label.appendChild(input);
      label.appendChild(text);
      label.appendChild(code);
      box.appendChild(label);
    });
  }

  function fillSelect(select, items) {
    select.innerHTML = "";
    items.forEach(function (item) {
      const opt = document.createElement("option");
      opt.value = item.code;
      opt.textContent = item.label;
      select.appendChild(opt);
    });
  }

  async function loadMeta() {
    try {
      const meta = await api.meta();
      maxGenes = meta.maxGenes;
      $("geneMax").textContent = ui.fmtNumber(maxGenes);

      const organisms = $("organism");
      organisms.innerHTML = "";
      meta.organisms.forEach(function (o) {
        const opt = document.createElement("option");
        opt.value = o.code;
        opt.textContent = o.chinese + " · " + o.label;
        organisms.appendChild(opt);
      });
      organisms.value = meta.defaults.organism;

      fillSelect($("correction"), meta.corrections);
      $("correction").value = meta.defaults.correction;
      buildCheckboxes(meta);
      refresh();
    } catch (err) {
      ui.toast("error", "无法加载参数元数据", err.message);
    }
  }

  function bindEvents() {
    $("genes").addEventListener("input", function () {
      setError($("genes"), "");
      refresh();
    });
    $("organism").addEventListener("change", refresh);
    $("significance").addEventListener("change", refresh);
    $("correction").addEventListener("change", refresh);

    Array.prototype.slice
      .call(document.querySelectorAll("[data-sample]"))
      .forEach(function (btn) {
        btn.addEventListener("click", function () {
          const key = btn.getAttribute("data-sample");
          $("genes").value = SAMPLES[key].join("\n");
          setError($("genes"), "");
          refresh();
          ui.toast("info", "已填入示例基因集", SAMPLES[key].length + " 个符号");
        });
      });

    $("clearGenes").addEventListener("click", function () {
      $("genes").value = "";
      refresh();
      $("genes").focus();
    });

    $("pasteFocus").addEventListener("click", function () {
      $("genes").focus();
      $("genes").select();
    });

    $("resetBtn").addEventListener("click", function () {
      $("analysisForm").reset();
      clearErrors();
      refresh();
    });

    $("analysisForm").addEventListener("submit", submit);
  }

  async function submit(event) {
    event.preventDefault();
    clearErrors();

    const button = $("submitBtn");
    const state = localState();

    if (state.accepted.length === 0) {
      setError($("genes"), "请至少输入 1 个有效的基因符号");
      $("genes").focus();
      return;
    }
    if (state.accepted.length > maxGenes) {
      setError($("genes"), "一次最多 " + maxGenes + " 个基因");
      return;
    }
    if (state.rejectedCount > 0) {
      setError(
        $("genes"),
        "有 " + state.rejectedCount + " 个符号包含不支持的字符，请先修正"
      );
      return;
    }
    if (currentSources().length === 0) {
      ui.toast("error", "请选择至少一个注释来源");
      return;
    }

    button.classList.add("btn--loading");
    button.disabled = true;
    $("submitHint").textContent = "正在提交…";

    try {
      const detail = await api.createRun({
        label: $("runLabel").value.trim(),
        genes: $("genes").value,
        organism: $("organism").value,
        sources: currentSources(),
        significance: $("significance").value,
        correction: $("correction").value,
        background: $("background").value,
      });
      $("submitHint").textContent = "已入队，正在打开运行详情…";
      ui.toast("success", "任务已入队", "worker 正在调用 g:Profiler：" + detail.id);
      window.setTimeout(function () {
        window.location.href = "/runs/" + encodeURIComponent(detail.id);
      }, 300);
    } catch (err) {
      button.classList.remove("btn--loading");
      button.disabled = false;
      $("submitHint").textContent = "提交后由后台 worker 调用 g:Profiler 分析";
      if (err.code === "validation_failed") {
        applyValidation(err.details);
        ui.toast("error", "提交未通过校验", err.message);
      } else if (err.code === "executor_not_ready") {
        ui.toast("error", "分析执行器未就绪", err.message);
      } else if (err.code === "queue_full") {
        ui.toast("error", "执行队列已满", err.message);
      } else if (err.code === "network_error") {
        ui.toast("error", "无法连接服务器", "请确认服务仍在运行（go run ./cmd/server）");
      } else {
        ui.toast("error", "提交失败", err.message);
      }
    }
  }

  function prefillFromQuery() {
    // Shareable link support: /new?genes=TP53,BAX&organism=mmusculus
    const params = new URLSearchParams(window.location.search);
    const genes = params.get("genes");
    if (genes) {
      $("genes").value = genes.replace(/,/g, "\n");
    }
    const organism = params.get("organism");
    if (organism && $("organism").querySelector('option[value="' + organism + '"]')) {
      $("organism").value = organism;
    }
  }

  bindEvents();
  prefillFromQuery();
  loadMeta();
})();
