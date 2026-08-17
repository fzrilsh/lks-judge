// Public leaderboard client. Fetches /leaderboard.json for the ranked rows and
// re-renders on the ScoreUpdated WS event (spec: WS replaces polling). Anonymous
// WS clients already receive ScoreUpdated; no auth cookie needed.
(function () {
  var backoff = 1000;

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : s;
    return d.innerHTML;
  }

  function renderHead(modules) {
    var head = document.getElementById("leaderboard-head");
    if (!head) return;
    head.querySelectorAll(".js-module-col").forEach(function (n) { n.remove(); });
    modules.forEach(function (m) {
      var th = document.createElement("th");
      th.className = "js-module-col py-5 px-2 w-[80px] text-xs uppercase tracking-wider font-bold text-outline text-center";
      th.setAttribute("title", esc(m.name));
      th.innerHTML = esc(m.name);
      head.insertBefore(th, head.lastElementChild);
    });
  }

  function render(entries, modules) {
    renderHead(modules);
    var body = document.getElementById("leaderboard-body");
    if (!body) return;
    if (!entries.length) {
      body.innerHTML = '<tr><td colspan="99" class="text-center px-4 py-8 text-on-surface-variant">No scores yet.</td></tr>';
      return;
    }
    body.innerHTML = entries.map(function (e) {
      var rank = e.rank;
      var tint = rank <= 3 ? "bg-surface-container-low/60 hover:bg-surface-container-low"
        : "hover:bg-surface-container-low";

      // Rank cell: top 3 get a colored medal icon; a Medallion for Excellence
      // gets the military_tech icon (same left slot, not beside the name); the
      // rest a plain muted rank number.
      var medalColor = rank === 1 ? "text-amber-500"
        : rank === 2 ? "text-slate-400"
        : rank === 3 ? "text-orange-500" : "";
      var rankCell;
      if (rank <= 3) {
        rankCell = '<span class="material-symbols-outlined text-3xl ' + medalColor + '" aria-hidden="true">workspace_premium</span>' +
          '<span class="text-sm font-black ' + medalColor + '">' + rank + '</span>';
      } else if (e.award === "Medallion for Excellence") {
        rankCell = '<span class="material-symbols-outlined text-2xl text-primary" title="Medallion for Excellence" aria-hidden="true">military_tech</span>' +
          '<span class="text-lg font-bold text-primary tabular-nums">' + rank + '</span>';
      } else {
        rankCell = '<span class="text-lg font-bold text-on-surface-variant tabular-nums">' + rank + '</span>';
      }

      var totalColor = rank === 1 ? "text-amber-500 drop-shadow-sm" : "text-primary";

      var moduleCells = modules.map(function (m) {
        var val = (Number((e.scores && e.scores["" + m.id]) || 0)).toFixed(2);
        return '<td class="py-5 px-2 w-[80px] text-center"><div class="w-14 h-9 mx-auto flex items-center justify-center rounded-lg bg-surface-container border border-surface-container-high font-bold text-on-surface text-sm group-hover:bg-surface-container-high transition-colors">' + val + '</div></td>';
      }).join("");

      return '<tr class="group transition-colors ' + tint + '">' +
        '<td class="py-4 px-6">' +
          '<div class="flex items-center gap-2 w-14">' + rankCell + '</div>' +
        '</td>' +
        '<td class="py-4 px-6">' +
          '<span class="font-bold text-on-surface text-base">' + esc(e.name) + '</span>' +
          '<p class="text-xs font-medium text-on-surface-variant mt-0.5">' + esc(e.school) + '</p>' +
        '</td>' +
        moduleCells +
        '<td class="py-4 px-6 text-center"><span class="text-2xl font-black font-manrope ' + totalColor + '">' + e.wsi + '</span></td>' +
        '</tr>';
    }).join("");
  }

  var loaded = false; // once we have painted real data, keep it on later failures

  function refresh() {
    fetch("/leaderboard.json", { cache: "no-store" })
      .then(function (r) {
        if (!r.ok) throw new Error("http " + r.status);
        return r.json();
      })
      .then(function (d) { loaded = true; render(d.entries || [], d.modules || []); })
      .catch(function () {
        // First load failed: replace the "Loading..." row with a retry affordance
        // instead of hanging forever. A later failure keeps the last good render.
        if (loaded) return;
        var body = document.getElementById("leaderboard-body");
        if (!body) return;
        body.innerHTML =
          '<tr><td colspan="99" class="text-center py-8 text-on-surface-variant">' +
            'Failed to load data. ' +
            '<button type="button" id="lb-retry" class="chip chip-neutral">Try again</button>' +
          '</td></tr>';
        var btn = document.getElementById("lb-retry");
        if (btn) btn.addEventListener("click", refresh);
      });
  }

  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var ws = new WebSocket(proto + "//" + location.host + "/ws");
    ws.onopen = function () { backoff = 1000; };
    ws.onmessage = function (e) {
      try {
        var msg = JSON.parse(e.data);
        if (msg.event === "ScoreUpdated") refresh();
      } catch (_) {}
    };
    ws.onclose = function () {
      setTimeout(connect, backoff);
      backoff = Math.min(backoff * 2, 15000);
    };
    ws.onerror = function () { ws.close(); };
  }

  refresh();
  connect();
})();
