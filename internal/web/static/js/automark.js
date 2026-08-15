// Automark console client.
//  - Tabs: raw JSON textarea (source of truth) <-> visual builder.
//  - Run: POST /jury/automark/run, then listen on the shared /ws for
//    AutomarkResult (one per participant) and AutomarkDone.
// The builder itself lives in automark_builder.js (window.AutomarkBuilder); this
// file only wires tabs, the Load-example button, run, and the live result rows.
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
      if (name === "builder" && window.AutomarkBuilder) window.AutomarkBuilder.render();
    });
  });

  // ---- load example ------------------------------------------------------
  // Fills the textarea from the embedded example so an empty config is a
  // starting point, not a blank wall. Only overwrites on an empty/confirm.
  var loadBtn = document.getElementById("am-load-example");
  var exampleEl = document.getElementById("am-example");
  if (loadBtn && exampleEl) {
    loadBtn.addEventListener("click", function () {
      if (ta.value.trim() && !confirm("Replace the current config with the example?")) return;
      ta.value = exampleEl.textContent.trim();
    });
  }

  // ---- run + live results -------------------------------------------------
  var runBtn = document.getElementById("am-run");
  var statusEl = document.getElementById("am-status");
  var resultsEl = document.getElementById("am-results");
  var expected = 0, got = 0;
  var progress = {}; // pc_number -> live progress row element, replaced by the final row
  var totals = {};   // participant_id -> raw total_score, collected for "apply to module"

  // progressRow renders a per-participant live bar that fills as assertions
  // finish, before the full result arrives. Keyed by pc_number in `progress`.
  function progressRow(p) {
    var el = progress[p.pc_number];
    if (!el) {
      el = document.createElement("div");
      el.className = "flex items-center gap-3 rounded-xl border border-outline-variant border-dashed px-3 py-2";
      progress[p.pc_number] = el;
      resultsEl.appendChild(el);
    }
    var frac = p.total ? Math.round((p.done / p.total) * 100) : 0;
    el.innerHTML = "";
    var pc = document.createElement("span");
    pc.className = "w-10 font-bold text-on-surface";
    pc.textContent = p.pc_number;
    var mid = document.createElement("div");
    mid.className = "flex-1 min-w-0";
    var line = document.createElement("div");
    line.className = "flex justify-between text-body-small";
    var title = document.createElement("span");
    title.className = "truncate text-on-surface-variant";
    title.textContent = (p.passed ? "✓ " : "✗ ") + p.title;
    var cnt = document.createElement("span");
    cnt.className = "font-bold text-on-surface";
    cnt.textContent = p.done + " / " + p.total;
    line.appendChild(title); line.appendChild(cnt);
    var track = document.createElement("div");
    track.className = "h-2 mt-1 rounded-full bg-surface-container-high overflow-hidden";
    var fill = document.createElement("div");
    fill.className = "h-full bg-primary"; fill.style.width = frac + "%";
    track.appendChild(fill);
    mid.appendChild(line); mid.appendChild(track);
    var pctEl = document.createElement("span");
    pctEl.className = "w-12 text-right font-black text-on-surface-variant";
    pctEl.textContent = frac + "%";
    el.appendChild(pc); el.appendChild(mid); el.appendChild(pctEl);
  }

  function onProgress(p) { progressRow(p); }

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
    if (res.participant_id) totals[res.participant_id] = res.total_score;
    var live = progress[res.pc_number];
    if (live) { live.outerHTML = row(res); delete progress[res.pc_number]; }
    else resultsEl.insertAdjacentHTML("beforeend", row(res));
    statusEl.textContent = "Running... " + got + (expected ? " / " + expected : "") + " done";
  }
  function onDone(payload) {
    statusEl.textContent = "Done. " + (payload && payload.count != null ? payload.count : got) + " participant(s) marked.";
    if (runBtn) runBtn.disabled = false;
    var apply = document.getElementById("am-apply");
    if (apply && Object.keys(totals).length) apply.classList.remove("hidden");
  }

  // ---- apply raw totals to a module --------------------------------------
  // Two buttons (replace / add) POST the collected per-participant totals to a
  // chosen module. A confirm names the module first so a wrong pick is caught.
  var applyModule = document.getElementById("am-apply-module");
  function applyScores(mode) {
    if (!applyModule) return;
    var opt = applyModule.options[applyModule.selectedIndex];
    var name = opt ? opt.textContent : "";
    var verb = mode === "add" ? "add these totals onto" : "replace scores in";
    if (!confirm("Confirm module: " + verb + " \"" + name + "\"?\n\nThis writes " + Object.keys(totals).length + " participant total(s).")) return;
    var scores = Object.keys(totals).map(function (pid) {
      return { participant_id: Number(pid), total: totals[pid] };
    });
    fetch("/jury/automark/apply", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Requested-With": "fetch" },
      body: JSON.stringify({ module_id: Number(applyModule.value), mode: mode, scores: scores }),
    }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t || "http " + r.status); });
      return r.json();
    }).then(function (d) {
      var msg = "Applied " + d.applied + " score(s) to \"" + name + "\" (" + mode + ").";
      statusEl.textContent = msg;
      applyFlash(msg, true);
    }).catch(function (e) { applyFlash("Apply error: " + e.message, false); });
  }
  // applyFlash shows a coloured chip inside the apply panel so a successful
  // write is not silent (the top status line alone is easy to miss).
  function applyFlash(msg, ok) {
    var apply = document.getElementById("am-apply");
    if (!apply) return;
    var el = document.getElementById("am-apply-flash");
    if (!el) {
      el = document.createElement("p");
      el.id = "am-apply-flash";
      el.className = "text-body-medium rounded-lg px-3 py-2";
      apply.appendChild(el);
    }
    el.className = "text-body-medium rounded-lg px-3 py-2 " +
      (ok ? "bg-secondary-container text-on-secondary-container" : "bg-error-container text-on-error-container");
    el.textContent = msg;
  }
  var replaceBtn = document.getElementById("am-apply-replace");
  var addBtn = document.getElementById("am-apply-add");
  if (replaceBtn) replaceBtn.addEventListener("click", function () { applyScores("replace"); });
  if (addBtn) addBtn.addEventListener("click", function () { applyScores("add"); });

  if (runBtn) {
    runBtn.addEventListener("click", function () {
      runBtn.disabled = true;
      got = 0; expected = 0;
      progress = {};
      totals = {};
      var apply = document.getElementById("am-apply");
      if (apply) apply.classList.add("hidden");
      var flash = document.getElementById("am-apply-flash");
      if (flash) flash.remove();
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
        else if (msg.event === "AutomarkProgress") onProgress(msg.payload);
        else if (msg.event === "AutomarkDone") onDone(msg.payload);
      } catch (_) {}
    };
    ws.onclose = function () { setTimeout(connect, 2000); };
    ws.onerror = function () { ws.close(); };
  }
  connect();
})();
