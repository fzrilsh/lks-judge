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
