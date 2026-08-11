// Renders the two dashboard charts from the JSON embedded server-side. Chart.js
// (vendored) must load first. No network, no CDN: everything ships in the binary.
(function () {
  var el = document.getElementById("dash-chart-data");
  if (!el || typeof Chart === "undefined") return;

  var data;
  try {
    data = JSON.parse(el.textContent);
  } catch (e) {
    return;
  }

  var ink = "#2f5fd0";
  var grid = "rgba(27,32,40,0.08)";

  var timing = document.getElementById("chart-timing");
  if (timing) {
    new Chart(timing, {
      type: "bar",
      data: {
        labels: data.timingLabels || [],
        datasets: [{ data: data.timingCounts || [], backgroundColor: ink, borderRadius: 4 }],
      },
      options: {
        plugins: { legend: { display: false } },
        scales: {
          x: { grid: { display: false } },
          y: { beginAtZero: true, ticks: { precision: 0 }, grid: { color: grid } },
        },
      },
    });
  }

  var mod = document.getElementById("chart-module");
  if (mod) {
    new Chart(mod, {
      type: "bar",
      data: {
        labels: data.moduleLabels || [],
        datasets: [{ data: data.moduleCounts || [], backgroundColor: "#2c7a52", borderRadius: 4 }],
      },
      options: {
        indexAxis: "y",
        plugins: { legend: { display: false } },
        scales: {
          x: { beginAtZero: true, ticks: { precision: 0 }, grid: { color: grid } },
          y: { grid: { display: false } },
        },
      },
    });
  }
})();
