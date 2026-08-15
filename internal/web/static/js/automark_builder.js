// Automark visual form builder. Reads #am-config (the JSON textarea, the source
// of truth), renders editable fields into #am-pane-builder, and writes back to
// the textarea on every change so switching tabs never loses edits.
// Exposes exactly one global: window.AutomarkBuilder = { render, validate }.
// automark.js calls AutomarkBuilder.render() on tab switch.
(function () {
  var ta = document.getElementById("am-config");
  var host = document.getElementById("am-pane-builder");
  if (!ta || !host) return;

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : s;
    return d.innerHTML;
  }

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

  function render() {
    var cfg = parse();
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
    sec.innerHTML = '<h3 class="text-title-medium text-on-surface">Base + Auth (global)</h3>';
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

  // validate() returns a count of inline problems; wired in a later task. For
  // now the server is the gate, so it reports none.
  function validate() { return 0; }

  window.AutomarkBuilder = { render: render, validate: validate };
})();
