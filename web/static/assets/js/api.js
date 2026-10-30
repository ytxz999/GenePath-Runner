/* GenePath Runner — thin API client.
   Every network call goes through here so error handling, base paths and
   response shapes stay in one place. */

(function (global) {
  "use strict";

  class ApiError extends Error {
    constructor(message, code, status, details) {
      super(message);
      this.name = "ApiError";
      this.code = code || "unknown";
      this.status = status || 0;
      this.details = details || null;
    }
  }

  async function request(path, options) {
    const opts = Object.assign({ headers: {} }, options || {});
    if (opts.body && typeof opts.body !== "string") {
      opts.body = JSON.stringify(opts.body);
      opts.headers["Content-Type"] = "application/json";
    }
    opts.headers["Accept"] = "application/json";

    let response;
    try {
      response = await fetch(path, opts);
    } catch (err) {
      throw new ApiError("无法连接服务器，请确认服务仍在运行。", "network_error", 0);
    }

    const text = await response.text();
    let payload = null;
    if (text) {
      try {
        payload = JSON.parse(text);
      } catch (err) {
        payload = null;
      }
    }

    if (!response.ok) {
      const message =
        (payload && payload.message) ||
        "请求失败（HTTP " + response.status + "）";
      throw new ApiError(
        message,
        (payload && payload.code) || "http_" + response.status,
        response.status,
        payload && payload.details
      );
    }
    return payload;
  }

  const BASE = "/api/v1";

  const api = {
    ApiError,

    meta() {
      return request(BASE + "/meta");
    },

    dashboard() {
      return request(BASE + "/dashboard");
    },

    listRuns(params) {
      const qs = new URLSearchParams();
      Object.keys(params || {}).forEach((k) => {
        const v = params[k];
        if (v !== undefined && v !== null && v !== "") qs.set(k, v);
      });
      const query = qs.toString();
      return request(BASE + "/runs" + (query ? "?" + query : ""));
    },

    getRun(id) {
      return request(BASE + "/runs/" + encodeURIComponent(id));
    },

    results(id) {
      return request(BASE + "/runs/" + encodeURIComponent(id) + "/results");
    },

    createRun(body) {
      return request(BASE + "/runs", { method: "POST", body: body });
    },

    validateRun(body) {
      return request(BASE + "/runs/validate", { method: "POST", body: body });
    },

    reportUrl(id) {
      return BASE + "/runs/" + encodeURIComponent(id) + "/report.md";
    },

    reportHtmlUrl(id) {
      return BASE + "/runs/" + encodeURIComponent(id) + "/report.html";
    },

    manifestUrl(id) {
      return BASE + "/runs/" + encodeURIComponent(id) + "/manifest.json";
    },

    artifactUrl(id, kind) {
      return (
        BASE +
        "/runs/" +
        encodeURIComponent(id) +
        "/artifacts/" +
        encodeURIComponent(kind)
      );
    },

    artifacts(id) {
      return request(BASE + "/runs/" + encodeURIComponent(id) + "/artifacts");
    },

    rerun(id) {
      return request(BASE + "/runs/" + encodeURIComponent(id) + "/rerun", {
        method: "POST",
      });
    },

    rawUrl(id) {
      return BASE + "/runs/" + encodeURIComponent(id) + "/raw";
    },

    clearDemo() {
      return request(BASE + "/demo", { method: "DELETE" });
    },
  };

  global.GP = global.GP || {};
  global.GP.api = api;
})(window);
