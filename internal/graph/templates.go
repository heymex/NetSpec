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
    <p class="meta" style="margin-top:0.5rem">
      <a href="/fleet">Fleet / top-talkers</a>
      {{if .NetSpecPublicURL}} · <a href="{{.NetSpecPublicURL}}">NetSpec</a>{{end}}
    </p>

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
    header .nav { display:flex; gap:1rem; align-items:center; }
    header a { color:var(--accent); text-decoration:none; }
    .toolbar { display:flex; flex-wrap:wrap; gap:0.5rem; align-items:center; padding:0.75rem 1.25rem; border-bottom:1px solid var(--bd); }
    .toolbar button, .toolbar select {
      background:var(--bg3); color:var(--fg); border:1px solid var(--bd); border-radius:6px;
      padding:0.35rem 0.65rem; font:500 0.85rem 'Outfit',sans-serif; cursor:pointer;
    }
    .toolbar button.active { border-color:var(--accent); color:var(--accent); }
    .toolbar .chk { display:inline-flex; align-items:center; gap:0.35rem; font-size:0.8rem; color:var(--muted); cursor:pointer; user-select:none; }
    .toolbar .chk input { accent-color:var(--accent); }
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
    <div class="nav">
      <a href="/fleet">Fleet</a>
      <a href="{{.OpticsPath}}">Optics / DOM</a>
      {{if .NetSpecDeviceURL}}<a href="{{.NetSpecDeviceURL}}">NetSpec device</a>{{end}}
      <a href="/">← All interfaces</a>
    </div>
  </header>
  <div class="toolbar">
    <button type="button" data-range="1h">1h</button>
    <button type="button" data-range="6h" class="active">6h</button>
    <button type="button" data-range="24h">24h</button>
    <select id="mode">
      <option value="bps" selected>bits/s</option>
      <option value="pct">% utilization</option>
    </select>
    <label class="chk"><input type="checkbox" id="band" checked> seasonality</label>
    <select id="baseline" title="Baseline overlay">
      <option value="" selected>No baseline</option>
      <option value="1w">1 week ago</option>
      <option value="52w">Same week last year</option>
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
    const bandEl = document.getElementById('band');
    const baselineEl = document.getElementById('baseline');
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
              const label = u.series[i].label || ('s'+i);
              if (label.indexOf('band ') === 0) continue;
              let v = u.data[i] ? u.data[i][idx] : null;
              if (absOut && (label === 'Out' || label === 'Out baseline') && v != null) v = Math.abs(v);
              const color = u.series[i].stroke || '#e6edf3';
              html += '<div><span style="color:' + color + '">' + label + '</span>: ' + fmtY(v) + '</div>';
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

      const cols = [inS, negateSeries(outS)];
      const seriesOpts = [
        {},
        {
          label: 'In',
          stroke: '#3fb950',
          fill: 'rgba(63,185,80,0.28)',
          fillTo: 0,
          width: 1.5,
          spanGaps: false,
        },
        {
          label: 'Out',
          stroke: '#a371f7',
          fill: 'rgba(163,113,247,0.28)',
          fillTo: 0,
          width: 1.5,
          spanGaps: false,
        },
      ];
      const bands = [];

      const band = payload.band;
      if (band && band.has_data && band.series) {
        const p10in = mode === 'pct' ? (band.series.in_p10_pct || []) : (band.series.in_p10 || []);
        const p90in = mode === 'pct' ? (band.series.in_p90_pct || []) : (band.series.in_p90 || []);
        const p10out = mode === 'pct' ? (band.series.out_p10_pct || []) : (band.series.out_p10 || []);
        const p90out = mode === 'pct' ? (band.series.out_p90_pct || []) : (band.series.out_p90 || []);
        if (hasAny(p10in) || hasAny(p90in) || hasAny(p10out) || hasAny(p90out)) {
          // Insert band series before live In/Out so live strokes stay on top.
          cols.length = 0;
          cols.push(p10in, p90in, negateSeries(p90out), negateSeries(p10out), inS, negateSeries(outS));
          seriesOpts.length = 0;
          seriesOpts.push(
            {},
            { label: 'band in p10', stroke: 'rgba(63,185,80,0.0)', width: 0, points: { show: false }, spanGaps: true },
            { label: 'band in p90', stroke: 'rgba(63,185,80,0.0)', width: 0, points: { show: false }, spanGaps: true },
            { label: 'band out p90', stroke: 'rgba(163,113,247,0.0)', width: 0, points: { show: false }, spanGaps: true },
            { label: 'band out p10', stroke: 'rgba(163,113,247,0.0)', width: 0, points: { show: false }, spanGaps: true },
            {
              label: 'In',
              stroke: '#3fb950',
              fill: 'rgba(63,185,80,0.22)',
              fillTo: 0,
              width: 1.5,
              spanGaps: false,
            },
            {
              label: 'Out',
              stroke: '#a371f7',
              fill: 'rgba(163,113,247,0.22)',
              fillTo: 0,
              width: 1.5,
              spanGaps: false,
            },
          );
          bands.push(
            { series: [1, 2], fill: 'rgba(63,185,80,0.14)' },
            { series: [3, 4], fill: 'rgba(163,113,247,0.14)' },
          );
        }
      }

      const base = payload.baseline;
      if (base && base.has_data && base.series) {
        const bin = mode === 'pct' ? (base.series.in_util_pct || []) : (base.series.in_bps || []);
        const bout = mode === 'pct' ? (base.series.out_util_pct || []) : (base.series.out_bps || []);
        if (hasAny(bin) || hasAny(bout)) {
          cols.push(bin, negateSeries(bout));
          seriesOpts.push(
            { label: 'In baseline', stroke: 'rgba(63,185,80,0.55)', width: 1, dash: [4, 4], points: { show: false }, spanGaps: false },
            { label: 'Out baseline', stroke: 'rgba(163,113,247,0.55)', width: 1, dash: [4, 4], points: { show: false }, spanGaps: false },
          );
        }
      }

      const data = mergeTime(cols);
      if (!data[0] || data[0].length === 0) {
        renderEmpty('traffic', 'No traffic samples in this range');
        return;
      }
      const opts = baseOpts(
        'traffic',
        300,
        mode === 'pct' ? '% utilization (out down)' : 'bits/s (out down)',
        fmtY,
        { y: { auto: true } },
        { absOut: true }
      );
      opts.series = seriesOpts;
      if (bands.length) opts.bands = bands;
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
      let sub = fmtSpeed(payload.speed_bps);
      if (payload.band && payload.band.has_data) {
        sub += ' · p10/p90 ' + (payload.band.window_days || '?') + 'd ' + (payload.band.timezone || '');
      } else if (bandEl.checked) {
        sub += ' · seasonality: waiting for history';
      }
      if (payload.baseline) {
        sub += payload.baseline.has_data
          ? (' · baseline: ' + (payload.baseline.label || payload.baseline.shift))
          : ' · baseline: no data yet';
      }
      speedLine.textContent = sub;
      drawTraffic(payload);
      drawErrors(payload);
      drawOper(payload);
    }
    async function load() {
      statusEl.textContent = 'loading…';
      statusEl.classList.remove('err');
      try {
        const params = new URLSearchParams();
        params.set('range', range);
        if (!bandEl.checked) params.set('band', '0');
        if (baselineEl.value) params.set('baseline', baselineEl.value);
        const res = await fetch(SERIES_URL + '?' + params.toString());
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
    bandEl.addEventListener('change', () => load());
    baselineEl.addEventListener('change', () => load());
    window.addEventListener('resize', () => { if (lastPayload) render(lastPayload); });
    load();
    setInterval(load, 30000);
  </script>
</body>
</html>
`))

var opticsTemplate = template.Must(template.New("optics").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Device}} {{.Interface}} optics — NetSpecGraph</title>
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
    header .nav { display:flex; gap:1rem; align-items:center; }
    header a { color:var(--accent); text-decoration:none; }
    .toolbar { display:flex; flex-wrap:wrap; gap:0.5rem; align-items:center; padding:0.75rem 1.25rem; border-bottom:1px solid var(--bd); }
    .toolbar button {
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
    .chart { width:100%; min-height:200px; position:relative; }
    .empty { color:var(--muted); font-size:0.9rem; padding:1.5rem 0; text-align:center; }
    .legend { display:flex; gap:1rem; flex-wrap:wrap; font-size:0.78rem; color:var(--muted); margin-bottom:0.35rem; }
    .legend i { display:inline-block; width:10px; height:10px; border-radius:2px; margin-right:0.35rem; }
    .legend i.dash {
      height:0; border-radius:0; border-top:2px dashed; background:transparent; vertical-align:middle;
    }
    .uplot-tip {
      display:none; position:absolute; z-index:10; pointer-events:none;
      background:rgba(22,27,34,0.96); border:1px solid var(--bd); border-radius:6px;
      padding:0.45rem 0.6rem; font:500 0.75rem/1.45 'JetBrains Mono',monospace;
      color:var(--fg); white-space:nowrap; box-shadow:0 4px 16px rgba(0,0,0,0.35);
    }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>{{.Device}} <span>{{.Interface}}</span> · optics</h1>
      <div class="meta">NetSpecGraph DOM · {{.Timezone}} · v{{.Version}}</div>
    </div>
    <div class="nav">
      <a href="/fleet">Fleet</a>
      <a href="{{.TrafficPath}}">← Traffic</a>
      {{if .NetSpecDeviceURL}}<a href="{{.NetSpecDeviceURL}}">NetSpec device</a>{{end}}
      <a href="/">All interfaces</a>
    </div>
  </header>
  <div class="toolbar">
    <button type="button" data-range="1h">1h</button>
    <button type="button" data-range="6h" class="active">6h</button>
    <button type="button" data-range="24h">24h</button>
    <span id="status" class="status">loading…</span>
  </div>
  <main>
    <section>
      <h2>Optical power</h2>
      <p class="sub" id="powerSub">rx / tx dBm · dashed lines = typical warn thresholds (visual only)</p>
      <div class="legend" id="powerLegend">
        <span><i style="background:#3fb950"></i>Rx</span>
        <span><i style="background:#a371f7"></i>Tx</span>
      </div>
      <div id="power" class="chart"></div>
    </section>
    <section>
      <h2>Laser bias</h2>
      <p class="sub">mA</p>
      <div id="bias" class="chart"></div>
    </section>
    <section>
      <h2>Temperature</h2>
      <p class="sub" id="tempSub">°C</p>
      <div class="legend" id="tempLegend">
        <span><i style="background:#f85149"></i>Temp</span>
      </div>
      <div id="temp" class="chart"></div>
    </section>
  </main>
  <script src="https://cdn.jsdelivr.net/npm/uplot@1.6.31/dist/uPlot.iife.min.js"></script>
  <script>
    const SERIES_URL = {{.SeriesURLJS}};
    const statusEl = document.getElementById('status');
    let range = '6h';
    let plots = {};
    let lastPayload = null;

    function fmt(v, unit) {
      if (v == null || !isFinite(v)) return '—';
      return v.toFixed(2) + (unit ? (' ' + unit) : '');
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
    function constSeries(times, value) {
      if (value == null || !times || !times.length) return [];
      return times.map(t => ({ t: t, v: value }));
    }
    function hasAny(points) {
      return (points || []).some(p => p.v != null);
    }
    function destroy(name) {
      if (plots[name]) { try { plots[name].destroy(); } catch (e) {} plots[name] = null; }
      const el = document.getElementById(name);
      if (el) el.innerHTML = '';
    }
    function renderEmpty(id, msg) {
      destroy(id);
      document.getElementById(id).innerHTML = '<div class="empty">' + msg + '</div>';
    }
    function tooltipPlugin(fmtY) {
      let tip, root;
      return {
        hooks: {
          init: [u => {
            root = u.root && u.root.parentElement;
            if (!root) return;
            tip = document.createElement('div');
            tip.className = 'uplot-tip';
            root.appendChild(tip);
          }],
          setCursor: [u => {
            if (!tip || !u.cursor || !u.data || !u.data[0]) return;
            const { left, top, idx } = u.cursor;
            if (idx == null || idx < 0 || left < 0 || top < 0) { tip.style.display = 'none'; return; }
            const ts = u.data[0][idx];
            if (ts == null) { tip.style.display = 'none'; return; }
            let html = '<div class="t">' + new Date(ts * 1000).toLocaleString() + '</div>';
            for (let i = 1; i < u.series.length; i++) {
              if (!u.series[i] || u.series[i].show === false) continue;
              const label = u.series[i].label || ('s'+i);
              const v = u.data[i] ? u.data[i][idx] : null;
              html += '<div><span style="color:' + (u.series[i].stroke || '#e6edf3') + '">' + label + '</span>: ' + fmtY(v) + '</div>';
            }
            tip.innerHTML = html;
            tip.style.display = 'block';
            tip.style.transform = 'translate(' + (left + 14) + 'px,' + (top + 14) + 'px)';
          }]
        }
      };
    }
    function baseOpts(id, height, yLabel, fmtY) {
      const el = document.getElementById(id);
      return {
        width: Math.max(320, (el && el.clientWidth) || 320),
        height: height,
        axes: [
          { stroke: '#8b949e', grid: { stroke: '#21262d' }, ticks: { stroke: '#30363d' } },
          {
            label: yLabel, labelSize: 13, size: 64, gap: 6,
            stroke: '#8b949e', grid: { stroke: '#21262d' }, ticks: { stroke: '#30363d' },
            values: (u, vals) => vals.map(v => Number.isFinite(v) ? fmtY(v) : ''),
          }
        ],
        series: [{}],
        legend: { show: false },
        plugins: [tooltipPlugin(fmtY)],
      };
    }
    function drawPower(payload) {
      const rx = payload.series.rx_power_dbm || [];
      const tx = payload.series.tx_power_dbm || [];
      if (!hasAny(rx) && !hasAny(tx)) {
        renderEmpty('power', payload.has_data ? 'No power samples' : 'No optics data for this interface');
        return;
      }
      destroy('power');
      const th = payload.thresholds || {};
      const times = (rx.length ? rx : tx).map(p => p.t);
      const cols = [rx, tx];
      const series = [
        {},
        { label: 'Rx', stroke: '#3fb950', width: 2, spanGaps: false },
        { label: 'Tx', stroke: '#a371f7', width: 2, spanGaps: false },
      ];
      const legendBits = [
        '<span><i style="background:#3fb950"></i>Rx</span>',
        '<span><i style="background:#a371f7"></i>Tx</span>',
      ];
      const thrRows = [
        ['Rx high', th.rx_high_dbm, 'rgba(248,81,73,0.55)'],
        ['Tx high', th.tx_high_dbm, 'rgba(210,153,34,0.55)'],
        ['Tx low', th.tx_low_dbm, 'rgba(210,153,34,0.85)'],
        ['Rx low', th.rx_low_dbm, 'rgba(248,81,73,0.85)'],
      ];
      thrRows.forEach(row => {
        if (row[1] == null) return;
        cols.push(constSeries(times, row[1]));
        series.push({ label: row[0], stroke: row[2], width: 1, dash: [6, 4], points: { show: false }, spanGaps: true });
        legendBits.push('<span><i class="dash" style="border-color:' + row[2] + '"></i>' + row[0] + ' (' + Number(row[1]).toFixed(1) + ' dBm)</span>');
      });
      document.getElementById('powerLegend').innerHTML = legendBits.join('');
      const opts = baseOpts('power', 240, 'dBm', v => fmt(v, 'dBm'));
      opts.series = series;
      try {
        plots.power = new uPlot(opts, mergeTime(cols), document.getElementById('power'));
        document.getElementById('powerSub').textContent =
          'rx / tx dBm · thresholds: ' + (payload.threshold_profile || 'default') + ' (visual only, not alerts)';
      } catch (e) {
        renderEmpty('power', 'Chart error: ' + (e && e.message ? e.message : e));
      }
    }
    function drawBias(payload) {
      const s = payload.series.laser_bias_ma || [];
      if (!hasAny(s)) { renderEmpty('bias', 'No bias samples'); return; }
      destroy('bias');
      const opts = baseOpts('bias', 180, 'mA', v => fmt(v, 'mA'));
      opts.series = [{}, { label: 'bias', stroke: '#58a6ff', width: 2, fill: 'rgba(88,166,255,0.12)', spanGaps: false }];
      try { plots.bias = new uPlot(opts, toUplot(s), document.getElementById('bias')); }
      catch (e) { renderEmpty('bias', 'Chart error: ' + (e && e.message ? e.message : e)); }
    }
    function drawTemp(payload) {
      const s = payload.series.temp_celsius || [];
      if (!hasAny(s)) { renderEmpty('temp', 'No temperature samples'); return; }
      destroy('temp');
      const th = payload.thresholds || {};
      const cols = [s];
      const series = [{}, { label: 'Temp', stroke: '#f85149', width: 2, fill: 'rgba(248,81,73,0.1)', spanGaps: false }];
      const legendBits = ['<span><i style="background:#f85149"></i>Temp</span>'];
      if (th.temp_high_c != null) {
        cols.push(constSeries(s.map(p => p.t), th.temp_high_c));
        series.push({ label: 'Temp high', stroke: 'rgba(248,81,73,0.6)', width: 1, dash: [6, 4], points: { show: false }, spanGaps: true });
        legendBits.push('<span><i class="dash" style="border-color:rgba(248,81,73,0.6)"></i>Temp high (' + Number(th.temp_high_c).toFixed(0) + ' °C)</span>');
      }
      document.getElementById('tempLegend').innerHTML = legendBits.join('');
      const opts = baseOpts('temp', 180, '°C', v => fmt(v, '°C'));
      opts.series = series;
      try {
        plots.temp = new uPlot(opts, mergeTime(cols), document.getElementById('temp'));
        document.getElementById('tempSub').textContent = '°C · dashed = high reference (visual only)';
      } catch (e) { renderEmpty('temp', 'Chart error: ' + (e && e.message ? e.message : e)); }
    }
    function render(payload) {
      lastPayload = payload;
      drawPower(payload);
      drawBias(payload);
      drawTemp(payload);
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
          : 'no optics data yet';
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
    window.addEventListener('resize', () => { if (lastPayload) render(lastPayload); });
    load();
    setInterval(load, 30000);
  </script>
</body>
</html>
`))

var fleetTemplate = template.Must(template.New("fleet").Funcs(template.FuncMap{
	"fmtBPS": FormatBPS,
	"fmtPct": FormatUtilPct,
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Fleet — NetSpecGraph</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg:#0d1117; --bg2:#161b22; --bg3:#21262d; --bd:#30363d; --fg:#e6edf3; --muted:#8b949e;
      --accent:#58a6ff; --green:#3fb950; --red:#f85149; --yellow:#d29922;
    }
    * { box-sizing: border-box; }
    body { margin:0; font-family:'Outfit',system-ui,sans-serif; background:var(--bg); color:var(--fg); min-height:100vh; }
    header { display:flex; flex-wrap:wrap; gap:0.75rem 1.25rem; align-items:baseline; justify-content:space-between;
      padding:1rem 1.25rem; border-bottom:1px solid var(--bd); background:var(--bg2); }
    header h1 { margin:0; font-size:1.25rem; font-weight:600; }
    header .meta { font-family:'JetBrains Mono',monospace; font-size:0.78rem; color:var(--muted); }
    header .nav { display:flex; gap:1rem; align-items:center; }
    a { color:var(--accent); text-decoration:none; }
    main { padding:1rem 1.25rem 2.5rem; max-width:1200px; margin:0 auto; display:grid; gap:1rem; }
    .filters { display:grid; gap:0.75rem; grid-template-columns:repeat(4,minmax(0,1fr)); padding:1rem;
      background:var(--bg2); border:1px solid var(--bd); border-radius:10px; }
    @media (max-width:900px) { .filters { grid-template-columns:1fr 1fr; } }
    label { font-size:0.8rem; color:var(--muted); display:block; }
    select, input[type=text], input[type=number] {
      width:100%; margin-top:0.3rem; padding:0.5rem 0.65rem; background:var(--bg); border:1px solid var(--bd);
      border-radius:6px; color:var(--fg); font:500 0.85rem 'JetBrains Mono',monospace;
    }
    button { margin-top:1.35rem; padding:0.55rem 0.9rem; background:var(--green); color:#0d1117; border:none;
      border-radius:6px; font:600 0.9rem 'Outfit',sans-serif; cursor:pointer; }
    .grid { display:grid; grid-template-columns:1.4fr 1fr; gap:1rem; align-items:start; }
    @media (max-width:960px) { .grid { grid-template-columns:1fr; } }
    .panel { background:var(--bg2); border:1px solid var(--bd); border-radius:10px; overflow:hidden; }
    .panel h2 { margin:0; padding:0.75rem 1rem; font-size:0.95rem; background:var(--bg3); border-bottom:1px solid var(--bd); }
    .panel .body { padding:0.75rem 1rem 1rem; }
    table { width:100%; border-collapse:collapse; font-size:0.8rem; }
    th, td { text-align:left; padding:0.4rem 0.45rem; border-bottom:1px solid var(--bd); vertical-align:top; }
    th { color:var(--muted); font-weight:500; }
    td.mono, .mono { font-family:'JetBrains Mono',monospace; font-size:0.75rem; }
    td.num { text-align:right; font-family:'JetBrains Mono',monospace; font-size:0.75rem; }
    .sev-ok { color:var(--green); }
    .sev-warning { color:var(--yellow); }
    .sev-critical { color:var(--red); font-weight:600; }
    .hex-map-svg { max-height:320px; width:100%; display:block; }
    .hex-link { outline:none; }
    .hex-shape { cursor:pointer; transition:filter 0.15s ease; }
    .hex-link:hover .hex-shape { filter:brightness(1.12); }
    .hex-empty, .empty { color:var(--muted); padding:1rem 0; font-size:0.9rem; }
    .legend { display:flex; gap:1rem; flex-wrap:wrap; font-size:0.75rem; color:var(--muted); margin-top:0.65rem; }
    .legend i { display:inline-block; width:10px; height:10px; border-radius:2px; margin-right:0.3rem; vertical-align:middle; }
    .count { font-size:0.78rem; color:var(--muted); padding:0 1rem 0.5rem; }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Fleet utilization</h1>
      <div class="meta">NetSpecGraph · {{.Timezone}} · v{{.Version}} · rate window {{if .Snapshot}}{{.Snapshot.Range}}{{else}}5m{{end}}</div>
    </div>
    <div class="nav">
      <a href="/">Browse</a>
      {{if .NetSpecPublicURL}}<a href="{{.NetSpecPublicURL}}/noc">NetSpec NOC</a>{{end}}
    </div>
  </header>
  <main>
    <form class="filters" method="GET" action="/fleet">
      <div>
        <label for="port_role">Port role</label>
        <select id="port_role" name="port_role">
          <option value="all" {{if eq .PortRole ""}}selected{{end}}>Any</option>
          {{range .PortRoles}}
          <option value="{{.Label}}" {{if eq $.PortRole .Label}}selected{{end}}>{{.Label}} ({{.Count}})</option>
          {{end}}
        </select>
      </div>
      <div>
        <label for="device_prefix">Device prefix</label>
        <select id="device_prefix" name="device_prefix">
          <option value="">Any</option>
          {{range .DeviceRoles}}
          <option value="{{.Prefix}}" {{if eq $.DevicePrefix .Prefix}}selected{{end}}>{{.Prefix}} — {{.Name}}</option>
          {{end}}
        </select>
      </div>
      <div>
        <label for="device">Device</label>
        <input id="device" name="device" type="text" value="{{.Device}}" placeholder="e.g. csw-mcd-01" autocomplete="off">
      </div>
      <div>
        <label for="limit">Top N</label>
        <input id="limit" name="limit" type="number" min="5" max="200" value="{{.Limit}}">
        <button type="submit">Apply</button>
      </div>
    </form>

    <div class="grid">
      <div class="panel">
        <h2>Top talkers</h2>
        {{if .Snapshot}}
        <div class="count">{{.Snapshot.Count}} interfaces{{if .PortRole}} · {{.PortRole}}{{end}}</div>
        {{if .Snapshot.Talkers}}
        <table>
          <thead>
            <tr>
              <th>Device</th>
              <th>Interface</th>
              <th class="num">In</th>
              <th class="num">Out</th>
              <th class="num">Util</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {{range .Snapshot.Talkers}}
            <tr>
              <td class="mono">{{.Device}}</td>
              <td class="mono"><a href="{{.GraphPath}}">{{.Interface}}</a>{{if .Alias}}<div style="color:var(--muted);font-size:0.7rem">{{.Alias}}</div>{{end}}</td>
              <td class="num">{{fmtBPS .InBPS}}</td>
              <td class="num">{{fmtBPS .OutBPS}}</td>
              <td class="num">{{fmtPct .UtilPct}}</td>
              <td>{{if .NetSpecPath}}<a href="{{.NetSpecPath}}">NetSpec</a>{{end}}</td>
            </tr>
            {{end}}
          </tbody>
        </table>
        {{else}}
        <div class="body empty">No interfaces with traffic for this filter.</div>
        {{end}}
        {{else}}
        <div class="body empty">No snapshot.</div>
        {{end}}
      </div>
      <div class="panel">
        <h2>Utilization honeycomb</h2>
        <div class="body">
          {{.HexSVG}}
          <div class="legend">
            <span><i style="background:none;border:1.5px solid #3fb950"></i>&lt;50%</span>
            <span><i style="background:#d29922"></i>50–80%</span>
            <span><i style="background:#f85149"></i>≥80%</span>
          </div>
          <p class="empty" style="padding-top:0.5rem;font-size:0.78rem">Hex color = worst util among filtered ports on that device. Click a tile to filter the table.</p>
        </div>
      </div>
    </div>
  </main>
</body>
</html>
`))
