// Polls /countdown/time once a second and paints #cd-clock (and #cd-status when present).
// The element opts into the TV behaviour with data-alert="<competition id>": an alert sound and a
// blink when the clock reaches zero, deduped across reloads via localStorage.
(function () {
  var clock = document.getElementById("cd-clock");
  if (!clock) return;
  var statusEl = document.getElementById("cd-status");
  var compID = clock.dataset.alert;
  var storeKey = compID ? "last_seconds_" + compID : null;

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

  function poll() {
    fetch("/countdown/time", { cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        clock.textContent = format(d.seconds);
        if (statusEl) statusEl.textContent = d.status;
        if (!storeKey) return;
        if (d.seconds === 0 && d.status !== "waiting") {
          alertZero();
          clock.classList.add("animate-pulse");
        } else {
          clock.classList.remove("animate-pulse");
        }
        localStorage.setItem(storeKey, String(d.seconds));
      })
      .catch(function () {});
  }

  poll();
  setInterval(poll, 1000);
})();
