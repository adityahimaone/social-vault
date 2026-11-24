(function () {
  "use strict";

  if (window.__socialVaultBulkInitialized) {
    return;
  }
  window.__socialVaultBulkInitialized = true;

  var extractor = globalThis.SocialVaultBulkExtractor;
  var runner = globalThis.SocialVaultBulkRunner;
  if (!extractor || !runner) {
    return;
  }

  var ROOT_ID = "social-vault-bulk-root";
  var MAX_POSTS = extractor.MAX_POSTS_PER_SESSION || 200;

  var currentHref = "";
  var activeRoute = null;
  var hostEl = null;
  var shadow = null;
  var domObserver = null;
  var scanTimer = null;
  var routeInterval = null;
  var activeSession = null;

  var state = {
    collapsed: false,
    autoScroll: true,
    detectedUrls: [],
    phase: "idle", // idle | scrolling | saving | done | stopped | error
    collected: 0,
    total: 0,
    processed: 0,
    saved: 0,
    skipped: 0,
    failed: 0,
    limitReached: false,
    resumedFromScroll: false,
    alertKind: "", // "" | "info" | "error" | "success"
    alertTitle: "",
    alertDetail: "",
    showSettingsAction: false
  };

  var refs = {};

  function sendRuntimeMessage(payload) {
    return new Promise(function (resolve) {
      if (!chrome || !chrome.runtime || !chrome.runtime.sendMessage) {
        resolve({ ok: false, error: "Extension runtime is unavailable." });
        return;
      }
      try {
        chrome.runtime.sendMessage(payload, function (response) {
          if (chrome.runtime.lastError) {
            resolve({
              ok: false,
              error: chrome.runtime.lastError.message || "Extension communication failed."
            });
            return;
          }
          resolve(response || { ok: false, error: "Empty response from extension." });
        });
      } catch (err) {
        resolve({
          ok: false,
          error: (err && err.message) || "Could not message extension background."
        });
      }
    });
  }

  function createSvgIcon(type) {
    var svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("aria-hidden", "true");

    if (type === "bookmark") {
      svg.setAttribute("width", "15");
      svg.setAttribute("height", "15");
      svg.setAttribute("fill", "currentColor");
      var path = document.createElementNS("http://www.w3.org/2000/svg", "path");
      path.setAttribute("d", "M6 3a2 2 0 0 0-2 2v16l8-4 8 4V5a2 2 0 0 0-2-2H6z");
      svg.appendChild(path);
      return svg;
    }

    svg.setAttribute("width", "15");
    svg.setAttribute("height", "15");
    svg.setAttribute("fill", "none");
    svg.setAttribute("stroke", "currentColor");
    svg.setAttribute("stroke-width", "2");
    svg.setAttribute("stroke-linecap", "round");
    svg.setAttribute("stroke-linejoin", "round");

    if (type === "chevron") {
      var poly = document.createElementNS("http://www.w3.org/2000/svg", "polyline");
      poly.setAttribute("points", "6 9 12 15 18 9");
      svg.appendChild(poly);
    } else if (type === "stop") {
      var rect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
      rect.setAttribute("x", "6");
      rect.setAttribute("y", "6");
      rect.setAttribute("width", "12");
      rect.setAttribute("height", "12");
      rect.setAttribute("rx", "2");
      rect.setAttribute("fill", "currentColor");
      rect.setAttribute("stroke", "none");
      svg.appendChild(rect);
    } else if (type === "save") {
      var p1 = document.createElementNS("http://www.w3.org/2000/svg", "path");
      p1.setAttribute("d", "M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z");
      var p2 = document.createElementNS("http://www.w3.org/2000/svg", "polyline");
      p2.setAttribute("points", "17 21 17 13 7 13 7 21");
      var p3 = document.createElementNS("http://www.w3.org/2000/svg", "polyline");
      p3.setAttribute("points", "7 3 7 8 15 8");
      svg.appendChild(p1);
      svg.appendChild(p2);
      svg.appendChild(p3);
    } else if (type === "arrow-down") {
      svg.setAttribute("width", "13");
      svg.setAttribute("height", "13");
      var arrowLine = document.createElementNS("http://www.w3.org/2000/svg", "line");
      arrowLine.setAttribute("x1", "12");
      arrowLine.setAttribute("y1", "5");
      arrowLine.setAttribute("x2", "12");
      arrowLine.setAttribute("y2", "19");
      var arrowHead = document.createElementNS("http://www.w3.org/2000/svg", "polyline");
      arrowHead.setAttribute("points", "19 12 12 19 5 12");
      svg.appendChild(arrowLine);
      svg.appendChild(arrowHead);
    } else if (type === "alert") {
      var circle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      circle.setAttribute("cx", "12");
      circle.setAttribute("cy", "12");
      circle.setAttribute("r", "10");
      var l1 = document.createElementNS("http://www.w3.org/2000/svg", "line");
      l1.setAttribute("x1", "12");
      l1.setAttribute("y1", "8");
      l1.setAttribute("x2", "12");
      l1.setAttribute("y2", "12");
      var l2 = document.createElementNS("http://www.w3.org/2000/svg", "line");
      l2.setAttribute("x1", "12");
      l2.setAttribute("y1", "16");
      l2.setAttribute("x2", "12.01");
      l2.setAttribute("y2", "16");
      svg.appendChild(circle);
      svg.appendChild(l1);
      svg.appendChild(l2);
    }

    return svg;
  }

  function buildStyles() {
    var style = document.createElement("style");
    style.textContent = [
      ":host {",
      "  --font-sans: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;",
      "  --font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;",
      "  --ease-out: cubic-bezier(0.23, 1, 0.32, 1);",
      "  --bg: #0F172A;",
      "  --surface: #192134;",
      "  --surface-muted: #10192E;",
      "  --surface-hover: rgba(255, 255, 255, 0.07);",
      "  --fg: #F8FAFC;",
      "  --fg-secondary: #CBD5E1;",
      "  --fg-muted: #94A3B8;",
      "  --primary: #1E3A5F;",
      "  --accent: #10B981;",
      "  --accent-hover: #059669;",
      "  --on-accent: #041B11;",
      "  --accent-soft: rgba(16, 185, 129, 0.14);",
      "  --accent-text: #34D399;",
      "  --info-soft: rgba(56, 189, 248, 0.14);",
      "  --info-border: rgba(56, 189, 248, 0.32);",
      "  --info-text: #7DD3FC;",
      "  --border: rgba(255, 255, 255, 0.12);",
      "  --border-strong: rgba(255, 255, 255, 0.24);",
      "  --destructive: #EF4444;",
      "  --destructive-hover: #DC2626;",
      "  --destructive-soft: rgba(239, 68, 68, 0.14);",
      "  --destructive-border: rgba(239, 68, 68, 0.34);",
      "  --destructive-text: #FCA5A5;",
      "  --ring: #38BDF8;",
      "  --shadow: 0 12px 32px rgba(2, 6, 23, 0.45), 0 2px 6px rgba(2, 6, 23, 0.3);",
      "  position: fixed;",
      "  right: 20px;",
      "  bottom: 20px;",
      "  z-index: 2147483640;",
      "  font-family: var(--font-sans);",
      "  font-size: 13px;",
      "  line-height: 1.45;",
      "  color: var(--fg);",
      "  -webkit-font-smoothing: antialiased;",
      "}",
      "@media (prefers-color-scheme: light) {",
      "  :host {",
      "    --bg: #F8FAFC;",
      "    --surface: #FFFFFF;",
      "    --surface-muted: #F1F5F9;",
      "    --surface-hover: rgba(15, 23, 42, 0.05);",
      "    --fg: #0F172A;",
      "    --fg-secondary: #334155;",
      "    --fg-muted: #475569;",
      "    --primary: #1E3A5F;",
      "    --accent: #047857;",
      "    --accent-hover: #065F46;",
      "    --on-accent: #FFFFFF;",
      "    --accent-soft: rgba(4, 120, 87, 0.10);",
      "    --accent-text: #047857;",
      "    --info-soft: rgba(2, 132, 199, 0.10);",
      "    --info-border: rgba(2, 132, 199, 0.28);",
      "    --info-text: #0369A1;",
      "    --border: rgba(15, 23, 42, 0.12);",
      "    --border-strong: rgba(15, 23, 42, 0.24);",
      "    --destructive: #DC2626;",
      "    --destructive-hover: #B91C1C;",
      "    --destructive-soft: rgba(220, 38, 38, 0.09);",
      "    --destructive-border: rgba(220, 38, 38, 0.28);",
      "    --destructive-text: #B91C1C;",
      "    --ring: #1E3A5F;",
      "    --shadow: 0 12px 30px rgba(15, 23, 42, 0.14), 0 2px 6px rgba(15, 23, 42, 0.08);",
      "  }",
      "}",
      "*, *::before, *::after { box-sizing: border-box; }",
      ".hidden { display: none !important; }",
      ".panel {",
      "  width: 312px;",
      "  max-width: calc(100vw - 24px);",
      "  background-color: var(--surface);",
      "  border: 1px solid var(--border);",
      "  border-radius: 14px;",
      "  box-shadow: var(--shadow);",
      "  padding: 13px;",
      "  display: flex;",
      "  flex-direction: column;",
      "  gap: 11px;",
      "  opacity: 0;",
      "  transform: scale(0.96) translateY(8px);",
      "  transform-origin: bottom right;",
      "  transition: opacity 200ms var(--ease-out), transform 200ms var(--ease-out);",
      "}",
      ".panel[data-mounted='true'] {",
      "  opacity: 1;",
      "  transform: scale(1) translateY(0);",
      "}",
      ".header {",
      "  display: flex;",
      "  align-items: center;",
      "  justify-content: space-between;",
      "  gap: 8px;",
      "}",
      ".brand {",
      "  display: flex;",
      "  align-items: center;",
      "  gap: 9px;",
      "  min-width: 0;",
      "}",
      ".brand-mark {",
      "  width: 28px;",
      "  height: 28px;",
      "  border-radius: 8px;",
      "  background-color: var(--primary);",
      "  border: 1px solid rgba(255, 255, 255, 0.12);",
      "  color: var(--accent-text);",
      "  display: grid;",
      "  place-items: center;",
      "  flex-shrink: 0;",
      "}",
      ".title-group { min-width: 0; }",
      ".title {",
      "  margin: 0;",
      "  font-size: 13px;",
      "  font-weight: 700;",
      "  letter-spacing: -0.01em;",
      "  color: var(--fg);",
      "  line-height: 1.2;",
      "}",
      ".subtitle {",
      "  margin: 1px 0 0;",
      "  font-size: 11.5px;",
      "  color: var(--fg-muted);",
      "  white-space: nowrap;",
      "  overflow: hidden;",
      "  text-overflow: ellipsis;",
      "}",
      ".header-right {",
      "  display: flex;",
      "  align-items: center;",
      "  gap: 6px;",
      "  flex-shrink: 0;",
      "}",
      ".count-pill {",
      "  display: inline-flex;",
      "  align-items: center;",
      "  padding: 3px 8px;",
      "  border-radius: 999px;",
      "  background-color: var(--accent-soft);",
      "  color: var(--accent-text);",
      "  font-size: 11.5px;",
      "  font-weight: 600;",
      "  font-variant-numeric: tabular-nums;",
      "  white-space: nowrap;",
      "}",
      ".icon-btn {",
      "  width: 28px;",
      "  height: 28px;",
      "  border-radius: 8px;",
      "  border: 1px solid transparent;",
      "  background: transparent;",
      "  color: var(--fg-secondary);",
      "  display: grid;",
      "  place-items: center;",
      "  cursor: pointer;",
      "  padding: 0;",
      "  transition: transform 140ms var(--ease-out), background-color 160ms ease, color 160ms ease;",
      "}",
      ".icon-btn svg {",
      "  transition: transform 180ms var(--ease-out);",
      "}",
      ".panel[data-collapsed='true'] .icon-btn svg {",
      "  transform: rotate(180deg);",
      "}",
      ".icon-btn:active:not(:disabled) { transform: scale(0.95); }",
      ".body {",
      "  display: flex;",
      "  flex-direction: column;",
      "  gap: 10px;",
      "}",
      ".toggle-row {",
      "  display: flex;",
      "  align-items: center;",
      "  justify-content: space-between;",
      "  gap: 10px;",
      "  padding: 8px 10px;",
      "  border-radius: 10px;",
      "  background-color: var(--surface-muted);",
      "  border: 1px solid var(--border);",
      "}",
      ".toggle-copy {",
      "  min-width: 0;",
      "  display: flex;",
      "  flex-direction: column;",
      "  gap: 1px;",
      "}",
      ".toggle-label {",
      "  font-size: 12.5px;",
      "  font-weight: 600;",
      "  color: var(--fg);",
      "}",
      ".toggle-hint {",
      "  font-size: 11px;",
      "  color: var(--fg-muted);",
      "}",
      ".switch {",
      "  position: relative;",
      "  width: 38px;",
      "  height: 22px;",
      "  border-radius: 999px;",
      "  border: 1px solid var(--border-strong);",
      "  background-color: var(--bg);",
      "  padding: 0;",
      "  cursor: pointer;",
      "  flex-shrink: 0;",
      "  transition: transform 140ms var(--ease-out), background-color 160ms ease, border-color 160ms ease;",
      "}",
      ".switch[aria-checked='true'] {",
      "  background-color: var(--accent);",
      "  border-color: transparent;",
      "}",
      ".switch:active:not(:disabled) { transform: scale(0.96); }",
      ".switch:disabled { opacity: 0.55; cursor: not-allowed; }",
      ".switch-thumb {",
      "  position: absolute;",
      "  top: 2px;",
      "  left: 2px;",
      "  width: 16px;",
      "  height: 16px;",
      "  border-radius: 50%;",
      "  background-color: #FFFFFF;",
      "  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);",
      "  transform: translateX(0);",
      "  transition: transform 160ms var(--ease-out);",
      "}",
      ".switch[aria-checked='true'] .switch-thumb {",
      "  transform: translateX(16px);",
      "}",
      ".btn {",
      "  width: 100%;",
      "  min-height: 38px;",
      "  padding: 0 12px;",
      "  border-radius: 10px;",
      "  border: 1px solid transparent;",
      "  font-family: inherit;",
      "  font-size: 13px;",
      "  font-weight: 600;",
      "  display: inline-flex;",
      "  align-items: center;",
      "  justify-content: center;",
      "  gap: 7px;",
      "  cursor: pointer;",
      "  transition: transform 140ms var(--ease-out), background-color 160ms ease, border-color 160ms ease, color 160ms ease;",
      "}",
      ".btn:active:not(:disabled) { transform: scale(0.97); }",
      ".btn:disabled { opacity: 0.55; cursor: not-allowed; }",
      ".btn-primary { background-color: var(--accent); color: var(--on-accent); }",
      ".btn-stop { background-color: var(--destructive); color: #FFFFFF; }",
      ".btn-secondary {",
      "  background-color: var(--surface-muted);",
      "  border-color: var(--border);",
      "  color: var(--fg);",
      "}",
      ".btn-sm { min-height: 30px; font-size: 12px; width: auto; padding: 0 10px; border-radius: 7px; }",
      ".progress-card {",
      "  display: flex;",
      "  flex-direction: column;",
      "  gap: 8px;",
      "  padding: 9px 10px;",
      "  border-radius: 10px;",
      "  background-color: var(--surface-muted);",
      "  border: 1px solid var(--border);",
      "}",
      ".status-line {",
      "  display: flex;",
      "  align-items: center;",
      "  justify-content: space-between;",
      "  gap: 8px;",
      "  font-size: 12px;",
      "  color: var(--fg-secondary);",
      "}",
      ".status-text { font-weight: 500; min-width: 0; }",
      ".status-count { font-variant-numeric: tabular-nums; font-weight: 600; color: var(--fg); white-space: nowrap; }",
      ".resume-note {",
      "  display: flex;",
      "  align-items: center;",
      "  gap: 6px;",
      "  padding: 5px 8px;",
      "  border-radius: 7px;",
      "  background-color: var(--info-soft);",
      "  border: 1px solid var(--info-border);",
      "  color: var(--info-text);",
      "  font-size: 11.5px;",
      "  font-weight: 500;",
      "  line-height: 1.35;",
      "  opacity: 1;",
      "  transform: translateY(0);",
      "  transition: opacity 160ms var(--ease-out), transform 160ms var(--ease-out);",
      "}",
      ".resume-note svg { flex-shrink: 0; }",
      "@starting-style {",
      "  .resume-note { opacity: 0; transform: translateY(-2px); }",
      "}",
      ".progress-track {",
      "  width: 100%;",
      "  height: 5px;",
      "  border-radius: 999px;",
      "  background-color: var(--border);",
      "  overflow: hidden;",
      "}",
      ".progress-fill {",
      "  width: 100%;",
      "  height: 100%;",
      "  border-radius: 999px;",
      "  background-color: var(--accent);",
      "  transform-origin: left center;",
      "  transform: scaleX(0);",
      "  transition: transform 180ms var(--ease-out);",
      "}",
      ".metrics {",
      "  display: grid;",
      "  grid-template-columns: repeat(3, 1fr);",
      "  gap: 6px;",
      "}",
      ".metric {",
      "  padding: 5px 6px;",
      "  border-radius: 8px;",
      "  background-color: var(--surface);",
      "  border: 1px solid var(--border);",
      "  display: flex;",
      "  flex-direction: column;",
      "  align-items: center;",
      "  gap: 1px;",
      "}",
      ".metric-val {",
      "  font-size: 13px;",
      "  font-weight: 700;",
      "  font-variant-numeric: tabular-nums;",
      "  line-height: 1.2;",
      "}",
      ".metric-lbl { font-size: 10.5px; color: var(--fg-muted); }",
      ".metric-saved .metric-val { color: var(--accent-text); }",
      ".metric-skipped .metric-val { color: var(--info-text); }",
      ".metric-failed .metric-val { color: var(--destructive-text); }",
      ".alert {",
      "  display: flex;",
      "  flex-direction: column;",
      "  gap: 7px;",
      "  padding: 9px 10px;",
      "  border-radius: 10px;",
      "  border: 1px solid var(--border);",
      "  font-size: 12px;",
      "  line-height: 1.4;",
      "}",
      ".alert-head { display: flex; align-items: flex-start; gap: 7px; }",
      ".alert-head svg { flex-shrink: 0; margin-top: 1px; }",
      ".alert-title { font-weight: 600; display: block; }",
      ".alert-detail { display: block; margin-top: 1px; opacity: 0.92; }",
      ".alert-error {",
      "  background-color: var(--destructive-soft);",
      "  border-color: var(--destructive-border);",
      "  color: var(--destructive-text);",
      "}",
      ".alert-info {",
      "  background-color: var(--info-soft);",
      "  border-color: var(--info-border);",
      "  color: var(--info-text);",
      "}",
      ".spinner {",
      "  width: 14px;",
      "  height: 14px;",
      "  border: 2px solid currentColor;",
      "  border-right-color: transparent;",
      "  border-radius: 50%;",
      "  animation: sv-bulk-spin 650ms linear infinite;",
      "}",
      "button:focus-visible {",
      "  outline: 2px solid var(--ring);",
      "  outline-offset: 2px;",
      "}",
      "@media (hover: hover) and (pointer: fine) {",
      "  .icon-btn:hover:not(:disabled) { background-color: var(--surface-hover); color: var(--fg); }",
      "  .btn-primary:hover:not(:disabled) { background-color: var(--accent-hover); }",
      "  .btn-stop:hover:not(:disabled) { background-color: var(--destructive-hover); }",
      "  .btn-secondary:hover:not(:disabled) { background-color: var(--surface-hover); border-color: var(--border-strong); }",
      "}",
      "@keyframes sv-bulk-spin { to { transform: rotate(360deg); } }",
      "@media (prefers-reduced-motion: reduce) {",
      "  .panel { transform: none; transition: opacity 150ms ease; }",
      "  .btn, .icon-btn, .switch, .switch-thumb, .progress-fill, .icon-btn svg, .resume-note { transition: none; }",
      "  .resume-note { transform: none; }",
      "  .btn:active:not(:disabled), .icon-btn:active:not(:disabled), .switch:active:not(:disabled) { transform: none; }",
      "  .spinner { animation-duration: 1.4s; }",
      "}"
    ].join("\n");
    return style;
  }

  function getCurrentScrollY() {
    var raw = typeof window.scrollY === "number" ? window.scrollY : window.pageYOffset;
    if (typeof raw !== "number" || !isFinite(raw) || raw <= 0) {
      return 0;
    }
    return raw;
  }

  function scanVisibleUrls() {
    if (!activeRoute) {
      return [];
    }
    var found = extractor.extractPostUrls(activeRoute.platform, document, window.location.origin);
    var merged = extractor.mergeUniqueUrls(state.detectedUrls, found, MAX_POSTS);
    state.detectedUrls = merged.urls;
    return found;
  }

  function scheduleScan() {
    if (!activeRoute || scanTimer !== null) {
      return;
    }
    if (activeSession && activeSession.isRunning()) {
      return;
    }
    scanTimer = setTimeout(function () {
      scanTimer = null;
      if (activeSession && activeSession.isRunning()) {
        return;
      }
      scanVisibleUrls();
      render();
    }, 350);
  }

  function setAlert(kind, title, detail, showSettingsAction) {
    state.alertKind = kind || "";
    state.alertTitle = title || "";
    state.alertDetail = detail || "";
    state.showSettingsAction = Boolean(showSettingsAction);
  }

  function clearAlert() {
    setAlert("", "", "", false);
  }

  function render() {
    if (!hostEl || !activeRoute || !refs.panel) {
      return;
    }

    var busy = state.phase === "collecting" || state.phase === "scrolling" || state.phase === "saving";
    refs.panel.setAttribute("data-collapsed", state.collapsed ? "true" : "false");
    refs.routeLabel.textContent = activeRoute.label;

    var count = state.phase === "idle" ? state.detectedUrls.length : state.collected;
    refs.countPill.textContent = count === 1 ? "1 post" : count + " posts";
    refs.collapseBtn.setAttribute("aria-expanded", state.collapsed ? "false" : "true");
    refs.collapseBtn.setAttribute("aria-label", state.collapsed ? "Expand Social Vault panel" : "Collapse Social Vault panel");
    refs.body.classList.toggle("hidden", state.collapsed);

    refs.switchBtn.setAttribute("aria-checked", state.autoScroll ? "true" : "false");
    refs.switchBtn.disabled = busy;

    refs.actionBtn.className = busy ? "btn btn-stop" : "btn btn-primary";
    refs.actionBtn.replaceChildren();

    if (busy) {
      refs.actionBtn.appendChild(createSvgIcon("stop"));
      var stopTxt = document.createElement("span");
      stopTxt.textContent = "Stop";
      refs.actionBtn.appendChild(stopTxt);
    } else {
      refs.actionBtn.appendChild(createSvgIcon("save"));
      var saveTxt = document.createElement("span");
      saveTxt.textContent = "Save all to Vault";
      refs.actionBtn.appendChild(saveTxt);
    }

    var showProgress = state.phase !== "idle";
    refs.progressCard.classList.toggle("hidden", !showProgress);
    refs.resumeNote.classList.toggle("hidden", !(state.resumedFromScroll && busy));

    if (showProgress) {
      var statusLabel = "Ready";
      var countText = state.processed + " / " + state.total;

      if (state.phase === "collecting") {
        statusLabel = "Collecting posts…";
        countText = state.collected + " found";
      } else if (state.phase === "scrolling") {
        statusLabel = state.limitReached ? "Reached 200-post limit" : "Scrolling & collecting posts…";
        countText = state.collected + " found";
      } else if (state.phase === "saving") {
        statusLabel = "Saving to Vault…";
        countText = state.processed + " / " + state.total;
      } else if (state.phase === "done") {
        statusLabel = state.limitReached ? "Finished (200-post limit)" : "Bulk save complete";
        countText = state.processed + " / " + state.total;
      } else if (state.phase === "stopped") {
        statusLabel = "Stopped";
        countText = state.processed + " / " + state.total;
      } else if (state.phase === "error") {
        statusLabel = "Paused due to error";
        countText = state.processed + " / " + state.total;
      }

      refs.statusText.textContent = statusLabel;
      refs.statusCount.textContent = countText;

      var ratio = 0;
      if (state.phase === "collecting" || state.phase === "scrolling") {
        ratio = Math.min(1, Math.max(0.08, state.collected / MAX_POSTS));
      } else if (state.total > 0) {
        ratio = Math.min(1, state.processed / state.total);
      }
      refs.progressFill.style.transform = "scaleX(" + ratio + ")";

      refs.savedVal.textContent = String(state.saved);
      refs.skippedVal.textContent = String(state.skipped);
      refs.failedVal.textContent = String(state.failed);
    }

    if (state.alertKind && state.alertTitle) {
      refs.alert.className = "alert alert-" + state.alertKind;
      refs.alertTitle.textContent = state.alertTitle;
      refs.alertDetail.textContent = state.alertDetail || "";
      refs.alertDetail.classList.toggle("hidden", !state.alertDetail);
      refs.alertSettingsBtn.classList.toggle("hidden", !state.showSettingsAction);
      refs.alert.classList.remove("hidden");
    } else {
      refs.alert.className = "alert hidden";
    }
  }

  function handleActionClick() {
    if (!activeRoute) {
      return;
    }

    if (activeSession && activeSession.isRunning()) {
      activeSession.stop();
      refs.statusText.textContent = "Stopping…";
      return;
    }

    var startScrollY = getCurrentScrollY();
    var resumedFromScroll = startScrollY > 0;

    clearAlert();
    if (!resumedFromScroll) {
      scanVisibleUrls();
    }

    sendRuntimeMessage({ type: "BULK_GET_CONFIG" }).then(function (cfg) {
      if (!cfg || !cfg.ok) {
        setAlert("error", "Extension unavailable", (cfg && cfg.error) || "Could not read Social Vault settings.", false);
        render();
        return;
      }

      if (!cfg.hasApiKey) {
        setAlert("info", "API key needed", "Add your VAULT_API_KEY in Extension Settings before bulk saving.", true);
        render();
        return;
      }

      if (!cfg.hasPermission) {
        setAlert(
          "info",
          "Site access needed",
          "Open Settings and click Save Settings or Test Connection to grant access to " + (cfg.vaultHost || cfg.vaultUrl) + ".",
          true
        );
        render();
        return;
      }

      var routeSnapshot = activeRoute;
      var initialUrls = resumedFromScroll
        ? extractor.extractPostUrls(routeSnapshot.platform, document, window.location.origin, {
            minDocumentTop: startScrollY
          })
        : state.detectedUrls.slice();
      if (!state.autoScroll && initialUrls.length === 0) {
        if (resumedFromScroll) {
          setAlert(
            "info",
            "No posts detected below your scroll position",
            "Scroll the feed or turn on Auto-scroll to load older posts.",
            false
          );
        } else {
          setAlert("info", "No posts detected yet", "Scroll the feed or turn on Auto-scroll to load posts first.", false);
        }
        render();
        return;
      }

      state.resumedFromScroll = resumedFromScroll;
      activeSession = runner.createBulkSession({
        maxPosts: MAX_POSTS,
        saveDelayMs: 150,
        scrollWaitMs: 900,
        maxStagnantRounds: 3,
        extractUrls: function (extractOpts) {
          return extractor.extractPostUrls(routeSnapshot.platform, document, window.location.origin, extractOpts);
        },
        scrollStep: function () {
          var stepPx = Math.max(Math.round((window.innerHeight || 800) * 0.85), 600);
          window.scrollBy({ top: stepPx, behavior: "auto" });
        },
        getScrollMetrics: function () {
          var docEl = document.documentElement;
          return {
            scrollY: window.scrollY || window.pageYOffset || 0,
            scrollHeight: docEl ? docEl.scrollHeight : 0
          };
        },
        saveUrl: function (postUrl) {
          return sendRuntimeMessage({
            type: "BULK_SAVE_BOOKMARK",
            url: postUrl,
            source: routeSnapshot.source
          });
        },
        onProgress: function (progress) {
          state.phase = progress.phase;
          state.collected = progress.collected;
          state.total = progress.total;
          state.processed = progress.processed;
          state.saved = progress.saved;
          state.skipped = progress.skipped;
          state.failed = progress.failed;
          state.limitReached = progress.limitReached;
          state.resumedFromScroll = Boolean(progress.resumedFromScroll);
          if (!resumedFromScroll && progress.collected > state.detectedUrls.length) {
            scanVisibleUrls();
          }
          render();
        }
      });

      activeSession
        .start({
          initialUrls: initialUrls,
          autoScroll: state.autoScroll,
          minDocumentTop: startScrollY
        })
        .then(function (summary) {
          activeSession = null;
          state.phase = summary.phase;
          state.collected = summary.collected;
          state.total = summary.total;
          state.processed = summary.processed;
          state.saved = summary.saved;
          state.skipped = summary.skipped;
          state.failed = summary.failed;
          state.limitReached = summary.limitReached;
          state.resumedFromScroll = Boolean(summary.resumedFromScroll);

          if (summary.fatalError) {
            var needsSettings =
              summary.fatalError.code === "NO_API_KEY" ||
              summary.fatalError.code === "UNAUTHORIZED" ||
              summary.fatalError.code === "PERMISSION_REQUIRED";
            setAlert("error", "Bulk save stopped", summary.fatalError.message, needsSettings);
            sendRuntimeMessage({
              type: "FLASH_BADGE",
              kind: "error",
              title: summary.fatalError.message
            });
          } else if (summary.total === 0) {
            setAlert("info", "No posts found", "No matching post links were detected on this page.", false);
          } else if (summary.failed > 0 && summary.saved === 0) {
            setAlert("error", "Could not save posts", summary.lastError || "All requests failed.", false);
            sendRuntimeMessage({
              type: "FLASH_BADGE",
              kind: "error",
              title: "Bulk save failed"
            });
          } else if (summary.saved === 0 && summary.skipped > 0) {
            sendRuntimeMessage({
              type: "FLASH_BADGE",
              kind: "duplicate",
              title: "All " + summary.skipped + " posts already in Social Vault"
            });
          } else if (summary.saved > 0) {
            sendRuntimeMessage({
              type: "FLASH_BADGE",
              kind: "success",
              title: "Saved " + summary.saved + " posts to Social Vault"
            });
          }

          render();
        })
        .catch(function (err) {
          activeSession = null;
          state.phase = "error";
          setAlert("error", "Bulk save failed", (err && err.message) || "Unexpected error.", false);
          render();
        });
    });
  }

  function mountPanel() {
    if (hostEl) {
      return;
    }

    var existing = document.getElementById(ROOT_ID);
    if (existing && existing.parentNode) {
      existing.parentNode.removeChild(existing);
    }

    hostEl = document.createElement("div");
    hostEl.id = ROOT_ID;
    shadow = hostEl.attachShadow({ mode: "open" });
    shadow.appendChild(buildStyles());

    var panel = document.createElement("section");
    panel.className = "panel";
    panel.setAttribute("aria-label", "Social Vault Bulk Fetch");

    var header = document.createElement("header");
    header.className = "header";

    var brand = document.createElement("div");
    brand.className = "brand";
    var mark = document.createElement("div");
    mark.className = "brand-mark";
    mark.appendChild(createSvgIcon("bookmark"));

    var titleGroup = document.createElement("div");
    titleGroup.className = "title-group";
    var title = document.createElement("h2");
    title.className = "title";
    title.textContent = "Social Vault";
    var routeLabel = document.createElement("p");
    routeLabel.className = "subtitle";
    titleGroup.appendChild(title);
    titleGroup.appendChild(routeLabel);

    brand.appendChild(mark);
    brand.appendChild(titleGroup);

    var headerRight = document.createElement("div");
    headerRight.className = "header-right";
    var countPill = document.createElement("span");
    countPill.className = "count-pill";
    var collapseBtn = document.createElement("button");
    collapseBtn.type = "button";
    collapseBtn.className = "icon-btn";
    collapseBtn.appendChild(createSvgIcon("chevron"));
    collapseBtn.addEventListener("click", function () {
      state.collapsed = !state.collapsed;
      render();
    });

    headerRight.appendChild(countPill);
    headerRight.appendChild(collapseBtn);
    header.appendChild(brand);
    header.appendChild(headerRight);

    var body = document.createElement("div");
    body.className = "body";

    var toggleRow = document.createElement("div");
    toggleRow.className = "toggle-row";
    var toggleCopy = document.createElement("div");
    toggleCopy.className = "toggle-copy";
    var toggleLabel = document.createElement("span");
    toggleLabel.className = "toggle-label";
    toggleLabel.id = "sv-autoscroll-label";
    toggleLabel.textContent = "Auto-scroll to load more";
    var toggleHint = document.createElement("span");
    toggleHint.className = "toggle-hint";
    toggleHint.textContent = "Max " + MAX_POSTS + " posts per session";
    toggleCopy.appendChild(toggleLabel);
    toggleCopy.appendChild(toggleHint);

    var switchBtn = document.createElement("button");
    switchBtn.type = "button";
    switchBtn.className = "switch";
    switchBtn.setAttribute("role", "switch");
    switchBtn.setAttribute("aria-checked", "true");
    switchBtn.setAttribute("aria-labelledby", "sv-autoscroll-label");
    var switchThumb = document.createElement("span");
    switchThumb.className = "switch-thumb";
    switchBtn.appendChild(switchThumb);
    switchBtn.addEventListener("click", function () {
      if (activeSession && activeSession.isRunning()) {
        return;
      }
      state.autoScroll = !state.autoScroll;
      render();
    });

    toggleRow.appendChild(toggleCopy);
    toggleRow.appendChild(switchBtn);

    var actionBtn = document.createElement("button");
    actionBtn.type = "button";
    actionBtn.className = "btn btn-primary";
    actionBtn.addEventListener("click", handleActionClick);

    var progressCard = document.createElement("div");
    progressCard.className = "progress-card hidden";
    progressCard.setAttribute("role", "status");
    progressCard.setAttribute("aria-live", "polite");
    progressCard.setAttribute("aria-atomic", "true");

    var statusLine = document.createElement("div");
    statusLine.className = "status-line";
    var statusText = document.createElement("span");
    statusText.className = "status-text";
    var statusCount = document.createElement("span");
    statusCount.className = "status-count";
    statusLine.appendChild(statusText);
    statusLine.appendChild(statusCount);

    var progressTrack = document.createElement("div");
    progressTrack.className = "progress-track";
    var progressFill = document.createElement("div");
    progressFill.className = "progress-fill";
    progressTrack.appendChild(progressFill);

    var metrics = document.createElement("div");
    metrics.className = "metrics";

    function createMetric(cls, label) {
      var box = document.createElement("div");
      box.className = "metric " + cls;
      var val = document.createElement("span");
      val.className = "metric-val";
      val.textContent = "0";
      var lbl = document.createElement("span");
      lbl.className = "metric-lbl";
      lbl.textContent = label;
      box.appendChild(val);
      box.appendChild(lbl);
      return { box: box, val: val };
    }

    var mSaved = createMetric("metric-saved", "Saved");
    var mSkipped = createMetric("metric-skipped", "Skipped");
    var mFailed = createMetric("metric-failed", "Failed");
    metrics.appendChild(mSaved.box);
    metrics.appendChild(mSkipped.box);
    metrics.appendChild(mFailed.box);

    var resumeNote = document.createElement("div");
    resumeNote.className = "resume-note hidden";
    resumeNote.appendChild(createSvgIcon("arrow-down"));
    var resumeNoteText = document.createElement("span");
    resumeNoteText.textContent = "Resuming from your scroll position — older posts only";
    resumeNote.appendChild(resumeNoteText);

    progressCard.appendChild(statusLine);
    progressCard.appendChild(resumeNote);
    progressCard.appendChild(progressTrack);
    progressCard.appendChild(metrics);

    var alert = document.createElement("div");
    alert.className = "alert hidden";
    alert.setAttribute("role", "alert");

    var alertHead = document.createElement("div");
    alertHead.className = "alert-head";
    alertHead.appendChild(createSvgIcon("alert"));
    var alertTextWrap = document.createElement("div");
    var alertTitle = document.createElement("span");
    alertTitle.className = "alert-title";
    var alertDetail = document.createElement("span");
    alertDetail.className = "alert-detail";
    alertTextWrap.appendChild(alertTitle);
    alertTextWrap.appendChild(alertDetail);
    alertHead.appendChild(alertTextWrap);

    var alertSettingsBtn = document.createElement("button");
    alertSettingsBtn.type = "button";
    alertSettingsBtn.className = "btn btn-secondary btn-sm hidden";
    alertSettingsBtn.textContent = "Open Settings";
    alertSettingsBtn.addEventListener("click", function () {
      sendRuntimeMessage({ type: "OPEN_OPTIONS" });
    });

    alert.appendChild(alertHead);
    alert.appendChild(alertSettingsBtn);

    body.appendChild(toggleRow);
    body.appendChild(actionBtn);
    body.appendChild(progressCard);
    body.appendChild(alert);

    panel.appendChild(header);
    panel.appendChild(body);
    shadow.appendChild(panel);

    refs = {
      panel: panel,
      routeLabel: routeLabel,
      countPill: countPill,
      collapseBtn: collapseBtn,
      body: body,
      switchBtn: switchBtn,
      actionBtn: actionBtn,
      progressCard: progressCard,
      statusText: statusText,
      statusCount: statusCount,
      resumeNote: resumeNote,
      progressFill: progressFill,
      savedVal: mSaved.val,
      skippedVal: mSkipped.val,
      failedVal: mFailed.val,
      alert: alert,
      alertTitle: alertTitle,
      alertDetail: alertDetail,
      alertSettingsBtn: alertSettingsBtn
    };

    var container = document.body || document.documentElement;
    container.appendChild(hostEl);

    requestAnimationFrame(function () {
      if (refs.panel) {
        refs.panel.setAttribute("data-mounted", "true");
      }
    });
  }

  function unmountPanel() {
    if (scanTimer !== null) {
      clearTimeout(scanTimer);
      scanTimer = null;
    }
    if (domObserver) {
      domObserver.disconnect();
      domObserver = null;
    }
    window.removeEventListener("scroll", scheduleScan);

    if (activeSession) {
      activeSession.stop();
      activeSession = null;
    }
    if (hostEl && hostEl.parentNode) {
      hostEl.parentNode.removeChild(hostEl);
    }
    hostEl = null;
    shadow = null;
    refs = {};
  }

  function resetSessionState() {
    state.detectedUrls = [];
    state.phase = "idle";
    state.collected = 0;
    state.total = 0;
    state.processed = 0;
    state.saved = 0;
    state.skipped = 0;
    state.failed = 0;
    state.limitReached = false;
    state.resumedFromScroll = false;
    clearAlert();
  }

  function startObservers() {
    if (domObserver) {
      domObserver.disconnect();
    }
    var target = document.body || document.documentElement;
    if (target && typeof MutationObserver !== "undefined") {
      domObserver = new MutationObserver(scheduleScan);
      domObserver.observe(target, { childList: true, subtree: true });
    }
    window.addEventListener("scroll", scheduleScan, { passive: true });
  }

  function syncRoute(force) {
    var href = window.location.href;
    if (!force && href === currentHref) {
      return;
    }
    currentHref = href;

    var nextRoute = extractor.detectPage(window.location);
    if (!nextRoute) {
      activeRoute = null;
      resetSessionState();
      unmountPanel();
      return;
    }

    var changedRoute =
      !activeRoute ||
      activeRoute.platform !== nextRoute.platform ||
      activeRoute.pageKind !== nextRoute.pageKind;

    activeRoute = nextRoute;
    if (changedRoute) {
      resetSessionState();
    }

    mountPanel();
    startObservers();
    scanVisibleUrls();
    render();
  }

  window.addEventListener("popstate", function () {
    syncRoute(true);
  });
  window.addEventListener("hashchange", function () {
    syncRoute(true);
  });

  routeInterval = setInterval(function () {
    syncRoute(false);
  }, 500);

  window.addEventListener("beforeunload", function () {
    if (routeInterval !== null) {
      clearInterval(routeInterval);
      routeInterval = null;
    }
    unmountPanel();
  });

  syncRoute(true);
})();
