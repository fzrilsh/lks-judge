// Chunked resumable uploader, shared by the jury file manager (/jury/files) and
// the participant dashboard. Slices the chosen file into 2MB chunks, POSTs
// /upload/init to open a session, PUTs each chunk in order (one retry per chunk
// via /upload/{id}/status), then POSTs /upload/{id}/complete.
//
// Behavior is parameterized by data-* attributes on #dropzone:
//   data-upload-type  "file" (default) | "submission"
//   data-module-id    module id, sent with submissions
//   data-success-url  where to redirect on success (default /jury/files?saved=1)
//   data-error-url    error redirect base (default /jury/files?error=)
// A submission with no data-success-url just reloads the current page.
(function () {
  var CHUNK = 2 * 1024 * 1024; // must match upload.MaxChunkSize
  var input = document.getElementById("file-input");
  if (!input) return;
  var wrap = document.getElementById("progress-wrap");
  var bar = document.getElementById("progress-bar");
  var pct = document.getElementById("progress-pct");
  var nameEl = document.getElementById("progress-name");
  var dropzone = document.getElementById("dropzone");

  var uploadType = dropzone.getAttribute("data-upload-type") || "file";
  var moduleId = dropzone.getAttribute("data-module-id") || null;
  var successUrl = dropzone.getAttribute("data-success-url") || "/jury/files?saved=1";
  var errorUrl = dropzone.getAttribute("data-error-url") || "/jury/files?error=";
  var flashHost = document.getElementById("client-flash");

  // showFlash renders an inline banner into #client-flash (participant dashboard).
  // Falls back to no-op when the host is absent (e.g. the jury file manager).
  function showFlash(msg) {
    if (!flashHost) return;
    flashHost.innerHTML =
      '<div class="flex items-start gap-3 rounded-xl px-4 py-3 mb-6 text-body-medium bg-error-container text-on-error-container" role="alert">' +
      '<span class="material-symbols-outlined text-xl" aria-hidden="true">error</span>' +
      "<span></span></div>";
    flashHost.querySelector("span:last-child").textContent = msg;
  }

  function clearFlash() {
    if (flashHost) flashHost.innerHTML = "";
  }


  function setProgress(done, total, name) {
    wrap.classList.remove("hidden");
    nameEl.textContent = name;
    var p = total === 0 ? 0 : Math.round((done / total) * 100);
    bar.style.width = p + "%";
    pct.textContent = p + "%";
  }

  function putChunk(id, n, blob) {
    return fetch("/upload/" + id + "/chunk/" + n, { method: "PUT", body: blob });
  }

  async function upload(file) {
    var total = Math.ceil(file.size / CHUNK);
    var manifest = {
      filename: file.name,
      total_chunks: total,
      total_size: file.size,
      upload_type: uploadType,
    };
    if (uploadType === "submission" && moduleId) manifest.module_id = parseInt(moduleId, 10);
    var initRes = await fetch("/upload/init", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(manifest),
    });
    if (!initRes.ok) throw new Error("init failed: " + initRes.status);
    var id = (await initRes.json()).upload_id;

    for (var n = 0; n < total; n++) {
      var blob = file.slice(n * CHUNK, Math.min((n + 1) * CHUNK, file.size));
      var res = await putChunk(id, n, blob);
      if (!res.ok) {
        // One retry: the chunk may already be staged, so re-check before resending.
        var st = await (await fetch("/upload/" + id + "/status")).json();
        if ((st.received_chunks || []).indexOf(n) === -1) {
          res = await putChunk(id, n, blob);
          if (!res.ok) throw new Error("chunk " + n + " failed: " + res.status);
        }
      }
      setProgress(n + 1, total, file.name);
    }

    var comp = await fetch("/upload/" + id + "/complete", { method: "POST" });
    if (!comp.ok) throw new Error("complete failed: " + comp.status);
    if (uploadType === "submission" && successUrl === "/jury/files?saved=1") {
      window.location.reload();
    } else {
      window.location.href = successUrl;
    }
  }

  function start(file) {
    if (!file) return;
    // Block a submission with no module before opening a session, so the
    // participant gets a clear message instead of a server 400 mid-upload.
    if (uploadType === "submission" && !moduleId) {
      input.value = "";
      showFlash("Module not selected");
      return;
    }
    clearFlash();
    input.disabled = true;
    setProgress(0, Math.ceil(file.size / CHUNK), file.name);
    upload(file).catch(function (err) {
      window.location.href = errorUrl + encodeURIComponent(err.message);
    });
  }

  input.addEventListener("change", function () { start(input.files[0]); });

  // Drag and drop onto the label.
  dropzone.addEventListener("dragover", function (e) { e.preventDefault(); });
  dropzone.addEventListener("drop", function (e) {
    e.preventDefault();
    if (e.dataTransfer.files.length) start(e.dataTransfer.files[0]);
  });
})();
