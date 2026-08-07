// Public leaderboard client. Fetches /leaderboard.json for the ranked rows and
// re-renders on the ScoreUpdated WS event (spec: WS replaces polling). Anonymous
// WS clients already receive ScoreUpdated; no auth cookie needed.
(function () {
  var backoff = 1000;
  var AWARD = { Gold: "🥇", Silver: "🥈", Bronze: "🥉", "Medallion for Excellence": "🎖" };

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : s;
    return d.innerHTML;
  }

  function render(entries) {
    var body = document.getElementById("leaderboard-body");
    if (!body) return;
    if (!entries.length) {
      body.innerHTML = '<tr><td colspan="4" class="text-center px-4 py-8 text-on-surface-variant">Belum ada nilai.</td></tr>';
      return;
    }
    body.innerHTML = entries.map(function (e) {
      var pc = e.pc_number != null ? ("0" + e.pc_number).slice(-2) + " " : "";
      var award = e.award ? (AWARD[e.award] || "") + " " + esc(e.award) : "";
      return '<tr class="border-t border-outline-variant">' +
        '<td class="px-4 py-3">' + e.rank + '</td>' +
        '<td class="px-4 py-3">' + pc + esc(e.name) +
          '<div class="text-label-small text-on-surface-variant">' + esc(e.school) + '</div></td>' +
        '<td class="px-4 py-3 text-center font-manrope">' + e.wsi + '</td>' +
        '<td class="px-4 py-3 text-center">' + award + '</td>' +
        '</tr>';
    }).join("");
  }

  function refresh() {
    fetch("/leaderboard.json", { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (d) { render(d.entries || []); })
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
