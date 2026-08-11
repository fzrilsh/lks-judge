// Global pending-state for real form submits: once a form starts navigating,
// disable its submit button and swap the label to "Menyimpan..." so a slow
// POST cannot be double-submitted and the user sees the click registered.
// One delegated listener, no per-form wiring. Skips forms whose submit was
// cancelled (a confirm() dialog that returned false) and the chunked uploader,
// which never fires a native submit.
(function () {
  document.addEventListener("submit", function (e) {
    if (e.defaultPrevented) return;
    var form = e.target;
    var btn = form.querySelector('button[type="submit"], button:not([type])');
    if (!btn || btn.disabled) return;
    // Defer so the browser still submits this button's name/value first.
    setTimeout(function () {
      btn.disabled = true;
      if (btn.dataset.pendingDone) return;
      btn.dataset.pendingDone = "1";
      var label = btn.getAttribute("aria-label") || btn.textContent.trim();
      btn.setAttribute("aria-label", label);
      btn.textContent = "Menyimpan...";
    }, 0);
  });
})();
