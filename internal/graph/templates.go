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
    main { max-width: 52rem; margin: 2.5rem auto; padding: 0 1.25rem 3rem; }
    h1 { font-size: 1.75rem; margin: 0 0 0.35rem; letter-spacing: -0.02em; }
    h2 { font-size: 1.05rem; margin: 1.75rem 0 0.65rem; font-weight:600; }
    p { color: var(--muted); line-height: 1.5; }
    code, input, select { font-family:'JetBrains Mono',monospace; }
    code { background:var(--bg2); padding:0.1em 0.35em; border-radius:4px; border:1px solid var(--bd); font-size:0.85em; }
    .panel { margin-top:1.25rem; display:grid; gap:0.75rem; padding:1.25rem; background:var(--bg2); border:1px solid var(--bd); border-radius:10px; }
    label { font-size:0.85rem; color:var(--muted); }
    input[type=text], select { width:100%; margin-top:0.3rem; padding:0.55rem 0.7rem; background:var(--bg); border:1px solid var(--bd); border-radius:6px; color:var(--fg); }
    .row { display:grid; gap:0.75rem; grid-template-columns:1fr 1fr; }
    @media (max-width:640px) { .row { grid-template-columns:1fr; } }
    button { margin-top:0.35rem; padding:0.65rem 1rem; background:var(--green); color:#0d1117; border:none; border-radius:6px; font:600 0.95rem 'Outfit',sans-serif; cursor:pointer; }
    button.secondary { background:transparent; color:var(--accent); border:1px solid var(--bd); }
    button:hover { opacity:0.9; }
    .meta { margin-top:1.25rem; font-size:0.85rem; color:var(--muted); }
    a { color: var(--accent); }
    #results { margin-top:0.5rem; }
    #results .count { font-size:0.8rem; color:var(--muted); margin-bottom:0.5rem; }
    #results table { width:100%; border-collapse:collapse; font-size:0.82rem; }
    #results th, #results td { text-align:left; padding:0.4rem 0.5rem; border-bottom:1px solid var(--bd); vertical-align:top; }
    #results th { color:var(--muted); font-weight:500; }
    #results td.mono { font-family:'JetBrains Mono',monospace; font-size:0.78rem; }
    #results .empty { color:var(--muted); padding:0.75rem 0; }
  </style>
</head>
<body>
  <main>
    <h1>NetSpecGraph</h1>
    <p>Metrics companion to NetSpec — filter ports by the same rules NetSpec uses, then open per-interface graphs.</p>

    <form class="panel" method="POST" action="/">
      <div class="row">
        <div>
          <label for="device">Device</label>
          <input id="device" name="device" type="text" value="csw-mcd-01" required autocomplete="off">
        </div>
        <div>
          <label for="interface">Interface</label>
          <input id="interface" name="interface" type="text" value="Port-channel20" required autocomplete="off">
        </div>
      </div>
      <button type="submit">Open graphs</button>
    </form>

    <h2>Browse by rules</h2>
    <div class="panel">
      <div class="row">
        <div>
          <label for="port_role">Port role</label>
          <select id="port_role">
            <option value="">Any</option>
            {{range .PortRoles}}<option value="{{.Label}}">{{.Label}} ({{.Count}})</option>{{end}}
          </select>
        </div>
        <div>
          <label for="device_prefix">Device prefix</label>
          <select id="device_prefix">
            <option value="">Any</option>
            {{range .DeviceRoles}}<option value="{{.Prefix}}">{{.Prefix}} — {{.Name}}</option>{{end}}
          </select>
        </div>
      </div>
      <div class="row">
        <div>
          <label for="monitored">Monitored</label>
          <select id="monitored">
            <option value="">Any</option>
            <option value="true">Yes</option>
            <option value="false">No</option>
          </select>
        </div>
        <div>
          <label for="q">Search</label>
          <input id="q" type="text" placeholder="device, alias, interface…" autocomplete="off">
        </div>
      </div>
      <button type="button" class="secondary" id="run-filter">Filter interfaces</button>
      <div id="results"></div>
    </div>

    <p class="meta">{{.DeviceCount}} devices · {{.IfaceCount}} interfaces · {{.Timezone}} · <a href="{{.ExamplePath}}">example</a> · v{{.Version}}</p>
  </main>
  <script>
    const results = document.getElementById('results');
    async function runFilter() {
      const params = new URLSearchParams();
      const portRole = document.getElementById('port_role').value;
      const prefix = document.getElementById('device_prefix').value;
      const monitored = document.getElementById('monitored').value;
      const q = document.getElementById('q').value.trim();
      if (portRole) params.set('port_role', portRole);
      if (prefix) params.set('device_prefix', prefix);
      if (monitored) params.set('monitored', monitored);
      if (q) params.set('q', q);
      results.innerHTML = '<div class="empty">loading…</div>';
      try {
        const res = await fetch('/api/interfaces?' + params.toString());
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || res.statusText);
        const items = body.interfaces || [];
        if (!items.length) {
          results.innerHTML = '<div class="empty">No interfaces match this filter. Port roles with (0) are hidden — AP ports are often excluded from desired-state (monitor: false). Try “Any” or a role that shows a count.</div>';
          return;
        }
        let html = '<div class="count">' + items.length + ' interface' + (items.length === 1 ? '' : 's') + '</div>';
        html += '<table><thead><tr><th>Device</th><th>Interface</th><th>Alias</th><th>Port role</th></tr></thead><tbody>';
        for (const it of items.slice(0, 200)) {
          html += '<tr>'
            + '<td class="mono">' + esc(it.device) + '</td>'
            + '<td class="mono"><a href="' + esc(it.graph_path) + '">' + esc(it.interface) + '</a></td>'
            + '<td>' + esc(it.alias || '—') + '</td>'
            + '<td>' + esc(it.port_role || '—') + '</td>'
            + '</tr>';
        }
        html += '</tbody></table>';
        if (items.length > 200) html += '<div class="empty">Showing first 200 of ' + items.length + '.</div>';
        results.innerHTML = html;
      } catch (e) {
        results.innerHTML = '<div class="empty">Error: ' + esc(String(e.message || e)) + '</div>';
      }
    }
    function esc(s) {
      return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    }
    document.getElementById('run-filter').addEventListener('click', runFilter);
    document.getElementById('q').addEventListener('keydown', e => { if (e.key === 'Enter') { e.preventDefault(); runFilter(); } });
  </script>
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
    .chart { width:100%; min-height:220px; position:relative; }
    .empty { color:var(--muted); font-size:0.9rem; padding:1.5rem 0; text-align:center; }
    .legend { display:flex; gap:1rem; flex-wrap:wrap; font-size:0.78rem; color:var(--muted); margin-bottom:0.35rem; }
    .legend i { display:inline-block; width:10px; height:10px; border-radius:2px; margin-right:0.35rem; }
    .stats {
      width:100%; border-collapse:collapse; margin:0.35rem 0 0.75rem;
      font:500 0.72rem/1.35 'JetBrains Mono',monospace; color:var(--muted);
    }
    .stats th, .stats td { padding:0.2rem 0.55rem; text-align:right; border-bottom:1px solid var(--bd); }
    .stats th:first-child, .stats td:first-child { text-align:left; }
    .stats th { color:var(--muted); font-weight:500; }
    .stats td.cur { color:var(--fg); }
    .stats .swatch { display:inline-block; width:8px; height:8px; border-radius:2px; margin-right:0.4rem; vertical-align:middle; }
    .uplot-tip {
      display:none; position:absolute; z-index:10; pointer-events:none;
      background:rgba(22,27,34,0.96); border:1px solid var(--bd); border-radius:6px;
      padding:0.45rem 0.6rem; font:500 0.75rem/1.45 'JetBrains Mono',monospace;
      color:var(--fg); white-space:nowrap; box-shadow:0 4px 16px rgba(0,0,0,0.35);
      transform: translate(12px, 12px);
    }
    .uplot-tip .t { color:var(--muted); margin-bottom:0.25rem; }
    .uplot .u-legend { font-family:'JetBrains Mono',monospace; font-size:0.72rem; }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>{{.Device}} <span>{{.Interface}}</span></h1>
      <div class="meta">NetSpecGraph · {{.Timezone}} · v{{.Version}}{{if .InConfig}} · {{if .DeviceRole}}{{.DeviceRole}}{{else}}unmatched device role{{end}}{{if .PortRole}} · {{.PortRole}}{{end}}{{if .Alias}} · {{.Alias}}{{end}} · desired {{.DesiredState}} · monitored {{.Monitored}}{{else}} · not in desired-state{{end}}</div>
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
      <p class="sub" id="speedLine">speed: —</p>
      <table class="stats" id="trafficStats">
        <thead>
          <tr><th></th><th>Min</th><th>Avg</th><th>Max</th><th>Current</th></tr>
        </thead>
        <tbody>
          <tr>
            <td><span class="swatch" style="background:#3fb950"></span>In</td>
            <td id="stat-in-min">—</td><td id="stat-in-avg">—</td><td id="stat-in-max">—</td><td id="stat-in-cur" class="cur">—</td>
          </tr>
          <tr>
            <td><span class="swatch" style="background:#a371f7"></span>Out</td>
            <td id="stat-out-min">—</td><td id="stat-out-avg">—</td><td id="stat-out-max">—</td><td id="stat-out-cur" class="cur">—</td>
          </tr>
        </tbody>
      </table>
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
    const SERIES_URL = {{.SeriesURLJS}};
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
    function fmtPct(v) {
      if (v == null || !isFinite(v)) return '—';
      return Math.abs(v).toFixed(2) + ' %';
    }
    function fmtRate(v) {
      if (v == null || !isFinite(v)) return '—';
      if (Math.abs(v) >= 100) return v.toFixed(1) + ' /s';
      if (Math.abs(v) >= 10) return v.toFixed(2) + ' /s';
      return v.toFixed(3) + ' /s';
    }
    function fmtOper(v) {
      if (v == null || !isFinite(v)) return '—';
      return v >= 0.5 ? 'up (1)' : 'down (0)';
    }
    function fmtSpeed(v) {
      if (v == null || !(v > 0)) return 'speed: unavailable (util% disabled)';
      return 'Port speed: ' + fmtBPS(v);
    }
    function fmtAxisBPS(v) {
      if (!Number.isFinite(v)) return '';
      const sign = v < 0 ? '-' : '';
      const a = Math.abs(v);
      if (a >= 1e12) return sign + (a/1e12).toFixed(1) + 'T';
      if (a >= 1e9) return sign + (a/1e9).toFixed(1) + 'G';
      if (a >= 1e6) return sign + (a/1e6).toFixed(1) + 'M';
      if (a >= 1e3) return sign + (a/1e3).toFixed(1) + 'k';
      return sign + String(Math.round(a));
    }
    function seriesStats(points) {
      let min = null, max = null, sum = 0, n = 0, cur = null;
      (points || []).forEach(p => {
        if (p.v == null || !isFinite(p.v)) return;
        const v = Math.abs(p.v);
        if (min == null || v < min) min = v;
        if (max == null || v > max) max = v;
        sum += v; n++; cur = v;
      });
      return {
        min: min, max: max,
        avg: n ? sum / n : null,
        cur: cur,
      };
    }
    function setStatRow(prefix, stats, fmt) {
      document.getElementById('stat-' + prefix + '-min').textContent = fmt(stats.min);
      document.getElementById('stat-' + prefix + '-avg').textContent = fmt(stats.avg);
      document.getElementById('stat-' + prefix + '-max').textContent = fmt(stats.max);
      document.getElementById('stat-' + prefix + '-cur').textContent = fmt(stats.cur);
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
    function negateSeries(points) {
      return (points || []).map(p => ({ t: p.t, v: p.v == null ? null : -Math.abs(p.v) }));
    }
    // Custom tooltip on the chart container (not u.over).
    function tooltipPlugin(fmtY, absOut) {
      let tip, root;
      return {
        hooks: {
          init: [u => {
            root = u.root && u.root.parentElement;
            if (!root) return;
            tip = root.querySelector('.uplot-tip');
            if (!tip) {
              tip = document.createElement('div');
              tip.className = 'uplot-tip';
              root.appendChild(tip);
            }
          }],
          setCursor: [u => {
            if (!tip || !u || !u.cursor || !u.data || !u.data[0]) return;
            const { left, top, idx } = u.cursor;
            if (idx == null || idx < 0 || left == null || left < 0 || top == null || top < 0) {
              tip.style.display = 'none';
              return;
            }
            const ts = u.data[0][idx];
            if (ts == null) { tip.style.display = 'none'; return; }
            let html = '<div class="t">' + new Date(ts * 1000).toLocaleString() + '</div>';
            for (let i = 1; i < u.series.length; i++) {
              if (!u.series[i] || u.series[i].show === false) continue;
              let v = u.data[i] ? u.data[i][idx] : null;
              if (absOut && i === 2 && v != null) v = Math.abs(v);
              const color = u.series[i].stroke || '#e6edf3';
              html += '<div><span style="color:' + color + '">' + (u.series[i].label || ('s'+i)) + '</span>: ' + fmtY(v) + '</div>';
            }
            tip.innerHTML = html;
            tip.style.display = 'block';
            const tw = tip.offsetWidth || 160;
            const th = tip.offsetHeight || 60;
            const maxW = root.clientWidth || 320;
            const maxH = root.clientHeight || 200;
            let x = left + 14;
            let y = top + 14;
            if (x + tw > maxW - 4) x = left - tw - 10;
            if (y + th > maxH - 4) y = top - th - 10;
            tip.style.transform = 'translate(' + Math.max(0, x) + 'px,' + Math.max(0, y) + 'px)';
          }],
          destroy: [() => {
            if (tip) tip.style.display = 'none';
          }]
        }
      };
    }
    // Do not set cursor.points.show to a boolean — uPlot expects a function that
    // returns a DOM node. show:true becomes () => true and crashes with:
    // can't access property "contains", t is undefined.
    function baseOpts(targetId, height, yAxis, fmtY, scales, tipOpts) {
      const el = document.getElementById(targetId);
      const width = Math.max(320, (el && el.clientWidth) || 320);
      return {
        width: width,
        height: height,
        scales: scales || { y: { auto: true } },
        axes: [
          {
            stroke: '#8b949e',
            grid: { stroke: '#21262d' },
            ticks: { stroke: '#30363d' },
          },
          {
            label: yAxis,
            labelSize: 13,
            size: 72,
            gap: 6,
            stroke: '#8b949e',
            grid: { stroke: '#21262d' },
            ticks: { stroke: '#30363d' },
            values: (u, vals) => vals.map(v => Number.isFinite(v) ? fmtAxisBPS(v) : ''),
          }
        ],
        series: [{}],
        legend: { show: false },
        plugins: [tooltipPlugin(fmtY, !!(tipOpts && tipOpts.absOut))],
      };
    }
    function destroy(name) {
      if (plots[name]) {
        try { plots[name].destroy(); } catch (e) {}
        plots[name] = null;
      }
      const el = document.getElementById(name);
      if (el) el.innerHTML = '';
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
      const fmtY = mode === 'pct' ? fmtPct : fmtBPS;
      setStatRow('in', seriesStats(inS), fmtY);
      setStatRow('out', seriesStats(outS), fmtY);
      if (!hasAny(inS) && !hasAny(outS)) {
        renderEmpty('traffic', payload.has_data ? 'No traffic samples in this range' : 'No data for this interface');
        return;
      }
      destroy('traffic');
      // LibreNMS-style: In above zero, Out below zero.
      const data = mergeTime([inS, negateSeries(outS)]);
      if (!data[0] || data[0].length === 0) {
        renderEmpty('traffic', 'No traffic samples in this range');
        return;
      }
      const opts = baseOpts(
        'traffic',
        280,
        mode === 'pct' ? '% utilization (out down)' : 'bits/s (out down)',
        fmtY,
        { y: { auto: true } },
        { absOut: true }
      );
      opts.series = [
        {},
        {
          label: 'In',
          stroke: '#3fb950',
          fill: 'rgba(63,185,80,0.35)',
          fillTo: 0,
          width: 1.5,
          spanGaps: false,
        },
        {
          label: 'Out',
          stroke: '#a371f7',
          fill: 'rgba(163,113,247,0.35)',
          fillTo: 0,
          width: 1.5,
          spanGaps: false,
        },
      ];
      if (mode === 'pct') {
        opts.axes[1].values = (u, vals) => vals.map(v => Number.isFinite(v) ? (Math.abs(v).toFixed(1) + '%') : '');
      } else {
        opts.axes[1].values = (u, vals) => vals.map(v => Number.isFinite(v) ? fmtAxisBPS(v) : '');
      }
      try {
        plots.traffic = new uPlot(opts, data, document.getElementById('traffic'));
      } catch (e) {
        console.error('traffic plot failed', e);
        renderEmpty('traffic', 'Chart error: ' + (e && e.message ? e.message : e));
      }
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
      const opts = baseOpts('errors', 220, 'packets/s', fmtRate);
      opts.series = [
        {},
        { label: 'in errors', stroke: '#f85149', width: 2, spanGaps: false },
        { label: 'out errors', stroke: '#d29922', width: 2, spanGaps: false },
        { label: 'in discards', stroke: '#a371f7', width: 2, spanGaps: false },
        { label: 'out discards', stroke: '#79c0ff', width: 2, spanGaps: false },
      ];
      opts.axes[1].values = (u, vals) => vals.map(v => Number.isFinite(v) ? (Math.abs(v) >= 10 ? v.toFixed(1) : v.toFixed(2)) : '');
      try {
        plots.errors = new uPlot(opts, data, document.getElementById('errors'));
      } catch (e) {
        console.error('errors plot failed', e);
        renderEmpty('errors', 'Chart error: ' + (e && e.message ? e.message : e));
      }
    }
    function drawOper(payload) {
      const s = payload.series.oper_status || [];
      if (!hasAny(s)) {
        renderEmpty('oper', 'No oper-status samples');
        return;
      }
      destroy('oper');
      const data = toUplot(s);
      const opts = baseOpts('oper', 140, 'oper', fmtOper, { y: { range: [-0.05, 1.05] } });
      opts.series = [{}, { label: 'oper', stroke: '#3fb950', width: 2, fill: 'rgba(63,185,80,0.15)', spanGaps: false, points: { show: false } }];
      opts.axes[1].values = (u, vals) => vals.map(v => (v === 0 || v === 1) ? String(v) : '');
      try {
        plots.oper = new uPlot(opts, data, document.getElementById('oper'));
      } catch (e) {
        console.error('oper plot failed', e);
        renderEmpty('oper', 'Chart error: ' + (e && e.message ? e.message : e));
      }
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
