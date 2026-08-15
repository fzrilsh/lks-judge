// Automark console client.
//  - Tabs: raw JSON textarea (source of truth) <-> visual builder.
//  - Run: POST /jury/automark/run, then listen on the shared /ws for
//    AutomarkResult (one per participant) and AutomarkDone.
// The builder reads the textarea, renders editable fields, and writes back to
// the textarea on every change so the JSON stays authoritative for submit.
(function () {
  var ta = document.getElementById("am-config");
  var pane = { json: document.getElementById("am-pane-json"), builder: document.getElementById("am-pane-builder") };
  var tabs = document.querySelectorAll(".am-tab");
  if (!ta) return;

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : s;
    return d.innerHTML;
  }

  // ---- tabs ---------------------------------------------------------------
  tabs.forEach(function (t) {
    t.addEventListener("click", function () {
      var name = t.dataset.tab;
      tabs.forEach(function (x) {
        var on = x === t;
        x.classList.toggle("am-tab-active", on);
        x.classList.toggle("border-primary", on);
        x.classList.toggle("text-primary", on);
        x.classList.toggle("border-transparent", !on);
        x.classList.toggle("text-on-surface-variant", !on);
      });
      pane.json.classList.toggle("hidden", name !== "json");
      pane.builder.classList.toggle("hidden", name !== "builder");
      if (name === "builder") renderBuilder();
    });
  });

  // ---- builder ------------------------------------------------------------
  function parse() {
    try { return JSON.parse(ta.value || "{}"); } catch (e) { return null; }
  }
  function sync(cfg) { ta.value = JSON.stringify(cfg, null, 2); }

  function field(label, value, oninput) {
    var wrap = document.createElement("label");
    wrap.className = "block";
    wrap.innerHTML = '<span class="text-label-medium text-on-surface-variant">' + esc(label) + "</span>";
    var inp = document.createElement("input");
    inp.className = "input w-full mt-1";
    inp.value = value == null ? "" : value;
    inp.addEventListener("input", function () { oninput(inp.value); });
    wrap.appendChild(inp);
    return wrap;
  }

  function renderBuilder() {
    var cfg = parse();
    var host = pane.builder;
    host.innerHTML = "";
    if (!cfg) {
      host.innerHTML = '<div class="bg-error-container text-on-error-container rounded-xl px-4 py-3 text-body-medium">JSON is invalid. Fix it on the JSON tab to use the builder.</div>';
      return;
    }
    cfg.base = cfg.base || {}; cfg.auth = cfg.auth || {}; cfg.auth.login = cfg.auth.login || {};
    cfg.groups = cfg.groups || [];

    // Base + global auth (single credential set, shared across participants).
    var sec = document.createElement("div");
    sec.className = "space-y-3";
    sec.innerHTML = '<h3 class="text-title-small text-on-surface">Base + Auth (global)</h3>';
    sec.appendChild(field("Scheme", cfg.base.scheme, function (v) { cfg.base.scheme = v; sync(cfg); }));
    sec.appendChild(field("Port", cfg.base.port, function (v) { cfg.base.port = Number(v) || 0; sync(cfg); }));
    sec.appendChild(field("Base path", cfg.base.path, function (v) { cfg.base.path = v; sync(cfg); }));
    sec.appendChild(field("Login endpoint", cfg.auth.login.endpoint, function (v) { cfg.auth.login.endpoint = v; sync(cfg); }));
    sec.appendChild(field("Login method", cfg.auth.login.method, function (v) { cfg.auth.login.method = v; sync(cfg); }));
    sec.appendChild(field("Token path", cfg.auth.tokenPath, function (v) { cfg.auth.tokenPath = v; sync(cfg); }));
    var body = cfg.auth.login.body || {};
    sec.appendChild(field("Login body (JSON)", JSON.stringify(body), function (v) {
      try { cfg.auth.login.body = JSON.parse(v); sync(cfg); } catch (_) {}
    }));
    host.appendChild(sec);

    // Groups + assertion counts. Deep per-assertion editing stays on the JSON
    // tab; the builder gives structure + the high-churn fields.
    cfg.groups.forEach(function (g, gi) {
      var card = document.createElement("div");
      card.className = "border border-outline-variant rounded-xl p-3 space-y-2";
      card.innerHTML = '<h4 class="text-label-large text-on-surface">Group ' + (gi + 1) + "</h4>";
      card.appendChild(field("Group name", g.group_name, function (v) { g.group_name = v; sync(cfg); }));
      card.appendChild(field("Group id", g.group_id, function (v) { g.group_id = v; sync(cfg); }));
      var meta = document.createElement("p");
      meta.className = "text-body-small text-on-surface-variant";
      meta.textContent = (g.assertions || []).length + " assertions (edit details on the JSON tab)";
      card.appendChild(meta);
      host.appendChild(card);
    });
  }

  // ---- run + live results -------------------------------------------------
  var runBtn = document.getElementById("am-run");
  var statusEl = document.getElementById("am-status");
  var resultsEl = document.getElementById("am-results");
  var expected = 0, got = 0;

  function row(res) {
    var pct = Math.round(res.pct);
    var bar = pct >= 75 ? "bg-primary" : pct >= 50 ? "bg-warning" : "bg-error";
    return '<div class="flex items-center gap-3 rounded-xl border border-outline-variant px-3 py-2">' +
      '<span class="w-10 font-bold text-on-surface">' + esc(res.pc_number) + '</span>' +
      '<div class="flex-1 min-w-0"><div class="flex justify-between text-body-small"><span class="truncate text-on-surface-variant">' + esc(res.host) + '</span>' +
      '<span class="font-bold text-on-surface">' + res.total_score.toFixed(2) + " / " + res.total_max.toFixed(2) + '</span></div>' +
      '<div class="h-2 mt-1 rounded-full bg-surface-container-high overflow-hidden"><div class="h-full ' + bar + '" style="width:' + pct + '%"></div></div></div>' +
      '<span class="w-12 text-right font-black text-on-surface">' + pct + '%</span></div>';
  }

  function onResult(res) {
    got++;
    resultsEl.insertAdjacentHTML("beforeend", row(res));
    statusEl.textContent = "Running... " + got + (expected ? " / " + expected : "") + " done";
  }
  function onDone(payload) {
    statusEl.textContent = "Done. " + (payload && payload.count != null ? payload.count : got) + " participant(s) marked.";
    if (runBtn) runBtn.disabled = false;
  }

  if (runBtn) {
    runBtn.addEventListener("click", function () {
      runBtn.disabled = true;
      got = 0; expected = 0;
      resultsEl.innerHTML = "";
      statusEl.textContent = "Starting...";
      fetch("/jury/automark/run", { method: "POST", headers: { "X-Requested-With": "fetch" } })
        .then(function (r) {
          if (r.status === 409) throw new Error("A run is already in progress.");
          if (!r.ok) return r.text().then(function (t) { throw new Error(t || "http " + r.status); });
          return r.json();
        })
        .then(function (d) { expected = d.targets || 0; statusEl.textContent = "Running " + expected + " participant(s)..."; })
        .catch(function (e) { statusEl.textContent = "Error: " + e.message; runBtn.disabled = false; });
    });
  }

  // Reuse the shared hub socket; jury connections receive automark events.
  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var ws = new WebSocket(proto + "//" + location.host + "/ws");
    ws.onmessage = function (e) {
      try {
        var msg = JSON.parse(e.data);
        if (msg.event === "AutomarkResult") onResult(msg.payload);
        else if (msg.event === "AutomarkDone") onDone(msg.payload);
      } catch (_) {}
    };
    ws.onclose = function () { setTimeout(connect, 2000); };
    ws.onerror = function () { ws.close(); };
  }
  connect();
})();
