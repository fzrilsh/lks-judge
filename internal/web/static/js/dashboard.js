// Participant dashboard live client. Opens the WebSocket at /ws (the
// participant_session cookie rides along, so the connection is authenticated
// and receives the full event set), reconnects with simple backoff, and mutates
// the page in place: active module, form-open overlay, public file list, and
// the countdown. uploader.js handles the actual submission upload.
(function () {
  var backoff = 1000; // ms, doubles up to 15s

  function fmtTime(sec) {
    if (sec == null || sec < 0) sec = 0;
    var h = Math.floor(sec / 3600);
    var m = Math.floor((sec % 3600) / 60);
    var s = sec % 60;
    var p = function (n) { return (n < 10 ? "0" : "") + n; };
    return p(h) + ":" + p(m) + ":" + p(s);
  }

  function onModuleChanged() {
    // Simplest correct path: reload so the server re-renders the active module
    // and this participant's existing submission for it.
    window.location.reload();
  }

  function onFormOpened(payload) {
    var layer = document.getElementById("sensor-layer");
    if (!layer) return;
    layer.classList.toggle("hidden", !!(payload && payload.status));
  }

  function onFileListUpdated(payload) {
    if (!payload || !payload.id) return;
    var list = document.getElementById("official-files-list");
    if (!list) return;
    var existing = list.querySelector('[data-file-id="' + payload.id + '"]');
    if (!payload.is_public) {
      if (existing) existing.remove();
      return;
    }
    if (existing) {
      existing.querySelector(".file-name").textContent = payload.name;
      return;
    }
    var node = document.createElement("div");
    node.setAttribute("data-file-id", payload.id);
    node.className = "bg-surface-container-low p-5 rounded-xl flex justify-between items-center";
    node.innerHTML =
      '<div class="flex items-center gap-4">' +
        '<div class="w-12 h-12 bg-surface-container-high rounded-lg flex items-center justify-center">' +
          '<span class="material-symbols-outlined">description</span>' +
        '</div>' +
        '<div><p class="file-name font-bold text-on-surface"></p></div>' +
      '</div>' +
      '<a href="/files/' + payload.id + '/download" class="p-2 hover:bg-primary/10 text-primary rounded-lg">' +
        '<span class="material-symbols-outlined">download</span>' +
      '</a>';
    node.querySelector(".file-name").textContent = payload.name;
    var empty = list.querySelector(".empty-placeholder");
    if (empty) empty.remove();
    list.appendChild(node);
  }

  var lastSeconds = null; // last value from the server; a local 1s ticker fills the gaps
  var localTimer = null;

  function paint() {
    var el = document.getElementById("countdown");
    if (el) el.textContent = fmtTime(lastSeconds);
  }

  function onCountdownTick(payload) {
    if (!payload) return;
    lastSeconds = payload.seconds;
    paint();
    // Only a running countdown advances. Paused/waiting/finished freeze the
    // clock, so clear any local ticker and do not decrement.
    if (localTimer) { clearInterval(localTimer); localTimer = null; }
    if (payload.status !== "running") return;
    // Server pushes every 5s; decrement locally each second so the clock is smooth.
    localTimer = setInterval(function () {
      if (lastSeconds == null) return;
      if (lastSeconds > 0) lastSeconds -= 1;
      paint();
    }, 1000);
  }

  function handle(msg) {
    switch (msg.event) {
      case "ModuleChanged": onModuleChanged(); break;
      case "FormOpened": onFormOpened(msg.payload); break;
      case "FileListUpdated": onFileListUpdated(msg.payload); break;
      case "CountdownTick": onCountdownTick(msg.payload); break;
    }
  }

  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var ws = new WebSocket(proto + "//" + location.host + "/ws");
    ws.onopen = function () { backoff = 1000; };
    ws.onmessage = function (e) {
      try { handle(JSON.parse(e.data)); } catch (_) { /* ignore malformed */ }
    };
    ws.onclose = function () {
      setTimeout(connect, backoff);
      backoff = Math.min(backoff * 2, 15000);
    };
    ws.onerror = function () { ws.close(); };
  }

  connect();
})();
