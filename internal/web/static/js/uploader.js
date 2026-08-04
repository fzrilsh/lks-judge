// Chunked resumable uploader for the jury file manager (/jury/files).
// Slices the chosen file into 2MB chunks, POSTs /upload/init to open a session,
// PUTs each chunk in order (one retry per chunk via /upload/{id}/status), then
// POSTs /upload/{id}/complete. On success it reloads to show the new row.
(function () {
  var CHUNK = 2 * 1024 * 1024; // must match upload.MaxChunkSize
  var input = document.getElementById("file-input");
  if (!input) return;
  var wrap = document.getElementById("progress-wrap");
  var bar = document.getElementById("progress-bar");
  var pct = document.getElementById("progress-pct");
  var nameEl = document.getElementById("progress-name");
  var dropzone = document.getElementById("dropzone");

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
    var initRes = await fetch("/upload/init", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        filename: file.name,
        total_chunks: total,
        total_size: file.size,
        upload_type: "file",
      }),
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
    window.location.href = "/jury/files?saved=1";
  }

  function start(file) {
    if (!file) return;
    input.disabled = true;
    setProgress(0, Math.ceil(file.size / CHUNK), file.name);
    upload(file).catch(function (err) {
      window.location.href = "/jury/files?error=" + encodeURIComponent(err.message);
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
