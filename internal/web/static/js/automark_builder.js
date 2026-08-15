// Automark visual form builder. Reads #am-config (the JSON textarea, the source
// of truth), renders editable fields into #am-pane-builder, and writes back to
// the textarea on every change so switching tabs never loses edits.
// Exposes exactly one global: window.AutomarkBuilder = { render, validate }.
// automark.js calls AutomarkBuilder.render() on tab switch. Full re-render, no
// diffing, matching dashboard.js: static chrome, user data via value/textContent.
(function () {
  var ta = document.getElementById("am-config");
  var host = document.getElementById("am-pane-builder");
  if (!ta || !host) return;

  var METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"];

  function parse() {
    try { return JSON.parse(ta.value || "{}"); } catch (e) { return null; }
  }
  function sync(cfg) { ta.value = JSON.stringify(cfg, null, 2); }

  // ---- input primitives ---------------------------------------------------
  // Each builds a <label> wrapping the control, applies the value via .value
  // (never attribute interpolation), and calls back on input/change.
  function labelled(text, control) {
    var wrap = document.createElement("label");
    wrap.className = "block";
    var span = document.createElement("span");
    span.className = "text-label-medium text-on-surface-variant";
    span.textContent = text;
    wrap.appendChild(span);
    control.classList.add("mt-1");
    wrap.appendChild(control);
    return wrap;
  }

  function textField(label, value, oninput) {
    var inp = document.createElement("input");
    inp.className = "input w-full";
    inp.value = value == null ? "" : value;
    inp.addEventListener("input", function () { oninput(inp.value, inp); });
    return labelled(label, inp);
  }

  function numField(label, value, oninput) {
    var inp = document.createElement("input");
    inp.type = "number";
    inp.className = "input w-full";
    inp.value = value == null ? "" : value;
    inp.addEventListener("input", function () { oninput(inp.value, inp); });
    return labelled(label, inp);
  }
  // PLACEHOLDER-A

  function selectField(label, value, options, onchange) {
    var sel = document.createElement("select");
    sel.className = "input w-full";
    options.forEach(function (o) {
      var opt = document.createElement("option");
      opt.value = o; opt.textContent = o;
      sel.appendChild(opt);
    });
    sel.value = value || options[0];
    sel.addEventListener("change", function () { onchange(sel.value); });
    return labelled(label, sel);
  }

  function jsonField(label, obj, onvalid) {
    var area = document.createElement("textarea");
    area.className = "input font-mono w-full text-body-small";
    area.rows = 3;
    area.value = obj ? JSON.stringify(obj, null, 2) : "";
    area.addEventListener("input", function () {
      area.classList.remove("input-error");
      if (!area.value.trim()) { onvalid(undefined); return; }
      try { onvalid(JSON.parse(area.value)); }
      catch (_) { area.classList.add("input-error"); }
    });
    return labelled(label, area);
  }

  function checkboxRow(label, checked, onchange) {
    var row = document.createElement("label");
    row.className = "flex items-center gap-2 text-body-medium text-on-surface";
    var cb = document.createElement("input");
    cb.type = "checkbox";
    cb.checked = !!checked;
    cb.addEventListener("change", function () { onchange(cb.checked); });
    row.appendChild(cb);
    var span = document.createElement("span");
    span.textContent = label;
    row.appendChild(span);
    return { row: row, cb: cb };
  }

  function iconBtn(icon, title, cls, onclick) {
    var b = document.createElement("button");
    b.type = "button";
    b.className = cls;
    b.title = title;
    b.setAttribute("aria-label", title);
    var s = document.createElement("span");
    s.className = "material-symbols-outlined text-lg";
    s.setAttribute("aria-hidden", "true");
    s.textContent = icon;
    b.appendChild(s);
    b.addEventListener("click", onclick);
    return b;
  }

  function textBtn(label, cls, onclick) {
    var b = document.createElement("button");
    b.type = "button";
    b.className = cls;
    b.textContent = label;
    b.addEventListener("click", onclick);
    return b;
  }

  function hint(text) {
    var p = document.createElement("p");
    p.className = "text-body-small text-on-surface-variant";
    p.textContent = text;
    return p;
  }

  function chip(kind, text) {
    var span = document.createElement("span");
    span.className = "chip-" + kind;
    span.textContent = text;
    return span;
  }

  function heading(level, text) {
    var h = document.createElement(level);
    h.className = level === "h3" ? "text-title-medium text-on-surface" : "text-label-large text-on-surface";
    h.textContent = text;
    return h;
  }

  function move(arr, i, delta) {
    var j = i + delta;
    if (j < 0 || j >= arr.length) return false;
    var tmp = arr[i]; arr[i] = arr[j]; arr[j] = tmp;
    return true;
  }
  // PLACEHOLDER-B

  // ---- sections -----------------------------------------------------------

  // 1. Server: scheme/port/base path. The participant IP is filled per target.
  function serverSection(cfg) {
    var sec = document.createElement("div");
    sec.className = "space-y-3";
    sec.appendChild(heading("h3", "Server"));
    sec.appendChild(selectField("Scheme", cfg.base.scheme || "http", ["http", "https"], function (v) { cfg.base.scheme = v; sync(cfg); }));
    sec.appendChild(numField("Port", cfg.base.port, function (v) { cfg.base.port = Number(v) || 0; sync(cfg); }));
    sec.appendChild(textField("Base path", cfg.base.path, function (v) { cfg.base.path = v; sync(cfg); }));
    sec.appendChild(hint("The participant IP is filled in per target from the address captured at their login."));
    return sec;
  }

  // 2. Authentication (global): one credential set shared by every participant.
  function authSection(cfg) {
    var sec = document.createElement("div");
    sec.className = "space-y-3";
    sec.appendChild(heading("h3", "Authentication (global)"));
    sec.appendChild(selectField("Login method", cfg.auth.login.method || "POST", METHODS, function (v) { cfg.auth.login.method = v; sync(cfg); }));
    sec.appendChild(textField("Login endpoint", cfg.auth.login.endpoint, function (v) { cfg.auth.login.endpoint = v; sync(cfg); }));
    sec.appendChild(textField("Token path", cfg.auth.tokenPath, function (v) { cfg.auth.tokenPath = v; sync(cfg); }));
    sec.appendChild(jsonField("Login body (JSON)", cfg.auth.login.body, function (v) { cfg.auth.login.body = v; sync(cfg); }));
    sec.appendChild(hint("These credentials are shared by every participant. Use {{uniqid}} in a string to get a fresh unique value per run."));
    return sec;
  }

  // 3. Grading notes: two ordered lists. noteFor takes the first note whose min
  // the pct clears, so a non-descending list misfires. Warn inline; the server
  // rejects it too.
  function noteList(cfg, key, title) {
    var notes = cfg.grading[key] || (cfg.grading[key] = []);
    var box = document.createElement("div");
    box.className = "space-y-2";
    box.appendChild(heading("h4", title));

    var descending = true;
    for (var i = 1; i < notes.length; i++) { if (notes[i].min > notes[i - 1].min) { descending = false; break; } }
    if (!descending) {
      box.appendChild(chip("warn", "Min values are not ordered high to low. The first matching note wins, so lower thresholds are shadowed. Sort them."));
    }

    notes.forEach(function (n, ni) {
      var row = document.createElement("div");
      row.className = "flex items-end gap-2";
      var min = document.createElement("input");
      min.type = "number"; min.className = "input w-24"; min.value = n.min == null ? "" : n.min;
      min.addEventListener("input", function () { n.min = Number(min.value) || 0; sync(cfg); });
      row.appendChild(labelled("Min", min));
      var txt = document.createElement("input");
      txt.className = "input w-full"; txt.value = n.text || "";
      txt.addEventListener("input", function () { n.text = txt.value; sync(cfg); });
      var txtWrap = labelled("Note", txt); txtWrap.className = "block flex-1";
      row.appendChild(txtWrap);
      row.appendChild(iconBtn("close", "Remove note", "btn-danger", function () { notes.splice(ni, 1); sync(cfg); render(); }));
      box.appendChild(row);
    });

    var actions = document.createElement("div");
    actions.className = "flex gap-2";
    actions.appendChild(textBtn("+ note", "btn-tonal", function () { notes.push({ min: 0, text: "" }); sync(cfg); render(); }));
    actions.appendChild(textBtn("Sort high to low", "btn-ghost", function () {
      notes.sort(function (a, b) { return (b.min || 0) - (a.min || 0); });
      sync(cfg); render();
    }));
    box.appendChild(actions);
    return box;
  }

  function gradingSection(cfg) {
    cfg.grading = cfg.grading || {};
    var sec = document.createElement("div");
    sec.className = "space-y-4";
    sec.appendChild(heading("h3", "Grading notes"));
    sec.appendChild(noteList(cfg, "groupNotes", "Per-group notes"));
    sec.appendChild(noteList(cfg, "totalNotes", "Total notes"));
    return sec;
  }
  // PLACEHOLDER-C

  // 5. Assertion: a <details> so a long suite stays scannable. Summary reads
  // "{title} - {METHOD} {endpoint} - {score} pts"; body holds every field.
  function assertionNode(cfg, g, a, ai, assertions) {
    a.expected = a.expected || {};
    var det = document.createElement("details");
    det.className = "border border-outline-variant rounded-xl";

    var sum = document.createElement("summary");
    sum.className = "flex items-center gap-2 px-3 py-2 cursor-pointer text-label-large text-on-surface";
    var label = document.createElement("span");
    label.className = "flex-1 truncate";
    label.textContent = (a.title || "(untitled)") + " - " + (a.method || "?") + " " + (a.endpoint || "") + " - " + (a.score || 0) + " pts";
    sum.appendChild(label);
    sum.appendChild(iconBtn("keyboard_arrow_up", "Move up", "btn-ghost", function (e) { e.preventDefault(); if (move(assertions, ai, -1)) { sync(cfg); render(); } }));
    sum.appendChild(iconBtn("keyboard_arrow_down", "Move down", "btn-ghost", function (e) { e.preventDefault(); if (move(assertions, ai, 1)) { sync(cfg); render(); } }));
    sum.appendChild(iconBtn("close", "Remove assertion", "btn-danger", function (e) { e.preventDefault(); assertions.splice(ai, 1); sync(cfg); render(); }));
    det.appendChild(sum);

    var body = document.createElement("div");
    body.className = "px-3 pb-3 pt-1 space-y-3";

    body.appendChild(textField("Title", a.title, function (v) { a.title = v; sync(cfg); label.textContent = summaryText(a); }));
    body.appendChild(selectField("Method", a.method || "GET", METHODS, function (v) { a.method = v; sync(cfg); render(); }));
    body.appendChild(textField("Endpoint", a.endpoint, function (v) { a.endpoint = v; sync(cfg); label.textContent = summaryText(a); }));

    // requires auth / invalidates token. omitempty in Go: on false, delete the
    // key so the JSON matches what the server would marshal.
    var ra = checkboxRow("Requires auth", a.requires_auth, function (c) { if (c) a.requires_auth = true; else delete a.requires_auth; sync(cfg); });
    body.appendChild(ra.row);
    var it = checkboxRow("Invalidates token (e.g. logout)", a.invalidates_token, function (c) { if (c) a.invalidates_token = true; else delete a.invalidates_token; sync(cfg); });
    body.appendChild(it.row);
    body.appendChild(hint("An authed request lazily re-logs in only after a passing invalidates-token assertion, so logout suites re-authenticate on the next authed call."));

    body.appendChild(numField("Score (pts)", a.score, function (v) { a.score = Number(v) || 0; sync(cfg); label.textContent = summaryText(a); }));

    // deduction: null and 0 differ. Unchecked deletes the key (any single fail
    // zeroes the assertion); checked writes the number, 0 included (record
    // failures, keep score).
    var dedWrap = document.createElement("div");
    dedWrap.className = "space-y-1";
    var dedCb = checkboxRow("Custom per-check deduction", a.deduction != null, function (c) {
      if (c) a.deduction = Number(dedNum.value) || 0; else delete a.deduction;
      dedNum.disabled = !c; sync(cfg);
    });
    var dedNum = document.createElement("input");
    dedNum.type = "number"; dedNum.className = "input w-32"; dedNum.disabled = a.deduction == null;
    dedNum.value = a.deduction == null ? "" : a.deduction;
    dedNum.addEventListener("input", function () { a.deduction = Number(dedNum.value) || 0; sync(cfg); });
    dedWrap.appendChild(dedCb.row);
    dedWrap.appendChild(labelled("Deduction per failed check", dedNum));
    dedWrap.appendChild(hint("Off: any single failed check zeroes the whole assertion. On: each failed check subtracts this amount (0 records failures but keeps the score)."));
    body.appendChild(dedWrap);

    body.appendChild(jsonField("Request body (JSON)", a.request && a.request.body, function (v) {
      if (v === undefined) delete a.request; else a.request = { body: v };
      sync(cfg);
    }));
    if ((a.method === "GET" || a.method === "DELETE") && a.request && a.request.body) {
      body.appendChild(chip("warn", "A " + a.method + " request sends no body; this request body will be dropped at run time."));
    }

    body.appendChild(numField("Expected status code", a.expected.status_code, function (v) { a.expected.status_code = Number(v) || 0; sync(cfg); }));

    // Expected body shape: a visual tree so the author never sees numeric keys
    // or "*". Unrepresentable shapes fall back to a warning + JSON edit.
    body.appendChild(shapeSection(cfg, a));

    det.appendChild(body);
    return det;
  }

  function summaryText(a) {
    return (a.title || "(untitled)") + " - " + (a.method || "?") + " " + (a.endpoint || "") + " - " + (a.score || 0) + " pts";
  }

  // 4. Groups: a card per group with reorder/remove, its assertions, and an
  // add-assertion button.
  function groupCard(cfg, g, gi) {
    var card = document.createElement("div");
    card.className = "border border-outline-variant rounded-xl p-3 space-y-3";

    var head = document.createElement("div");
    head.className = "flex items-center gap-2";
    var title = heading("h4", "Group " + (gi + 1));
    title.classList.add("flex-1");
    head.appendChild(title);
    head.appendChild(iconBtn("keyboard_arrow_up", "Move group up", "btn-ghost", function () { if (move(cfg.groups, gi, -1)) { sync(cfg); render(); } }));
    head.appendChild(iconBtn("keyboard_arrow_down", "Move group down", "btn-ghost", function () { if (move(cfg.groups, gi, 1)) { sync(cfg); render(); } }));
    head.appendChild(iconBtn("close", "Remove group", "btn-danger", function () { cfg.groups.splice(gi, 1); sync(cfg); render(); }));
    card.appendChild(head);

    card.appendChild(textField("Group id", g.group_id, function (v) { g.group_id = v; sync(cfg); }));
    card.appendChild(textField("Group name", g.group_name, function (v) { g.group_name = v; sync(cfg); }));

    var list = document.createElement("div");
    list.className = "space-y-2";
    g.assertions = g.assertions || [];
    g.assertions.forEach(function (a, ai) { list.appendChild(assertionNode(cfg, g, a, ai, g.assertions)); });
    card.appendChild(list);

    card.appendChild(textBtn("+ assertion", "btn-tonal", function () {
      g.assertions.push({ title: "", method: "GET", endpoint: "", score: 0, expected: { status_code: 200 } });
      sync(cfg); render();
    }));
    return card;
  }

  // ---- expected-shape tree ------------------------------------------------
  // Turns expected.body into a visual tree so the author never sees numeric
  // keys or "*". Node model:
  //   {k:"scalar", name:"status"|"message", value:"..."}   top level only
  //   {k:"field",  name:"token"}                            children only
  //   {k:"object", name:"data",   children:[...]}
  //   {k:"list",   name:"errors", children:[...]}
  var RESERVED = ["status", "message"];
  function isNumeric(s) { return /^\d+$/.test(s); }
  function isPlainObject(v) { return v && typeof v === "object" && !Array.isArray(v); }

  // serChildren: all-field children -> a plain array (matches hand-written
  // configs); a mix -> an object with numeric keys for the fields plus named
  // keys for the rest. entries() treats both identically at run time.
  function serChildren(children) {
    var fields = children.filter(function (c) { return c.k === "field"; });
    var rest = children.filter(function (c) { return c.k !== "field"; });
    if (!rest.length) return fields.map(function (c) { return c.name; });
    var out = {};
    fields.forEach(function (c, i) { out[String(i)] = c.name; });
    rest.forEach(function (c) {
      out[c.name] = c.k === "list" ? { "*": serChildren(c.children) } : serChildren(c.children);
    });
    return out;
  }

  function serTop(nodes) {
    var out = {};
    nodes.forEach(function (n) {
      if (n.k === "scalar") out[n.name] = n.value;
      else if (n.k === "list") out[n.name] = { "*": serChildren(n.children) };
      else out[n.name] = serChildren(n.children);
    });
    return out;
  }

  // parseChildren is the inverse. Returns {nodes, ok}; ok=false means some part
  // has no tree representation (a scalar under a non-status/message key), so the
  // tree must not be shown as authoritative.
  function parseChildren(value) {
    var nodes = [], ok = true;
    if (Array.isArray(value)) {
      value.forEach(function (item) { nodes.push({ k: "field", name: String(item) }); });
      return { nodes: nodes, ok: ok };
    }
    if (isPlainObject(value)) {
      Object.keys(value).forEach(function (key) {
        var v = value[key];
        if (isNumeric(key)) { nodes.push({ k: "field", name: String(v) }); return; }
        if (isPlainObject(v) && v.hasOwnProperty("*")) {
          var inner = parseChildren(v["*"]);
          ok = ok && inner.ok;
          nodes.push({ k: "list", name: key, children: inner.nodes });
          return;
        }
        if (isPlainObject(v) || Array.isArray(v)) {
          var ch = parseChildren(v);
          ok = ok && ch.ok;
          nodes.push({ k: "object", name: key, children: ch.nodes });
          return;
        }
        ok = false; // scalar under a plain key: not representable
      });
      return { nodes: nodes, ok: ok };
    }
    return { nodes: nodes, ok: false };
  }

  // parseTop reads expected.body into top-level nodes. status/message are
  // scalar nodes; everything else recurses via parseChildren.
  function parseTop(bodyObj) {
    var nodes = [], ok = true;
    if (!isPlainObject(bodyObj)) return { nodes: nodes, ok: true };
    Object.keys(bodyObj).forEach(function (key) {
      var v = bodyObj[key];
      if (key === "status" || key === "message") { nodes.push({ k: "scalar", name: key, value: v }); return; }
      if (isPlainObject(v) && v.hasOwnProperty("*")) {
        var inner = parseChildren(v["*"]);
        ok = ok && inner.ok;
        nodes.push({ k: "list", name: key, children: inner.nodes });
        return;
      }
      if (isPlainObject(v) || Array.isArray(v)) {
        var ch = parseChildren(v);
        ok = ok && ch.ok;
        nodes.push({ k: "object", name: key, children: ch.nodes });
        return;
      }
      ok = false; // a plain top-level scalar other than status/message is ignored by the engine
    });
    return { nodes: nodes, ok: ok };
  }

  // childrenEditor renders the children of an object/list node, with add
  // buttons for field/object/list, and recurses.
  function childrenEditor(cfg, a, root, children) {
    var box = document.createElement("div");
    box.className = "ml-4 pl-3 border-l border-outline-variant space-y-2";
    children.forEach(function (c, ci) {
      box.appendChild(childRow(cfg, a, root, children, c, ci));
    });
    var actions = document.createElement("div");
    actions.className = "flex gap-2";
    actions.appendChild(textBtn("+ field", "btn-ghost", function () { children.push({ k: "field", name: "" }); commit(cfg, a, root); }));
    actions.appendChild(textBtn("+ object", "btn-ghost", function () { children.push({ k: "object", name: "", children: [] }); commit(cfg, a, root); }));
    actions.appendChild(textBtn("+ list", "btn-ghost", function () { children.push({ k: "list", name: "", children: [] }); commit(cfg, a, root); }));
    box.appendChild(actions);
    return box;
  }

  function childRow(cfg, a, root, siblings, c, ci) {
    var wrap = document.createElement("div");
    wrap.className = "space-y-1";
    var row = document.createElement("div");
    row.className = "flex items-center gap-2";
    var tag = chip("neutral", c.k === "field" ? "has field" : c.k === "list" ? "is list of" : "is object");
    row.appendChild(tag);
    var name = document.createElement("input");
    name.className = "input flex-1"; name.value = c.name || "";
    name.addEventListener("input", function () { c.name = name.value; commitQuiet(cfg, a, root); });
    row.appendChild(name);
    row.appendChild(iconBtn("close", "Remove", "btn-danger", function () { siblings.splice(ci, 1); commit(cfg, a, root); }));
    wrap.appendChild(row);
    if (c.k !== "field") {
      c.children = c.children || [];
      wrap.appendChild(childrenEditor(cfg, a, root, c.children));
    }
    return wrap;
  }

  // commit rewrites a.expected.body from the node tree (root), syncs, and does a
  // full re-render (which reparses the tree from the fresh JSON).
  function commit(cfg, a, root) {
    a.expected.body = serTop(root);
    sync(cfg); render();
  }
  // commitQuiet writes without a re-render so a text input keeps focus while
  // typing a name. The node tree stays live in the closure until the next
  // structural change forces a re-render.
  function commitQuiet(cfg, a, root) {
    a.expected.body = serTop(root);
    sync(cfg);
  }

  function shapeSection(cfg, a) {
    var box = document.createElement("div");
    box.className = "space-y-2";
    box.appendChild(heading("h4", "Expected body"));

    var parsed = parseTop(a.expected.body || {});
    if (!parsed.ok) {
      box.appendChild(chip("warn", "This expected shape has parts the builder cannot show (a scalar value under a non-status/message key, which the engine ignores). Edit it on the JSON tab."));
      box.appendChild(textBtn("Rebuild visually (drops the unsupported parts)", "btn-tonal", function () {
        commit(cfg, a, parsed.nodes);
      }));
      return box;
    }
    // root is the live node tree, held only in this closure so it never leaks
    // into the serialized config (JSON.stringify(cfg) must not see it).
    var root = parsed.nodes;

    root.forEach(function (n, ni) {
      box.appendChild(topRow(cfg, a, root, n, ni));
    });

    var actions = document.createElement("div");
    actions.className = "flex flex-wrap gap-2";
    // scalar (status/message) addable once each; object; list. No "has field":
    // top level cannot assert a field exists.
    RESERVED.forEach(function (name) {
      if (root.some(function (n) { return n.k === "scalar" && n.name === name; })) return;
      actions.appendChild(textBtn("+ " + name, "btn-ghost", function () { root.push({ k: "scalar", name: name, value: "" }); commit(cfg, a, root); }));
    });
    actions.appendChild(textBtn("+ object", "btn-ghost", function () { root.push({ k: "object", name: "", children: [] }); commit(cfg, a, root); }));
    actions.appendChild(textBtn("+ list", "btn-ghost", function () { root.push({ k: "list", name: "", children: [] }); commit(cfg, a, root); }));
    box.appendChild(actions);
    box.appendChild(hint("Top level cannot assert a bare field exists (the engine ignores it); nest it under an object or list. status/message compare values; every other key checks presence and shape only."));
    return box;
  }

  function topRow(cfg, a, root, n, ni) {
    var wrap = document.createElement("div");
    wrap.className = "space-y-1";
    var row = document.createElement("div");
    row.className = "flex items-center gap-2";
    if (n.k === "scalar") {
      row.appendChild(chip("neutral", n.name + (n.name === "message" ? " equals (case-insensitive)" : " equals")));
      var val = document.createElement("input");
      val.className = "input flex-1"; val.value = n.value == null ? "" : n.value;
      val.addEventListener("input", function () { n.value = val.value; commitQuiet(cfg, a, root); });
      row.appendChild(val);
    } else {
      row.appendChild(chip("neutral", n.k === "list" ? "is list of" : "is object"));
      var name = document.createElement("input");
      name.className = "input flex-1"; name.value = n.name || "";
      name.addEventListener("input", function () { n.name = name.value; commitQuiet(cfg, a, root); });
      row.appendChild(name);
    }
    row.appendChild(iconBtn("close", "Remove", "btn-danger", function () { root.splice(ni, 1); commit(cfg, a, root); }));
    wrap.appendChild(row);
    if (n.k !== "scalar") {
      n.children = n.children || [];
      wrap.appendChild(childrenEditor(cfg, a, root, n.children));
    }
    return wrap;
  }

  // ---- render -------------------------------------------------------------
  function render() {
    var cfg = parse();
    host.innerHTML = "";
    if (!cfg) {
      var err = document.createElement("div");
      err.className = "bg-error-container text-on-error-container rounded-xl px-4 py-3 text-body-medium";
      err.textContent = "JSON is invalid. Fix it on the JSON tab to use the builder.";
      host.appendChild(err);
      return;
    }
    cfg.base = cfg.base || {}; cfg.auth = cfg.auth || {}; cfg.auth.login = cfg.auth.login || {};
    cfg.groups = cfg.groups || [];

    var warnBox = document.createElement("div");
    warnBox.id = "am-b-warn";
    host.appendChild(warnBox);

    host.appendChild(serverSection(cfg));
    host.appendChild(authSection(cfg));
    host.appendChild(gradingSection(cfg));

    var groups = document.createElement("div");
    groups.className = "space-y-4";
    groups.appendChild(heading("h3", "Groups"));
    cfg.groups.forEach(function (g, gi) { groups.appendChild(groupCard(cfg, g, gi)); });
    groups.appendChild(textBtn("+ group", "btn-primary", function () {
      cfg.groups.push({ group_id: "", group_name: "", assertions: [] });
      sync(cfg); render();
    }));
    host.appendChild(groups);

    if (window.AutomarkBuilder && window.AutomarkBuilder.validate) window.AutomarkBuilder.validate();
  }

  function validate() { return 0; }

  window.AutomarkBuilder = { render: render, validate: validate };
})();
