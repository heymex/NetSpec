package graph

import "html/template"

var indexTemplate = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NetSpecGraph</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg:#0d1117; --bg2:#161b22; --bd:#30363d; --fg:#e6edf3; --muted:#8b949e;
      --accent:#58a6ff; --green:#3fb950;
    }
    * { box-sizing: border-box; }
    body { margin:0; font-family:'Outfit',system-ui,sans-serif; background:var(--bg); color:var(--fg); min-height:100vh; }
    main { max-width: 36rem; margin: 3rem auto; padding: 0 1.25rem; }
    h1 { font-size: 1.75rem; margin: 0 0 0.35rem; letter-spacing: -0.02em; }
    p { color: var(--muted); line-height: 1.5; }
    code, input { font-family:'JetBrains Mono',monospace; }
    code { background:var(--bg2); padding:0.1em 0.35em; border-radius:4px; border:1px solid var(--bd); font-size:0.85em; }
    form { margin-top:1.75rem; display:grid; gap:0.75rem; padding:1.25rem; background:var(--bg2); border:1px solid var(--bd); border-radius:10px; }
    label { font-size:0.85rem; color:var(--muted); }
    input[type=text] { width:100%; margin-top:0.3rem; padding:0.55rem 0.7rem; background:var(--bg); border:1px solid var(--bd); border-radius:6px; color:var(--fg); }
    button { margin-top:0.35rem; padding:0.65rem 1rem; background:var(--green); color:#0d1117; border:none; border-radius:6px; font:600 0.95rem 'Outfit',sans-serif; cursor:pointer; }
    button:hover { opacity:0.9; }
    .meta { margin-top:1.5rem; font-size:0.85rem; color:var(--muted); }
    a { color: var(--accent); }
  </style>
</head>
<body>
  <main>
    <h1>NetSpecGraph</h1>
    <p>Metrics companion to NetSpec — per-interface utilization and errors.</p>
    <form method="POST" action="/">
      <div>
        <label for="device">Device</label>
        <input id="device" name="device" type="text" value="csw-mcd-01" required autocomplete="off">
      </div>
      <div>
        <label for="interface">Interface</label>
        <input id="interface" name="interface" type="text" value="Port-channel20" required autocomplete="off">
      </div>
      <button type="submit">Open graphs</button>
    </form>
    <p class="meta">{{.DeviceCount}} devices loaded · {{.Timezone}} · <a href="{{.ExamplePath}}">example</a> · v{{.Version}}</p>
  </main>
</body>
</html>
`))

var ifaceTemplate = template.Must(template.New("iface").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Device}} {{.Interface}} — NetSpecGraph</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@400;500;600;700&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/uplot@1.6.31/dist/uPlot.min.css">
  <style>
    :root {
      --bg:#0d1117; --bg2:#161b22; --bg3:#21262d; --bd:#30363d; --fg:#e6edf3; --muted:#8b949e;
      --accent:#58a6ff; --green:#3fb950; --red:#f85149; --yellow:#d29922; --purple:#a371f7;
    }
    * { box-sizing: border-box; }
    body { margin:0; font-family:'Outfit',system-ui,sans-serif; background:var(--bg); color:var(--fg); min-height:100vh; }
    header { display:flex; flex-wrap:wrap; gap:0.75rem 1.25rem; align-items:baseline; justify-content:space-between;
      padding:1rem 1.25rem; border-bottom:1px solid var(--bd); background:var(--bg2); }
    header h1 { margin:0; font-size:1.15rem; font-weight:600; }
    header h1 span { color:var(--muted); font-weight:500; }
    header .meta { font-family:'JetBrains Mono',monospace; font-size:0.78rem; color:var(--muted); }
    header a { color:var(--accent); text-decoration:none; }
    .toolbar { display:flex; flex-wrap:wrap; gap:0.5rem; align-items:center; padding:0.75rem 1.25rem; border-bottom:1px solid var(--bd); }
    .toolbar button, .toolbar select {
      background:var(--bg3); color:var(--fg); border:1px solid var(--bd); border-radius:6px;
      padding:0.35rem 0.65rem; font:500 0.85rem 'Outfit',sans-serif; cursor:pointer;
    }
    .toolbar button.active { border-color:var(--accent); color:var(--accent); }
    .status { margin-left:auto; font-family:'JetBrains Mono',monospace; font-size:0.75rem; color:var(--muted); }
    .status.err { color:var(--red); }
    main { padding:1rem 1.25rem 2rem; display:grid; gap:1.25rem; max-width:1200px; margin:0 auto; }
    section { background:var(--bg2); border:1px solid var(--bd); border-radius:10px; padding:0.85rem 1rem 0.5rem; }
    section h2 { margin:0 0 0.35rem; font-size:0.95rem; font-weight:600; }
    section .sub { margin:0 0 0.65rem; font-size:0.78rem; color:var(--muted); font-family:'JetBrains Mono',monospace; }
    .chart { width:100%; min-height:220px; }
    .empty { color:var(--muted); font-size:0.9rem; padding:1.5rem 0; text-align:center; }
    .legend { display:flex; gap:1rem; flex-wrap:wrap; font-size:0.78rem; color:var(--muted); margin-bottom:0.35rem; }
    .legend i { display:inline-block; width:10px; height:10px; border-radius:2px; margin-right:0.35rem; }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>{{.Device}} <span>{{.Interface}}</span></h1>
      <div class="meta">NetSpecGraph · {{.Timezone}} · v{{.Version}}</div>
    </div>
    <a href="/">← All interfaces</a>
  </header>
  <div class="toolbar">
    <button type="button" data-range="1h">1h</button>
    <button type="button" data-range="6h" class="active">6h</button>
    <button type="button" data-range="24h">24h</button>
    <select id="mode">
      <option value="bps" selected>bits/s</option>
      <option value="pct">% utilization</option>
    </select>
    <span id="status" class="status">loading…</span>
  </div>
  <main>
    <section>
      <h2>Traffic</h2>
      <div class="legend">
        <span><i style="background:var(--accent)"></i>in</span>
        <span><i style="background:var(--green)"></i>out</span>
      </div>
      <p class="sub" id="speedLine">speed: —</p>
      <div id="traffic" class="chart"></div>
    </section>
    <section>
      <h2>Errors / discards</h2>
      <div class="legend">
        <span><i style="background:var(--red)"></i>in errors</span>
        <span><i style="background:var(--yellow)"></i>out errors</span>
        <span><i style="background:var(--purple)"></i>in discards</span>
        <span><i style="background:#79c0ff"></i>out discards</span>
      </div>
      <div id="errors" class="chart"></div>
    </section>
    <section>
      <h2>Oper status</h2>
      <p class="sub">1 = up, 0 = other</p>
      <div id="oper" class="chart"></div>
    </section>
  </main>
  <script src="https://cdn.jsdelivr.net/npm/uplot@1.6.31/dist/uPlot.iife.min.js"></script>
  <script>
    const SERIES_URL = {{printf "%q" .SeriesURL}};
    const statusEl = document.getElementById('status');
    const speedLine = document.getElementById('speedLine');
    const modeEl = document.getElementById('mode');
    let range = '6h';
    let plots = { traffic: null, errors: null, oper: null };
    let lastPayload = null;

    function fmtBPS(v) {
      if (v == null || !isFinite(v)) return '—';
      const u = ['bps','Kbps','Mbps','Gbps','Tbps'];
      let i = 0, x = Math.abs(v);
      while (x >= 1000 && i < u.length - 1) { x /= 1000; i++; }
      return (v < 0 ? '-' : '') + x.toFixed(x >= 100 ? 0 : x >= 10 ? 1 : 2) + ' ' + u[i];
    }
    function fmtSpeed(v) {
      if (v == null || !(v > 0)) return 'speed: unavailable (util% disabled)';
      return 'speed: ' + fmtBPS(v);
    }
    function toUplot(points) {
      const t = [], v = [];
      (points || []).forEach(p => { t.push(p.t); v.push(p.v == null ? null : p.v); });
      return [t, v];
    }
    function mergeTime(seriesList) {
      const set = new Set();
      seriesList.forEach(s => (s || []).forEach(p => set.add(p.t)));
      const t = Array.from(set).sort((a,b)=>a-b);
      const cols = [t];
      seriesList.forEach(points => {
        const map = new Map((points || []).map(p => [p.t, p.v]));
        cols.push(t.map(ts => {
          const v = map.get(ts);
          return v == null ? null : v;
        }));
      });
      return cols;
    }
    function baseOpts(height, scales) {
      return {
        width: Math.max(320, document.getElementById('traffic').clientWidth),
        height,
        scales: scales || { y: { auto: true } },
        axes: [
          { stroke: '#8b949e', grid: { stroke: '#21262d' }, ticks: { stroke: '#30363d' } },
          { stroke: '#8b949e', grid: { stroke: '#21262d' }, ticks: { stroke: '#30363d' },
            values: (u, vals) => vals.map(v => Number.isFinite(v) ? (Math.abs(v) >= 1e9 ? (v/1e9).toFixed(1)+'G' : Math.abs(v) >= 1e6 ? (v/1e6).toFixed(1)+'M' : Math.abs(v) >= 1e3 ? (v/1e3).toFixed(1)+'k' : v.toFixed(2)) : '') }
        ],
        series: [{}],
        legend: { show: false },
      };
    }
    function destroy(name) {
      if (plots[name]) { plots[name].destroy(); plots[name] = null; }
      const el = document.getElementById(name);
      el.innerHTML = '';
    }
    function renderEmpty(id, msg) {
      destroy(id);
      const el = document.getElementById(id);
      el.innerHTML = '<div class="empty">' + msg + '</div>';
    }
    function hasAny(points) {
      return (points || []).some(p => p.v != null);
    }
    function drawTraffic(payload) {
      const mode = modeEl.value;
      const inKey = mode === 'pct' ? 'in_util_pct' : 'in_bps';
      const outKey = mode === 'pct' ? 'out_util_pct' : 'out_bps';
      const inS = payload.series[inKey] || [];
      const outS = payload.series[outKey] || [];
      if (!hasAny(inS) && !hasAny(outS)) {
        renderEmpty('traffic', payload.has_data ? 'No traffic samples in this range' : 'No data for this interface');
        return;
      }
      destroy('traffic');
      const data = mergeTime([inS, outS]);
      const opts = baseOpts(240, mode === 'pct' ? { y: { range: [0, null] } } : undefined);
      opts.series = [
        {},
        { label: 'in', stroke: '#58a6ff', width: 2, spanGaps: false },
        { label: 'out', stroke: '#3fb950', width: 2, spanGaps: false },
      ];
      if (mode === 'pct') {
        opts.axes[1].values = (u, vals) => vals.map(v => Number.isFinite(v) ? v.toFixed(1)+'%' : '');
      } else {
        opts.axes[1].values = (u, vals) => vals.map(v => Number.isFinite(v) ? fmtBPS(v) : '');
      }
      plots.traffic = new uPlot(opts, data, document.getElementById('traffic'));
    }
    function drawErrors(payload) {
      const keys = ['in_errors_ps','out_errors_ps','in_discards_ps','out_discards_ps'];
      const series = keys.map(k => payload.series[k] || []);
      if (!series.some(hasAny)) {
        renderEmpty('errors', 'No error/discard samples');
        return;
      }
      destroy('errors');
      const data = mergeTime(series);
      const opts = baseOpts(200);
      opts.series = [
        {},
        { label: 'in err', stroke: '#f85149', width: 2, spanGaps: false },
        { label: 'out err', stroke: '#d29922', width: 2, spanGaps: false },
        { label: 'in disc', stroke: '#a371f7', width: 2, spanGaps: false },
        { label: 'out disc', stroke: '#79c0ff', width: 2, spanGaps: false },
      ];
      plots.errors = new uPlot(opts, data, document.getElementById('errors'));
    }
    function drawOper(payload) {
      const s = payload.series.oper_status || [];
      if (!hasAny(s)) {
        renderEmpty('oper', 'No oper-status samples');
        return;
      }
      destroy('oper');
      const data = toUplot(s);
      const opts = baseOpts(120, { y: { range: [-0.05, 1.05] } });
      opts.series = [{}, { label: 'oper', stroke: '#3fb950', width: 2, fill: 'rgba(63,185,80,0.15)', spanGaps: false, points: { show: false } }];
      opts.axes[1].values = (u, vals) => vals.map(v => (v === 0 || v === 1) ? String(v) : '');
      plots.oper = new uPlot(opts, data, document.getElementById('oper'));
    }
    function render(payload) {
      lastPayload = payload;
      speedLine.textContent = fmtSpeed(payload.speed_bps);
      drawTraffic(payload);
      drawErrors(payload);
      drawOper(payload);
    }
    async function load() {
      statusEl.textContent = 'loading…';
      statusEl.classList.remove('err');
      try {
        const res = await fetch(SERIES_URL + '?range=' + encodeURIComponent(range));
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        render(body);
        statusEl.textContent = body.has_data
          ? ('updated ' + new Date().toLocaleTimeString())
          : 'no data';
      } catch (e) {
        statusEl.textContent = String(e.message || e);
        statusEl.classList.add('err');
      }
    }
    document.querySelectorAll('.toolbar button[data-range]').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.toolbar button[data-range]').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        range = btn.getAttribute('data-range');
        load();
      });
    });
    modeEl.addEventListener('change', () => { if (lastPayload) drawTraffic(lastPayload); });
    window.addEventListener('resize', () => { if (lastPayload) render(lastPayload); });
    load();
    setInterval(load, 30000);
  </script>
</body>
</html>
`))
