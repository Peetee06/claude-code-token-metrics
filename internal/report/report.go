package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/petertrost/claude-code-token-metrics/internal/export"
)

// LoadDaily reads and parses an analyze-produced daily.json file.
func LoadDaily(path string) ([]export.DailyRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var rows []export.DailyRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return rows, nil
}

// Render builds a self-contained HTML report from an Aggregation. The
// aggregation is embedded as inline JSON and the charts are drawn client-side
// by Chart.js loaded from a CDN — the resulting file needs no other assets.
func Render(agg Aggregation) ([]byte, error) {
	dataJSON, err := json.Marshal(agg)
	if err != nil {
		return nil, fmt.Errorf("marshaling aggregation: %w", err)
	}
	tmpl, err := template.New("report").Parse(reportTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing report template: %w", err)
	}
	var buf bytes.Buffer
	// template.JS marks the JSON as safe to embed inside a <script> block.
	if err := tmpl.Execute(&buf, template.JS(dataJSON)); err != nil {
		return nil, fmt.Errorf("rendering report: %w", err)
	}
	return buf.Bytes(), nil
}

// WriteReport reads daily.json from outDir, builds the HTML report, and writes
// report.html into the same directory. It returns the path written.
func WriteReport(outDir string) (string, error) {
	rows, err := LoadDaily(filepath.Join(outDir, "daily.json"))
	if err != nil {
		return "", err
	}
	html, err := Render(Aggregate(rows))
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(outDir, "report.html")
	if err := os.WriteFile(outPath, html, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", outPath, err)
	}
	return outPath, nil
}

// reportTemplate is the self-contained HTML page. The single template field is
// the aggregation JSON, embedded verbatim into a <script> block.
const reportTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Claude Code token usage</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.7/dist/chart.umd.min.js"></script>
<style>
  :root {
    --bg: #0f1115;
    --panel: #181b22;
    --panel-2: #1f2330;
    --text: #e6e8ee;
    --muted: #8b90a0;
    --border: #2a2e3a;
    --accent: #6ea8fe;
  }
  @media (prefers-color-scheme: light) {
    :root {
      --bg: #f6f7f9; --panel: #ffffff; --panel-2: #f0f2f5;
      --text: #1a1d24; --muted: #6b7180; --border: #e3e6ec; --accent: #2f6fed;
    }
  }
  * { box-sizing: border-box; }
  body {
    font: 14px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
    margin: 0; padding: 40px 24px; background: var(--bg); color: var(--text);
  }
  .wrap { max-width: 1080px; margin-inline: auto; }
  header { margin-bottom: 32px; }
  h1 { font-size: 22px; font-weight: 650; letter-spacing: -0.01em; margin: 0; }
  .sub { color: var(--muted); font-size: 13px; margin-top: 4px; }

  .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; margin-bottom: 32px; }
  .stat {
    background: var(--panel); border: 1px solid var(--border);
    border-radius: 14px; padding: 18px 20px;
  }
  .stat .n { font-size: 26px; font-weight: 650; letter-spacing: -0.02em; }
  .stat .l { color: var(--muted); font-size: 11px; text-transform: uppercase;
             letter-spacing: 0.06em; margin-top: 4px; }

  .card {
    background: var(--panel); border: 1px solid var(--border);
    border-radius: 16px; padding: 22px; margin-bottom: 22px;
  }
  .card-head { display: flex; align-items: center; justify-content: space-between;
               margin-bottom: 16px; gap: 12px; flex-wrap: wrap; }
  h2 { font-size: 15px; font-weight: 600; margin: 0; }

  .toggle { display: inline-flex; background: var(--panel-2);
            border: 1px solid var(--border); border-radius: 9px; padding: 3px; }
  .toggle button {
    appearance: none; border: 0; background: transparent; color: var(--muted);
    font: inherit; font-size: 12px; font-weight: 550; cursor: pointer;
    padding: 5px 13px; border-radius: 6px; transition: all .12s ease;
  }
  .toggle button.active { background: var(--accent); color: #fff; }

  /* Filters button in the card header; anchors the overlay popover. */
  .filters-anchor { position: relative; }
  .filters-btn {
    appearance: none; display: inline-flex; align-items: center; gap: 6px;
    border: 1px solid var(--border); background: var(--panel-2); color: var(--text);
    font: inherit; font-size: 12px; font-weight: 550; cursor: pointer;
    padding: 6px 13px; border-radius: 9px; transition: all .12s ease;
  }
  .filters-btn:hover { border-color: var(--accent); }
  .filters-btn.open { background: var(--accent); color: #fff; border-color: var(--accent); }
  .filters-btn .chev { transition: transform .15s ease; font-size: 10px; }
  .filters-btn.open .chev { transform: rotate(180deg); }

  /* Overlay filter popover: floats over the content, anchored to the button. */
  .filter-panel {
    display: none; position: absolute; top: calc(100% + 8px); right: 0; z-index: 20;
    background: var(--panel); border: 1px solid var(--border); border-radius: 14px;
    padding: 16px; min-width: 360px;
    box-shadow: 0 12px 32px rgba(0,0,0,.35);
  }
  .filter-panel.open { display: block; }
  .filter-row { display: flex; align-items: center; gap: 12px;
                flex-wrap: wrap; margin-bottom: 12px; }
  .filter-row:last-child { margin-bottom: 0; }
  .filter-label { color: var(--muted); font-size: 11px; font-weight: 600;
                  text-transform: uppercase; letter-spacing: 0.05em;
                  width: 56px; flex-shrink: 0; }

  .range { display: inline-flex; gap: 6px; align-items: center;
           color: var(--muted); font-size: 12px; }
  .range select {
    appearance: none; font: inherit; font-size: 12px; color: var(--text);
    background: var(--panel-2); border: 1px solid var(--border);
    border-radius: 7px; padding: 5px 8px; cursor: pointer;
  }
  .range .reset {
    appearance: none; border: 1px solid var(--border); background: var(--panel-2);
    color: var(--muted); font: inherit; font-size: 12px; font-weight: 550;
    cursor: pointer; padding: 5px 11px; border-radius: 7px;
  }

  .chart-wrap { position: relative; height: 360px; }
  .hint { color: var(--muted); font-size: 11px; margin-top: 10px; }

  table { border-collapse: collapse; width: 100%; font-size: 13px; }
  th, td { text-align: left; padding: 9px 12px; border-bottom: 1px solid var(--border); }
  th { color: var(--muted); font-weight: 550; font-size: 11px;
       text-transform: uppercase; letter-spacing: 0.05em; }
  tbody tr:last-child td { border-bottom: 0; }
  td.num, th.num { text-align: right; font-variant-numeric: tabular-nums; }
  .dot { display: inline-block; width: 9px; height: 9px; border-radius: 3px;
         margin-right: 8px; vertical-align: middle; }
  .repo-detail { color: var(--muted); font-size: 11px; }
</style>
</head>
<body>
<div class="wrap">
<header>
  <h1>Claude Code token usage</h1>
  <div class="sub" id="sub"></div>
</header>

<div class="stats">
  <div class="stat"><div class="n" id="t-cost"></div><div class="l">total cost</div></div>
  <div class="stat"><div class="n" id="t-tokens"></div><div class="l">total i/o tokens</div></div>
  <div class="stat"><div class="n" id="t-projects"></div><div class="l">projects</div></div>
</div>

<div class="card">
  <div class="card-head">
    <h2 id="trendTitle">Cost over time — by project</h2>
    <div class="filters-anchor">
      <button class="filters-btn" id="filtersBtn" aria-expanded="false">
        <span>Filters</span><span class="chev">▾</span>
      </button>
      <div class="filter-panel" id="filterPanel">
        <div class="filter-row">
          <span class="filter-label">View</span>
          <div class="toggle" id="metricToggle">
            <button data-metric="cost" class="active">Cost</button>
            <button data-metric="tokens">Tokens</button>
          </div>
          <div class="toggle" id="bucketToggle">
            <button data-bucket="day" class="active">Daily</button>
            <button data-bucket="week">Weekly</button>
          </div>
          <div class="toggle" id="scaleToggle">
            <button data-scale="linear" class="active">Linear</button>
            <button data-scale="logarithmic">Log</button>
          </div>
        </div>
        <div class="filter-row">
          <span class="filter-label">Range</span>
          <div class="toggle" id="presetToggle">
            <button data-preset="7">7d</button>
            <button data-preset="30">30d</button>
            <button data-preset="all" class="active">All</button>
          </div>
          <div class="range">
            <select id="fromSel" aria-label="from date"></select>
            <span>→</span>
            <select id="toSel" aria-label="to date"></select>
            <button class="reset" id="rangeReset">Reset</button>
          </div>
        </div>
      </div>
    </div>
  </div>
  <div class="chart-wrap"><canvas id="trendChart"></canvas></div>
  <div class="hint">Click a project in the legend to hide it — the axis rescales to the rest.</div>
</div>

<div class="card">
  <div class="card-head"><h2>Cost by project</h2></div>
  <div class="chart-wrap"><canvas id="projChart"></canvas></div>
</div>

<div class="card">
  <div class="card-head"><h2>Project totals</h2></div>
  <table id="projTable">
    <thead><tr>
      <th>Project</th><th class="num">Cost</th><th class="num">I/O tokens</th>
    </tr></thead>
    <tbody></tbody>
  </table>
</div>
</div>

<script>
const DATA = {{ . }};

const fmtUSD = n => "$" + n.toLocaleString(undefined, {minimumFractionDigits: 2, maximumFractionDigits: 2});
const fmtNum = n => n.toLocaleString();

// A modern, readable categorical palette; projects beyond its length wrap.
const PALETTE = ["#6ea8fe","#f6a23c","#e7596b","#4fc9b0","#7bd16b","#f0c945",
                 "#b98ad8","#ff9db0","#a8896f","#9aa3b2"];
const colorFor = i => PALETTE[i % PALETTE.length];

// Theme-aware axis/grid colours pulled from CSS custom properties.
const css = getComputedStyle(document.documentElement);
const MUTED = css.getPropertyValue("--muted").trim();
const GRID = css.getPropertyValue("--border").trim();

// Headline figures.
document.getElementById("t-cost").textContent = fmtUSD(DATA.totalCost);
document.getElementById("t-tokens").textContent = fmtNum(DATA.totalTokens);
document.getElementById("t-projects").textContent = DATA.projects.length;
const dates = DATA.days.map(d => d.date);
document.getElementById("sub").textContent = dates.length
  ? dates[0] + " → " + dates[dates.length - 1] + " · generated by claude-code-token-metrics"
  : "no data · generated by claude-code-token-metrics";

// --- Trend chart: state-driven, all controls feed renderTrend() ---

const state = { metric: "cost", bucket: "day", scale: "linear", from: null, to: null };

// Format a value for the current metric.
const fmtMetric = v => state.metric === "tokens" ? fmtNum(Math.round(v)) : fmtUSD(v);
const axisTick = v => state.metric === "tokens" ? fmtCompact(v) : "$" + v;
function fmtCompact(n) {
  if (n >= 1e9) return (n / 1e9).toFixed(1) + "B";
  if (n >= 1e6) return (n / 1e6).toFixed(1) + "M";
  if (n >= 1e3) return (n / 1e3).toFixed(1) + "k";
  return "" + n;
}

// Buckets for the current bucket size, filtered to the selected date window.
// Day buckets filter by their date; week buckets are kept whole (a week is
// in-window if any of its days could fall in range — we approximate by the
// week label, which is monotic, so a simple lexical compare on derived dates
// is avoided: weeks are filtered by their first day instead, computed below).
function visibleBuckets() {
  if (state.bucket === "week") {
    // Filter weeks by whether the week overlaps [from,to] using day dates.
    if (!state.from && !state.to) return DATA.weeks;
    const inWeeks = new Set(
      DATA.days
        .filter(d => (!state.from || d.date >= state.from) &&
                     (!state.to || d.date <= state.to))
        .map(d => weekOf(d.date))
    );
    return DATA.weeks.filter(w => inWeeks.has(w.week));
  }
  return DATA.days.filter(d =>
    (!state.from || d.date >= state.from) && (!state.to || d.date <= state.to));
}

// weekOf mirrors the Go isoWeek labelling closely enough for filtering: it
// uses the JS Date ISO week. Labels are "YYYY-Www".
function weekOf(dateStr) {
  const d = new Date(dateStr + "T00:00:00Z");
  const day = (d.getUTCDay() + 6) % 7;          // Mon=0..Sun=6
  d.setUTCDate(d.getUTCDate() - day + 3);        // nearest Thursday
  const firstThu = new Date(Date.UTC(d.getUTCFullYear(), 0, 4));
  const week = 1 + Math.round(
    ((d - firstThu) / 86400000 - 3 + ((firstThu.getUTCDay() + 6) % 7)) / 7);
  return d.getUTCFullYear() + "-W" + String(week).padStart(2, "0");
}

function bucketValue(b, proj) {
  return state.metric === "tokens"
    ? (b.tokensByProject[proj] || 0)
    : +(b.costByProject[proj] || 0).toFixed(2);
}

// One line dataset per project for the current metric.
function lineDatasets(buckets) {
  return DATA.projectKeys.map((proj, i) => ({
    label: proj,
    data: buckets.map(b => bucketValue(b, proj)),
    borderColor: colorFor(i),
    backgroundColor: colorFor(i),
    tension: 0.3,
    borderWidth: 2,
    pointRadius: 2,
    pointHoverRadius: 4,
    fill: false,
  }));
}

const trend = new Chart(document.getElementById("trendChart"), {
  type: "line",
  data: { labels: [], datasets: [] },
  options: {
    responsive: true, maintainAspectRatio: false,
    interaction: { mode: "index", intersect: false },
    scales: {
      x: { grid: { color: GRID }, ticks: { color: MUTED, maxRotation: 0, autoSkip: true } },
      y: { type: "linear", grid: { color: GRID }, ticks: { color: MUTED } },
    },
    plugins: {
      legend: { position: "bottom", labels: { color: MUTED, boxWidth: 10, boxHeight: 10,
                                               usePointStyle: true, padding: 14 } },
      tooltip: { callbacks: { label: c => c.dataset.label + ": " + fmtMetric(c.parsed.y) } },
    },
  },
});

// renderTrend rebuilds the chart from current state. Hidden-series choices
// (set by clicking the legend) are preserved across re-renders by label.
function renderTrend() {
  const hidden = new Set();
  trend.data.datasets.forEach((ds, i) => {
    if (trend.getDatasetMeta(i).hidden) hidden.add(ds.label);
  });

  const buckets = visibleBuckets();
  const labels = state.bucket === "week"
    ? buckets.map(b => b.week) : buckets.map(b => b.date);
  const datasets = lineDatasets(buckets);
  datasets.forEach(ds => { if (hidden.has(ds.label)) ds.hidden = true; });

  trend.data.labels = labels;
  trend.data.datasets = datasets;
  // Log scale cannot include zero; linear keeps a zero baseline.
  const y = trend.options.scales.y;
  y.type = state.scale;
  y.beginAtZero = state.scale === "linear";
  y.ticks.callback = axisTick;
  trend.update();

  const noun = state.metric === "tokens" ? "Tokens" : "Cost";
  document.getElementById("trendTitle").textContent = noun + " over time — by project";
}

// Wire a segmented toggle: clicking sets state[key] and re-renders.
function wireToggle(id, attr, key) {
  document.getElementById(id).addEventListener("click", e => {
    const btn = e.target.closest("button");
    if (!btn) return;
    state[key] = btn.dataset[attr];
    document.querySelectorAll("#" + id + " button")
      .forEach(b => b.classList.toggle("active", b === btn));
    renderTrend();
  });
}
wireToggle("metricToggle", "metric", "metric");
wireToggle("bucketToggle", "bucket", "bucket");
wireToggle("scaleToggle", "scale", "scale");

// Date dropdowns are populated from the actual day range.
const fromSel = document.getElementById("fromSel");
const toSel = document.getElementById("toSel");
for (const d of dates) {
  fromSel.add(new Option(d, d));
  toSel.add(new Option(d, d));
}
function setRange(from, to) {
  state.from = from;
  state.to = to;
  if (from) fromSel.value = from;
  if (to) toSel.value = to;
  renderTrend();
}
fromSel.addEventListener("change", () => setRange(fromSel.value, state.to || dates[dates.length - 1]));
toSel.addEventListener("change", () => setRange(state.from || dates[0], toSel.value));

function clearPresetActive() {
  document.querySelectorAll("#presetToggle button").forEach(b => b.classList.remove("active"));
}
document.getElementById("presetToggle").addEventListener("click", e => {
  const btn = e.target.closest("button");
  if (!btn) return;
  clearPresetActive();
  btn.classList.add("active");
  if (btn.dataset.preset === "all" || dates.length === 0) {
    setRange(null, null);
    return;
  }
  const n = +btn.dataset.preset;
  const to = dates[dates.length - 1];
  const from = dates[Math.max(0, dates.length - n)];
  setRange(from, to);
});
document.getElementById("rangeReset").addEventListener("click", () => {
  clearPresetActive();
  document.querySelector('#presetToggle button[data-preset="all"]').classList.add("active");
  setRange(null, null);
});

// Overlay filter popover: the header button toggles a floating panel.
// Clicking outside it, or pressing Escape, closes it.
const filtersBtn = document.getElementById("filtersBtn");
const filterPanel = document.getElementById("filterPanel");

function setFilters(open) {
  filterPanel.classList.toggle("open", open);
  filtersBtn.classList.toggle("open", open);
  filtersBtn.setAttribute("aria-expanded", String(open));
}
filtersBtn.addEventListener("click", e => {
  e.stopPropagation();
  setFilters(!filterPanel.classList.contains("open"));
});
// Clicks inside the panel must not bubble out and close it.
filterPanel.addEventListener("click", e => e.stopPropagation());
document.addEventListener("click", () => setFilters(false));
document.addEventListener("keydown", e => {
  if (e.key === "Escape") setFilters(false);
});

renderTrend();

// Horizontal bar of lifetime cost per project.
new Chart(document.getElementById("projChart"), {
  type: "bar",
  data: {
    labels: DATA.projects.map(p => p.project),
    datasets: [{
      data: DATA.projects.map(p => +p.cost.toFixed(2)),
      backgroundColor: DATA.projects.map((_, i) => colorFor(i)),
      borderRadius: 5,
      barThickness: "flex",
      maxBarThickness: 26,
    }],
  },
  options: {
    indexAxis: "y", responsive: true, maintainAspectRatio: false,
    scales: {
      x: { grid: { color: GRID }, ticks: { color: MUTED, callback: v => "$" + v },
           beginAtZero: true },
      y: { grid: { display: false }, ticks: { color: MUTED } },
    },
    plugins: {
      legend: { display: false },
      tooltip: { callbacks: { label: c => fmtUSD(c.parsed.x) } },
    },
  },
});

// Project totals table; project name primary, repo path as secondary detail.
const tbody = document.querySelector("#projTable tbody");
DATA.projects.forEach((p, i) => {
  const tr = document.createElement("tr");
  tr.innerHTML =
    "<td><span class='dot'></span><span class='pname'></span>" +
    "<div class='repo-detail'></div></td>" +
    "<td class='num'></td><td class='num'></td>";
  tr.querySelector(".dot").style.background = colorFor(i);
  tr.querySelector(".pname").textContent = p.project;
  tr.querySelector(".repo-detail").textContent = p.repo || "";
  tr.children[1].textContent = fmtUSD(p.cost);
  tr.children[2].textContent = fmtNum(p.tokens);
  tbody.appendChild(tr);
});
</script>
</body>
</html>
`
