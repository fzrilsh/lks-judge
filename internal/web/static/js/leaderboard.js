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
      body.innerHTML = '<tr><td colspan="99" class="text-center px-4 py-8 text-on-surface-variant">Belum ada nilai.</td></tr>';
      return;
    }
    body.innerHTML = entries.map(function (e) {
      var rank = e.rank;
      var tint = rank === 1 ? "bg-amber-500/10 hover:bg-amber-500/20"
        : rank === 2 ? "bg-slate-400/10 hover:bg-slate-400/20"
        : rank === 3 ? "bg-orange-500/10 hover:bg-orange-500/20"
        : "hover:bg-surface-container-low";

      var circle = rank === 1 ? "bg-gradient-to-br from-amber-300 to-amber-500"
        : rank === 2 ? "bg-gradient-to-br from-slate-300 to-slate-500"
        : rank === 3 ? "bg-gradient-to-br from-orange-400 to-orange-600"
        : "bg-surface-container-high text-on-surface";
      var seat = e.pc_number != null ? ("0" + e.pc_number).slice(-2) : "-";

      var medal = rank === 1 ? '<span title="Gold Medal" class="text-xl">🥇</span>'
        : rank === 2 ? '<span title="Silver Medal" class="text-xl">🥈</span>'
        : rank === 3 ? '<span title="Bronze Medal" class="text-xl">🥉</span>'
        : e.award === "Medallion for Excellence" ? '<span title="Medallion for Excellence" class="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-bold bg-blue-100 text-blue-800 border border-blue-200 ml-2">Medallion</span>'
        : "";

      var totalColor = rank === 1 ? "text-amber-500 drop-shadow-sm" : "text-primary";

      var moduleCells = modules.map(function (m) {
        var val = (Number((e.scores && e.scores["" + m.id]) || 0)).toFixed(2);
        return '<td class="py-5 px-2 w-[80px] text-center"><div class="w-14 h-9 mx-auto flex items-center justify-center rounded-lg bg-surface-container border border-surface-container-high font-bold text-on-surface text-sm group-hover:bg-surface-container-high transition-colors">' + val + '</div></td>';
      }).join("");

      return '<tr class="group transition-all duration-200 ease-in-out ' + tint + '">' +
        '<td class="py-5 px-6 font-bold text-on-surface">' +
          '<div class="flex items-center gap-3">' +
            '<span class="text-lg w-6 text-center text-outline font-black">#' + rank + '</span>' +
            '<span class="w-10 h-10 rounded-full flex items-center justify-center font-bold text-white shadow-sm ' + circle + '">' + seat + '</span>' +
          '</div>' +
        '</td>' +
        '<td class="py-5 px-6">' +
          '<div class="flex flex-col">' +
            '<div class="flex items-center gap-2"><p class="font-extrabold text-on-surface text-base">' + esc(e.name) + '</p>' + medal + '</div>' +
            '<p class="text-xs font-medium text-outline mt-0.5">' + esc(e.school) + '</p>' +
          '</div>' +
        '</td>' +
        moduleCells +
        '<td class="py-5 px-6 text-right"><div class="flex flex-col items-end"><span class="text-2xl font-black font-manrope ' + totalColor + '">' + e.wsi + '</span></div></td>' +
        '</tr>';
    }).join("");
  }

  function refresh() {
    fetch("/leaderboard.json", { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (d) { render(d.entries || [], d.modules || []); })
      .catch(function () { /* keep last render */ });
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
