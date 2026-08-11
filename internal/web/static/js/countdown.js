// Polls /countdown/time once a second and paints #cd-clock (and #cd-status when present).
// The element opts into the TV behaviour with data-alert="<competition id>": an alert sound and a
// blink when the clock reaches zero, deduped across reloads via localStorage.
(function () {
  var clock = document.getElementById("cd-clock");
  if (!clock) return;
  var statusEl = document.getElementById("cd-status");
  var compID = clock.dataset.alert;
  var storeKey = compID ? "last_seconds_" + compID : null;
  // Jury control panel: toggle each [data-cd-show] element by the live status
  // so SAVE/RESUME/PAUSE/STOP always match the clock without a reload.
  var controls = document.querySelectorAll("[data-cd-show]");

  function syncControls(status) {
    controls.forEach(function (el) {
      var show = el.dataset.cdShow.split(" ").indexOf(status) !== -1;
      el.classList.toggle("hidden", !show);
    });
  }

  function format(s) {
    var h = Math.floor(s / 3600);
    var m = Math.floor((s % 3600) / 60);
    var sec = s % 60;
    return String(h).padStart(2, "0") + ":" + String(m).padStart(2, "0") + ":" + String(sec).padStart(2, "0");
  }

  function alertZero() {
    var prev = parseInt(localStorage.getItem(storeKey) || "0", 10);
    if (prev > 0) {
      // Autoplay may be blocked until the page is interacted with; failing silently is fine.
      new Audio("/static/sounds/alert.mp3").play().catch(function () {});
    }
  }

  var lastOk = Date.now(); // wall-clock of the last successful poll

  function markStale(stale) {
    clock.classList.toggle("opacity-40", stale);
    if (statusEl && stale) statusEl.textContent = "koneksi terputus";
  }

  function poll() {
    fetch("/countdown/time", { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        lastOk = Date.now();
        markStale(false);
        clock.textContent = format(d.seconds);
        if (statusEl) statusEl.textContent = d.status;
        syncControls(d.status);
        if (!storeKey) return;
        if (d.seconds === 0 && d.status !== "waiting") {
          alertZero();
          clock.classList.add("animate-pulse");
        } else {
          clock.classList.remove("animate-pulse");
        }
        localStorage.setItem(storeKey, String(d.seconds));
      })
      .catch(function () {
        // Server unreachable: flag the frozen clock as stale after 5s so a dead
        // poll is not mistaken for a genuinely stopped countdown.
        if (Date.now() - lastOk > 5000) markStale(true);
      });
  }

  poll();
  setInterval(poll, 1000);
})();
