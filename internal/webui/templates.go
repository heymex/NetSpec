package webui

import (
	"html/template"
	"net/url"
)

// Templates contains all HTML templates for the web UI
var Templates = template.Must(template.New("").Funcs(template.FuncMap{
	"levelClass": func(level string) string {
		switch level {
		case "error", "fatal":
			return "log-error"
		case "warn":
			return "log-warn"
		case "debug":
			return "log-debug"
		default:
			return "log-info"
		}
	},
	"add": func(a, b, c uint64) uint64 {
		return a + b + c
	},
	"queryEscape": url.QueryEscape,
}).Parse(`
{{define "base"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>NetSpec Status</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --border-color: #30363d;
            --text-primary: #e6edf3;
            --text-secondary: #8b949e;
            --text-muted: #6e7681;
            --accent-green: #3fb950;
            --accent-green-dim: #238636;
            --accent-red: #f85149;
            --accent-yellow: #d29922;
            --accent-blue: #58a6ff;
            --accent-purple: #a371f7;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'Outfit', -apple-system, BlinkMacSystemFont, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.6;
            min-height: 100vh;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 2rem;
        }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 2rem;
            padding-bottom: 1.5rem;
            border-bottom: 1px solid var(--border-color);
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }

        .logo-icon {
            width: 40px;
            height: 40px;
            background: linear-gradient(135deg, var(--accent-green) 0%, var(--accent-blue) 100%);
            border-radius: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 700;
            font-size: 1.2rem;
        }

        h1 {
            font-size: 1.75rem;
            font-weight: 600;
            background: linear-gradient(135deg, var(--text-primary) 0%, var(--text-secondary) 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .header-actions {
            display: flex;
            gap: 1rem;
            align-items: center;
        }

        .status-badge {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.5rem 1rem;
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 20px;
            font-size: 0.875rem;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--accent-green);
            animation: pulse 2s infinite;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        .btn {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.625rem 1.25rem;
            border: none;
            border-radius: 8px;
            font-family: inherit;
            font-size: 0.875rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
        }

        .btn-primary {
            background: var(--accent-green-dim);
            color: var(--text-primary);
            border: 1px solid var(--accent-green);
        }

        .btn-primary:hover {
            background: var(--accent-green);
            transform: translateY(-1px);
        }

        .btn-secondary {
            background: var(--bg-tertiary);
            color: var(--text-primary);
            border: 1px solid var(--border-color);
        }

        .btn-secondary:hover {
            background: var(--border-color);
        }
        .btn-danger {
            background: rgba(248, 81, 73, 0.15);
            color: var(--accent-red);
            border: 1px solid var(--accent-red);
        }
        .btn-danger:hover {
            background: rgba(248, 81, 73, 0.25);
        }

        .grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1.5rem;
            margin-bottom: 1.5rem;
        }

        @media (max-width: 1024px) {
            .grid { grid-template-columns: 1fr; }
        }

        .hex-overview-row {
            margin-bottom: 1.5rem;
        }

        .hex-overview-card .card-body {
            padding: 1rem 1.25rem;
            background: var(--bg-primary);
        }

        #hex-overview-root {
            min-height: 120px;
            display: flex;
            justify-content: center;
            align-items: center;
        }

        .hex-map-svg {
            max-height: 280px;
            width: 100%;
            display: block;
        }

        .hex-link {
            outline: none;
        }

        .hex-shape {
            transition: filter 0.15s ease, opacity 0.15s ease;
            cursor: pointer;
        }

        .hex-link:hover .hex-shape {
            filter: brightness(1.12);
        }

        .hex-overview-empty {
            text-align: center;
            color: var(--text-muted);
            padding: 1.5rem;
            font-size: 0.875rem;
        }

        .card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            overflow: hidden;
        }

        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 1rem 1.25rem;
            background: var(--bg-tertiary);
            border-bottom: 1px solid var(--border-color);
        }

        .card-title {
            font-size: 1rem;
            font-weight: 600;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .card-body {
            padding: 1rem 1.25rem;
        }

        .card-body.no-padding {
            padding: 0;
        }

        .device-list {
            list-style: none;
        }

        .device-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 1rem 1.25rem;
            border-bottom: 1px solid var(--border-color);
            transition: background 0.15s ease;
        }

        .device-item:last-child {
            border-bottom: none;
        }

        .device-item:hover {
            background: var(--bg-tertiary);
        }

        .device-info h3 {
            font-size: 0.9375rem;
            font-weight: 500;
            margin-bottom: 0.25rem;
        }

        .device-meta {
            display: flex;
            gap: 1rem;
            font-size: 0.8125rem;
            color: var(--text-secondary);
        }

        .device-meta span {
            display: flex;
            align-items: center;
            gap: 0.375rem;
        }

        .interface-count {
            background: var(--bg-tertiary);
            padding: 0.375rem 0.75rem;
            border-radius: 6px;
            font-size: 0.8125rem;
            color: var(--text-secondary);
            font-family: 'JetBrains Mono', monospace;
        }

        .log-container {
            height: 400px;
            overflow-y: auto;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.8125rem;
            background: var(--bg-primary);
        }

        .log-entry {
            padding: 0.5rem 1rem;
            border-bottom: 1px solid var(--bg-tertiary);
            display: flex;
            gap: 1rem;
        }

        .log-time {
            color: var(--text-muted);
            white-space: nowrap;
        }

        .log-level {
            text-transform: uppercase;
            font-weight: 600;
            min-width: 50px;
        }

        .log-info .log-level { color: var(--accent-blue); }
        .log-warn .log-level { color: var(--accent-yellow); }
        .log-error .log-level { color: var(--accent-red); }
        .log-debug .log-level { color: var(--text-muted); }

        .log-message {
            color: var(--text-secondary);
            word-break: break-word;
        }

        .alert-list {
            list-style: none;
        }

        .alert-item {
            display: flex;
            align-items: flex-start;
            gap: 1rem;
            padding: 1rem 1.25rem;
            border-bottom: 1px solid var(--border-color);
        }

        .alert-item:last-child {
            border-bottom: none;
        }

        .alert-severity {
            padding: 0.25rem 0.625rem;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 600;
            text-transform: uppercase;
        }

        .alert-severity.critical {
            background: rgba(248, 81, 73, 0.15);
            color: var(--accent-red);
        }

        .alert-severity.warning {
            background: rgba(210, 153, 34, 0.15);
            color: var(--accent-yellow);
        }

        .alert-severity.info {
            background: rgba(88, 166, 255, 0.15);
            color: var(--accent-blue);
        }

        .alert-content h4 {
            font-size: 0.875rem;
            font-weight: 500;
            margin-bottom: 0.25rem;
        }

        .alert-content p {
            font-size: 0.8125rem;
            color: var(--text-secondary);
        }

        .empty-state {
            padding: 3rem 2rem;
            text-align: center;
            color: var(--text-muted);
        }

        .empty-state svg {
            width: 48px;
            height: 48px;
            margin-bottom: 1rem;
            opacity: 0.5;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 1rem;
            margin-bottom: 1.5rem;
        }

        @media (max-width: 768px) {
            .stats-grid { grid-template-columns: repeat(2, 1fr); }
        }

        .stat-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 10px;
            padding: 1.25rem;
        }

        .stat-label {
            font-size: 0.8125rem;
            color: var(--text-secondary);
            margin-bottom: 0.5rem;
        }

        .stat-value {
            font-size: 1.75rem;
            font-weight: 600;
            font-family: 'JetBrains Mono', monospace;
        }

        .stat-value.green { color: var(--accent-green); }
        .stat-value.yellow { color: var(--accent-yellow); }
        .stat-value.red { color: var(--accent-red); }
        .stat-value.blue { color: var(--accent-blue); }

        .config-details {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.8125rem;
        }

        .telemetry-mini {
            display: grid;
            grid-template-columns: repeat(4, minmax(0, 1fr));
            gap: 0.5rem;
            margin-bottom: 0.75rem;
        }

        .telemetry-pill {
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 0.5rem;
            background: var(--bg-primary);
        }

        .telemetry-pill .k {
            font-size: 0.72rem;
            color: var(--text-secondary);
        }

        .telemetry-pill .v {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.95rem;
            margin-top: 0.15rem;
        }

        .config-row {
            display: flex;
            justify-content: space-between;
            padding: 0.75rem 0;
            border-bottom: 1px solid var(--border-color);
        }

        .config-row:last-child {
            border-bottom: none;
        }

        .config-key {
            color: var(--text-secondary);
        }

        .config-value {
            color: var(--accent-blue);
        }

        .toast {
            position: fixed;
            bottom: 2rem;
            right: 2rem;
            max-width: min(520px, calc(100vw - 3rem));
            padding: 1rem 1.5rem;
            background: var(--bg-secondary);
            border: 1px solid var(--accent-green);
            border-radius: 8px;
            display: none;
            animation: slideIn 0.3s ease;
            white-space: pre-wrap;
            word-break: break-word;
            font-size: 0.875rem;
            z-index: 1000;
        }

        .toast.show {
            display: block;
        }

        .toast.error {
            border-color: var(--accent-red);
        }

        @keyframes slideIn {
            from {
                transform: translateY(20px);
                opacity: 0;
            }
            to {
                transform: translateY(0);
                opacity: 1;
            }
        }

        /* SNMP operator banners (fallback / snmp-only telemetry) */
        .snmp-banner-stack {
            display: flex;
            flex-direction: column;
            gap: 0.75rem;
            margin-bottom: 1.25rem;
        }
        .snmp-banner {
            border-radius: 10px;
            padding: 0.85rem 1.1rem;
            border: 1px solid var(--border-color);
            line-height: 1.45;
        }
        .snmp-banner-title {
            font-weight: 600;
            font-size: 0.95rem;
            margin-bottom: 0.35rem;
        }
        .snmp-banner-body {
            font-size: 0.85rem;
            color: var(--text-secondary);
        }
        .snmp-banner.snmp-banner-warning {
            background: rgba(210, 153, 34, 0.12);
            border-color: rgba(210, 153, 34, 0.45);
        }
        .snmp-banner.snmp-banner-warning .snmp-banner-title {
            color: var(--accent-yellow);
        }
        .snmp-banner.snmp-banner-info {
            background: rgba(88, 166, 255, 0.08);
            border-color: rgba(88, 166, 255, 0.35);
        }
        .snmp-banner.snmp-banner-info .snmp-banner-title {
            color: var(--accent-blue);
        }
    </style>
</head>
<body>
    <div class="container">
        {{template "content" .}}
    </div>
    <div id="toast" class="toast"></div>
    <script>
        function showToast(message, isError, durationMs) {
            const toast = document.getElementById('toast');
            toast.textContent = message;
            toast.className = 'toast show' + (isError ? ' error' : '');
            const ms = durationMs != null ? durationMs : 3000;
            setTimeout(() => toast.className = 'toast', ms);
        }

        function formatLocalTimestamp(value, withDate) {
            const d = new Date(value);
            if (Number.isNaN(d.getTime())) return value;
            if (withDate) return d.toLocaleString();
            return d.toLocaleTimeString();
        }

        function localizeTimestamps(root) {
            (root || document).querySelectorAll('[data-local-ts]').forEach(el => {
                const raw = el.getAttribute('data-local-ts');
                const mode = el.getAttribute('data-local-ts-mode') || 'datetime';
                el.textContent = formatLocalTimestamp(raw, mode !== 'time');
            });
        }

        async function reloadConfig() {
            const btn = event.target;
            btn.disabled = true;
            btn.textContent = 'Reloading...';
            try {
                const res = await fetch('/api/reload', { method: 'POST' });
                const data = await res.json();
                if (res.ok) {
                    showToast('Configuration reloaded successfully');
                    setTimeout(() => location.reload(), 1000);
                } else {
                    showToast(data.error || 'Failed to reload', true);
                }
            } catch (e) {
                showToast('Failed to reload: ' + e.message, true);
            }
            btn.disabled = false;
            btn.textContent = '↻ Reload Config';
        }

        async function testNotifications(ev) {
            const btn = ev && ev.target;
            const prev = btn && btn.textContent;
            if (btn) {
                btn.disabled = true;
                btn.textContent = 'Testing…';
            }
            try {
                const res = await fetch('/api/notifications/test', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: '{}',
                });
                let data = {};
                try {
                    data = await res.json();
                } catch (_) {
                    showToast('Notification test failed: invalid JSON response', true);
                    return;
                }
                const outcomes = Array.isArray(data.outcomes) ? data.outcomes : [];
                let summary;
                if (outcomes.length) {
                    summary = outcomes
                        .map(function (o) {
                            const st = o.ok ? 'OK' : 'FAIL';
                            const msg = o.message ? ' — ' + o.message : '';
                            return o.channel + ': ' + st + msg;
                        })
                        .join('\n');
                } else {
                    summary = data.error || 'Notification test returned no channel outcomes.';
                }
                const uiOk = res.ok && data.all_ok === true;
                showToast(summary, !uiOk, outcomes.length > 1 ? 9000 : 5000);
            } catch (e) {
                showToast('Notification test failed: ' + e.message, true);
            } finally {
                if (btn) {
                    btn.disabled = false;
                    btn.textContent = prev || 'Test alerts';
                }
            }
        }

        localizeTimestamps(document);
        // Auto-refresh logs every 5 seconds
        setInterval(() => {
            fetch('/api/logs')
                .then(r => r.json())
                .then(data => {
                    const container = document.querySelector('.log-container');
                    if (container && data.entries) {
                        const wasAtBottom = container.scrollHeight - container.scrollTop <= container.clientHeight + 50;
                        container.innerHTML = data.entries.map(e => 
                            '<div class="log-entry log-' + e.level + '">' +
                            '<span class="log-time">' + formatLocalTimestamp(e.timestamp, false) + '</span>' +
                            '<span class="log-level">' + e.level + '</span>' +
                            '<span class="log-message">' + escapeHtml(e.message) + '</span>' +
                            '</div>'
                        ).join('');
                        if (wasAtBottom) container.scrollTop = container.scrollHeight;
                    }
                });
        }, 5000);

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
    </script>
</body>
</html>
{{end}}

{{define "content"}}
        <header>
            <div class="logo">
                <div class="logo-icon">N</div>
                <div>
                    <h1>NetSpec</h1>
                    <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">
                        {{if .Version}}{{.Version}}{{if ne .Commit "unknown"}} <span style="color: var(--text-muted);">({{.Commit | printf "%.7s"}})</span>{{end}}{{else}}dev{{end}}
                    </div>
                </div>
            </div>
            <div class="header-actions">
                <div class="status-badge">
                    <span class="status-dot"></span>
                    Running
                </div>
                <a class="btn btn-secondary" href="/api-browser">API</a>
                <a class="btn btn-secondary" href="/diagnostics">Diagnostics</a>
                <a class="btn btn-secondary" href="/wizard">+ Add Device</a>
                <button type="button" class="btn btn-secondary" onclick="testNotifications(event)">🔔 Test alerts</button>
                <button class="btn btn-primary" onclick="reloadConfig()">↻ Reload Config</button>
            </div>
        </header>

        {{template "snmp-banner-stack" .}}

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Devices</div>
                <div class="stat-value blue">{{.DeviceCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Interfaces</div>
                <div class="stat-value blue">{{.InterfaceCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Active Alerts</div>
                <div class="stat-value {{if gt .AlertCount 0}}red{{else}}green{{end}}">{{.AlertCount}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Uptime</div>
                <div class="stat-value green">{{.Uptime}}</div>
            </div>
        </div>

        <div class="hex-overview-row">
            <div class="card hex-overview-card">
                <div class="card-header">
                    <span class="card-title">⬡ Host Overview</span>
                </div>
                <div class="card-body">
                    <div id="hex-overview-root">{{.HexMapSVG}}</div>
                </div>
            </div>
        </div>

        <div class="grid">
            <div class="card">
                <div class="card-header">
                    <span class="card-title">📡 Monitored Devices</span>
                </div>
                <div class="card-body no-padding">
                    {{if .Devices}}
                    <ul class="device-list">
                        {{range .Devices}}
                        <li class="device-item" onclick="window.location.href='/device/{{.Name}}'" style="cursor: pointer;">
                            <div class="device-info">
                                <h3>{{.Name}}</h3>
                                <div class="device-meta">
                                    <span>{{.Address}}</span>
                                    {{if .Description}}<span>{{.Description}}</span>{{end}}
                                </div>
                            </div>
                            <span class="interface-count">{{.InterfaceCount}} ifaces</span>
                        </li>
                        {{end}}
                    </ul>
                    {{else}}
                    <div class="empty-state">
                        <p>No devices configured</p>
                    </div>
                    {{end}}
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <span class="card-title">🚨 Active Alerts</span>
                </div>
                <div class="card-body no-padding">
                    {{if .Alerts}}
                    <ul class="alert-list">
                        {{range .Alerts}}
                        <li class="alert-item">
                            <span class="alert-severity {{.Severity}}">{{.Severity}}</span>
                            <div class="alert-content">
                                <h4>{{.Device}} - {{.Entity}}</h4>
                                <p>{{.Message}}</p>
                            </div>
                        </li>
                        {{end}}
                    </ul>
                    {{else}}
                    <div class="empty-state">
                        <p>✓ No active alerts</p>
                    </div>
                    {{end}}
                </div>
            </div>
        </div>

        <div class="grid">
            <div class="card">
                <div class="card-header">
                    <span class="card-title">⚙️ Configuration</span>
                </div>
                <div class="card-body">
                    <div style="margin-bottom: 0.8rem;">
                        <div style="font-size: 0.82rem; color: var(--text-secondary); margin-bottom: 0.4rem;">Inbound Telemetry (Push Ingest)</div>
                        <div class="telemetry-mini">
                            <div class="telemetry-pill"><div class="k">Received</div><div class="v">{{.Telemetry.Received}}</div></div>
                            <div class="telemetry-pill"><div class="k">Accepted</div><div class="v">{{.Telemetry.Accepted}}</div></div>
                            <div class="telemetry-pill"><div class="k">Rejected</div><div class="v">{{add .Telemetry.RejectedInvalidJSON .Telemetry.RejectedAuth .Telemetry.RejectedMissing}}</div></div>
                            <div class="telemetry-pill"><div class="k">Events/Sec</div><div class="v">{{printf "%.1f" .Telemetry.EventsPerSecond}}</div></div>
                        </div>
                        {{if .Telemetry.Listeners}}
                        <div style="margin-top:0.45rem; font-size:0.74rem; color: var(--text-secondary); line-height:1.35;">
                            <div style="margin-bottom:0.2rem;">TCP listeners (port → source tag, same NDJSON format on each)</div>
                            {{range .Telemetry.Listeners}}
                            <div>:{{.Port}}{{if .Source}} <span style="color:var(--text-muted);">({{.Source}})</span>{{end}} — rcv {{.Received}}, ok {{.Accepted}}</div>
                            {{end}}
                        </div>
                        {{end}}
                        {{if .Telemetry.LastEventAt.IsZero}}
                        <div style="font-size:0.75rem;color:var(--accent-yellow);">No accepted telemetry events yet.</div>
                        {{else}}
                        <div style="font-size:0.75rem;color:var(--text-secondary);">Last accepted: <span data-local-ts="{{.Telemetry.LastEventAt.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="datetime"></span></div>
                        {{end}}
                        {{if .Telemetry.UnknownDevices}}
                        <div style="margin-top:0.6rem; border-top:1px solid var(--border-color); padding-top:0.6rem;">
                            <div style="font-size:0.78rem; color: var(--accent-yellow); margin-bottom:0.35rem;">Unknown telemetry sources (not in config)</div>
                            {{range .Telemetry.UnknownDevices}}
                            <div style="display:flex; justify-content:space-between; gap:0.5rem; font-size:0.76rem; margin-bottom:0.2rem;">
                                <span>{{.Device}} ({{.Count}})</span>
                                <a href="{{.WizardURL}}" style="color: var(--accent-blue);">Add via Wizard</a>
                            </div>
                            {{end}}
                        </div>
                        {{end}}
                    </div>
                    <div class="config-details">
                        <div class="config-row">
                            <span class="config-key">Collection Interval</span>
                            <span class="config-value">{{.Config.CollectionInterval}}</span>
                        </div>
                        <div class="config-row">
                            <span class="config-key">Dedup Window</span>
                            <span class="config-value">{{.Config.DedupWindow}}</span>
                        </div>
                        <div class="config-row">
                            <span class="config-key">Config Path</span>
                            <span class="config-value">{{.Config.ConfigPath}}</span>
                        </div>
                        {{if .Version}}
                        <div class="config-row">
                            <span class="config-key">Version</span>
                            <span class="config-value">{{.Version}}</span>
                        </div>
                        {{end}}
                        {{if and .Commit (ne .Commit "unknown")}}
                        <div class="config-row">
                            <span class="config-key">Commit</span>
                            <span class="config-value">{{.Commit}}</span>
                        </div>
                        {{end}}
                        {{if and .BuildDate (ne .BuildDate "unknown")}}
                        <div class="config-row">
                            <span class="config-key">Build Date</span>
                            <span class="config-value">{{.BuildDate}}</span>
                        </div>
                        {{end}}
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <span class="card-title">📋 Recent Logs</span>
                    <button class="btn btn-secondary" onclick="document.querySelector('.log-container').scrollTop = document.querySelector('.log-container').scrollHeight">↓ Latest</button>
                </div>
                <div class="card-body no-padding">
                    <div class="log-container">
                        {{range .Logs}}
                        <div class="log-entry {{levelClass .Level}}">
                            <span class="log-time" data-local-ts="{{.Timestamp.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="time"></span>
                            <span class="log-level">{{.Level}}</span>
                            <span class="log-message">{{.Message}}</span>
                        </div>
                        {{end}}
                    </div>
                </div>
            </div>
        </div>
        <script>
        (function () {
            var HEX_R = 22;
            var HEX_MAX = 64;
            function severityRank(s) {
                var x = String(s || '').toLowerCase().trim();
                if (x === 'critical' || x === 'fatal' || x === 'error' || x === 'unreachable') return 4;
                if (x === 'warning' || x === 'warn') return 3;
                if (x === 'info' || x === 'unknown') return 2;
                return x ? 1 : 0;
            }
            function worstPerDevice(alerts) {
                var m = {};
                for (var i = 0; i < (alerts || []).length; i++) {
                    var a = alerts[i];
                    var d = String(a.Device || '').trim();
                    if (!d) continue;
                    var sev = String(a.Severity || '').trim();
                    if (!(d in m) || severityRank(sev) > severityRank(m[d])) m[d] = sev;
                }
                return m;
            }
            function snmpReachRaw(devices) {
                var m = {};
                for (var i = 0; i < (devices || []).length; i++) {
                    var d = devices[i];
                    if (!d || !d.name) continue;
                    var r = String(d.snmp_reachability || '').toLowerCase().trim();
                    if (r === 'fail') m[d.name] = 'unreachable';
                    else if (r === 'unknown') m[d.name] = 'unknown';
                }
                return m;
            }
            function mergeWorstRaw(a, b) {
                return severityRank(b) > severityRank(a) ? b : a;
            }
            function displayBucket(raw) {
                var x = String(raw || '').toLowerCase().trim();
                if (x === 'critical' || x === 'fatal' || x === 'error' || x === 'unreachable') return 'critical';
                if (x === 'warning' || x === 'warn') return 'warning';
                if (x === 'info' || x === 'unknown') return 'warning';
                if (!raw) return 'ok';
                return 'warning';
            }
            function hexTitle(name, rawWorst) {
                var r = String(rawWorst || '').toLowerCase().trim();
                if (r === 'unreachable') return name + ' — SNMP unreachable';
                if (r === 'unknown') return name + ' — awaiting SNMP';
                if (r === 'critical' || r === 'fatal' || r === 'error') return name + ' — critical';
                if (r === 'warning' || r === 'warn' || r === 'info') return name + ' — warning';
                return name + ' — ok';
            }
            function tileStyle(bucket) {
                if (bucket === 'critical') return { fill: '#f85149', stroke: '#30363d', sw: 1.5, cls: 'hex-critical' };
                if (bucket === 'warning') return { fill: '#d29922', stroke: '#30363d', sw: 1.5, cls: 'hex-warning' };
                return { fill: 'none', stroke: '#3fb950', sw: 1.5, cls: 'hex-ok' };
            }
            function hexPathD(cx, cy, R) {
                var parts = [];
                for (var i = 0; i < 6; i++) {
                    var ang = -Math.PI / 2 + i * Math.PI / 3;
                    var x = cx + R * Math.cos(ang);
                    var y = cy + R * Math.sin(ang);
                    parts.push((i === 0 ? 'M ' : 'L ') + x.toFixed(4) + ' ' + y.toFixed(4));
                }
                return parts.join(' ') + ' Z';
            }
            function escXml(t) {
                return String(t).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
            }
            function buildLayout(names, worst) {
                var sorted = names.slice().sort();
                var list = sorted.slice(0, Math.min(sorted.length, HEX_MAX));
                if (!list.length) return { empty: true };
                var w = HEX_R * Math.sqrt(3);
                var vSpace = HEX_R * 2 * 0.75;
                var cols = Math.max(1, Math.ceil(Math.sqrt(list.length)));
                var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
                var tiles = [];
                for (var i = 0; i < list.length; i++) {
                    var col = i % cols;
                    var row = Math.floor(i / cols);
                    var cx = col * w + (row % 2) * (w / 2);
                    var cy = row * vSpace;
                    var raw = worst[list[i]] || '';
                    var bucket = displayBucket(raw);
                    var st = tileStyle(bucket);
                    tiles.push({ name: list[i], cx: cx, cy: cy, raw: raw, bucket: bucket, fill: st.fill, stroke: st.stroke, sw: st.sw, cls: st.cls });
                    for (var k = 0; k < 6; k++) {
                        var ang = -Math.PI / 2 + k * Math.PI / 3;
                        var px = cx + HEX_R * Math.cos(ang);
                        var py = cy + HEX_R * Math.sin(ang);
                        minX = Math.min(minX, px); minY = Math.min(minY, py);
                        maxX = Math.max(maxX, px); maxY = Math.max(maxY, py);
                    }
                }
                var pad = HEX_R + 6;
                return { empty: false, tiles: tiles, vx: minX - pad, vy: minY - pad, vw: maxX - minX + 2 * pad, vh: maxY - minY + 2 * pad };
            }
            function renderSVG(layout) {
                var root = document.getElementById('hex-overview-root');
                if (!root) return;
                if (layout.empty) {
                    root.innerHTML = '<div class="hex-overview-empty"><p>No devices configured</p></div>';
                    return;
                }
                var html = '<svg xmlns="http://www.w3.org/2000/svg" class="hex-map-svg" viewBox="' + layout.vx + ' ' + layout.vy + ' ' + layout.vw + ' ' + layout.vh + '" preserveAspectRatio="xMidYMid meet" role="img" aria-label="Host overview honeycomb">';
                for (var j = 0; j < layout.tiles.length; j++) {
                    var t = layout.tiles[j];
                    var href = '/device/' + encodeURIComponent(t.name).replace(/'/g, '%27');
                    var title = escXml(hexTitle(t.name, t.raw));
                    var d = hexPathD(t.cx, t.cy, HEX_R);
                    html += '<a class="hex-link" href="' + href + '"><path class="hex-shape ' + t.cls + '" d="' + d + '" fill="' + t.fill + '" stroke="' + t.stroke + '" stroke-width="' + t.sw + '"/><title>' + title + '</title></a>';
                }
                html += '</svg>';
                root.innerHTML = html;
            }
            async function refreshHexOverview() {
                var root = document.getElementById('hex-overview-root');
                if (!root) return;
                try {
                    var dr = await fetch('/api/devices');
                    var ar = await fetch('/alerts');
                    var dj = await dr.json();
                    var aj = await ar.json();
                    var names = (dj.devices || []).map(function (x) { return x.name; }).filter(Boolean);
                    var wa = worstPerDevice(aj.alerts || []);
                    var sr = snmpReachRaw(dj.devices || []);
                    var worst = {};
                    for (var i = 0; i < names.length; i++) {
                        var n = names[i];
                        worst[n] = mergeWorstRaw(wa[n] || '', sr[n] || '');
                    }
                    renderSVG(buildLayout(names, worst));
                } catch (e) {
                    if (console && console.warn) console.warn('hex overview refresh failed', e);
                }
            }
            setInterval(refreshHexOverview, 10000);
            refreshHexOverview();
        })();
        </script>
{{end}}

{{define "snmp-banner-stack"}}
        {{if .SNMPWarnings}}
        <div class="snmp-banner-stack" role="region" aria-label="SNMP notices">
            {{range .SNMPWarnings}}
            <div class="snmp-banner snmp-banner-{{.Class}}">
                <div class="snmp-banner-title">{{.Title}}</div>
                <div class="snmp-banner-body">{{.Body}}</div>
            </div>
            {{end}}
        </div>
        {{end}}
{{end}}

{{define "wizard"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Discovery Wizard - NetSpec</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root { --bg:#0d1117; --bg2:#161b22; --bg3:#21262d; --bd:#30363d; --txt:#e6edf3; --mut:#8b949e; --green:#3fb950; --blue:#58a6ff; --red:#f85149; --yellow:#d29922; }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: 'Outfit', sans-serif; background: var(--bg); color: var(--txt); min-height: 100vh; }
        .container { max-width: 1200px; margin: 0 auto; padding: 2rem; }
        header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; padding-bottom: 1rem; border-bottom: 1px solid var(--bd); }
        .btn { border: 1px solid var(--bd); background: var(--bg3); color: var(--txt); border-radius: 8px; padding: 0.55rem 0.95rem; cursor: pointer; font-family: inherit; text-decoration: none; }
        .btn.primary { background: #238636; border-color: #3fb950; }
        .btn:disabled { opacity: 0.6; cursor: not-allowed; }
        .card { background: var(--bg2); border: 1px solid var(--bd); border-radius: 10px; margin-bottom: 1rem; overflow: hidden; }
        .card h3 { padding: 0.9rem 1rem; border-bottom: 1px solid var(--bd); background: var(--bg3); font-size: 1rem; }
        .card .body { padding: 1rem; }
        .step { display: none; }
        .step.active { display: block; }
        .row { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; margin-bottom: 0.75rem; }
        .field label { display: block; font-size: 0.8rem; color: var(--mut); margin-bottom: 0.3rem; }
        .field input, .field select { width: 100%; background: var(--bg); color: var(--txt); border: 1px solid var(--bd); border-radius: 7px; padding: 0.5rem; font-family: inherit; }
        .actions { display: flex; gap: 0.6rem; justify-content: flex-end; margin-top: 0.8rem; }
        .msg { margin-top: 0.75rem; font-size: 0.85rem; color: var(--mut); }
        .msg.error { color: var(--red); }
        .msg.ok { color: var(--green); }
        .warn { padding: 0.65rem; border: 1px solid #6b4f00; background: rgba(210,153,34,0.15); border-radius: 7px; margin-top: 0.5rem; color: var(--yellow); }
        table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
        th, td { border-bottom: 1px solid var(--bd); padding: 0.45rem; text-align: left; }
        th { color: var(--mut); font-weight: 500; }
        .small { font-size: 0.75rem; color: var(--mut); }
        .badge { display: inline-block; font-size: 0.72rem; padding: 0.15rem 0.4rem; border-radius: 4px; background: rgba(88,166,255,.15); color: var(--blue); }
        .snmp-banner-stack { display: flex; flex-direction: column; gap: 0.75rem; margin-bottom: 1rem; }
        .snmp-banner { border-radius: 10px; padding: 0.85rem 1.1rem; border: 1px solid var(--bd); line-height: 1.45; }
        .snmp-banner-title { font-weight: 600; font-size: 0.92rem; margin-bottom: 0.35rem; }
        .snmp-banner-body { font-size: 0.82rem; color: var(--mut); }
        .snmp-banner.snmp-banner-warning { background: rgba(210,153,34,0.14); border-color: rgba(210,153,34,0.45); }
        .snmp-banner.snmp-banner-warning .snmp-banner-title { color: var(--yellow); }
        .snmp-banner.snmp-banner-info { background: rgba(88,166,255,0.08); border-color: rgba(88,166,255,0.35); }
        .snmp-banner.snmp-banner-info .snmp-banner-title { color: var(--blue); }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1 style="font-size:1.45rem;">Device Discovery Wizard</h1>
                <div class="small">{{if .Version}}{{.Version}}{{else}}dev{{end}}</div>
            </div>
            <div style="display:flex;gap:0.5rem;flex-wrap:wrap;">
                <a href="/" class="btn">← Back to Dashboard</a>
                <a href="/api-browser" class="btn">API Browser</a>
            </div>
        </header>

        {{template "snmp-banner-stack" .}}

        <div id="step1" class="step active card">
            <h3>Step 1: Connect & Probe</h3>
            <div class="body">
                <div class="row">
                    <div class="field"><label>IP / Hostname</label><input id="address" placeholder="10.10.200.120"></div>
                    <div class="field"><label>SNMP Community</label><input id="community" placeholder="Uses SNMP_COMMUNITY from .env"></div>
                </div>
                <div class="row">
                    <div class="field"><label>SNMP Port</label><input id="port" type="number" value="161"></div>
                    <div></div>
                </div>
                <div id="editHint" class="warn" style="display:none;"></div>
                <div class="actions"><button class="btn primary" id="probeBtn">Probe Device</button></div>
                <div id="probeMsg" class="msg"></div>
            </div>
        </div>

        <div id="step2" class="step card">
            <h3>Step 2: Review Device Info</h3>
            <div class="body">
                <div id="probeInfo" class="small"></div>
                <div id="configuredWarn" class="warn" style="display:none;"></div>
                <div class="row" style="margin-top:0.75rem;">
                    <div class="field"><label>Device Key</label><input id="deviceKey"></div>
                    <div class="field"><label>Device Description</label><input id="deviceDescription"></div>
                </div>
                <div class="actions">
                    <button class="btn" id="backTo1">Back</button>
                    <button class="btn primary" id="walkBtn">Walk Interfaces</button>
                </div>
                <div id="walkMsg" class="msg"></div>
            </div>
        </div>

        <div id="step3" class="step card">
            <h3>Step 3: Interface Selection</h3>
            <div class="body">
                <div class="row">
                    <div class="field"><label>Search</label><input id="ifSearch" placeholder="Filter by interface"></div>
                    <div class="field"><label>Show only monitored</label><select id="onlyMonitored"><option value="0">No</option><option value="1">Yes</option></select></div>
                </div>
                <div class="actions" style="justify-content:flex-start; margin-bottom:0.6rem;">
                    <button class="btn" id="selectVisible">Select Visible</button>
                    <button class="btn" id="deselectVisible">Deselect Visible</button>
                </div>
                <div style="max-height:420px; overflow:auto; border:1px solid var(--bd); border-radius:8px;">
                    <table>
                        <thead><tr><th>Monitor</th><th>Interface</th><th>Alias</th><th>Admin</th><th>Oper</th><th>Desired</th><th>Severity</th></tr></thead>
                        <tbody id="ifRows"></tbody>
                    </table>
                </div>
                <div class="actions">
                    <button class="btn" id="backTo2">Back</button>
                    <button class="btn primary" id="toReview">Review & Commit</button>
                </div>
            </div>
        </div>

        <div id="step4" class="step card">
            <h3>Step 4: Confirm & Commit</h3>
            <div class="body">
                <div id="summary" class="small"></div>
                <div class="actions">
                    <button class="btn" id="backTo3">Back</button>
                    <button class="btn primary" id="commitBtn">Write to desired-state.yaml</button>
                </div>
                <div id="commitMsg" class="msg"></div>
            </div>
        </div>

        <div id="step5" class="step card">
            <h3>Step 5: Success</h3>
            <div class="body">
                <div id="successMsg" class="msg ok"></div>
                <div class="actions">
                    <button class="btn primary" id="reloadBtn">Reload Config Now</button>
                    <button class="btn" id="againBtn">Add Another Device</button>
                </div>
                <div id="reloadMsg" class="msg"></div>
            </div>
        </div>
    </div>

    <script>
        var state = { probe: null, walk: null, ifSelections: {} };
        var prefillDeviceKey = '';
        (function(){
            var qs = new URLSearchParams(window.location.search);
            var addr = qs.get('address');
            var devKey = qs.get('device_key');
            if (addr) {
                document.getElementById('address').value = addr;
            }
            if (devKey) {
                prefillDeviceKey = devKey;
            }
            if (addr && devKey) {
                var h = document.getElementById('editHint');
                if (h) {
                    h.style.display = 'block';
                    h.textContent = 'Re-walk mode for device \"' + devKey + '\" at ' + addr + '. Probe, then walk—unchecked interfaces are removed from desired state on commit.';
                }
            }
        })();
        function showStep(id) {
            ['step1','step2','step3','step4','step5'].forEach(function(s){ document.getElementById(s).classList.remove('active'); });
            document.getElementById(id).classList.add('active');
        }
        function msg(id, text, err) { var el=document.getElementById(id); el.textContent=text||''; el.className='msg'+(err?' error':''); }
        function slugify(v) { return (v||'').toLowerCase().trim().replace(/\s+/g,'-').replace(/[^a-z0-9_-]/g,'-').replace(/-+/g,'-'); }
        function esc(v){ var d=document.createElement('div'); d.textContent=v==null?'':String(v); return d.innerHTML; }

        document.getElementById('probeBtn').addEventListener('click', async function(){
            msg('probeMsg','Probing...');
            var body = { address: document.getElementById('address').value, community: document.getElementById('community').value, port: Number(document.getElementById('port').value||161) };
            try{
                var res = await fetch('/api/discovery/probe',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
                var data = await res.json();
                if(!res.ok){ msg('probeMsg', data.error || 'Probe failed', true); return; }
                state.probe = data;
                var info = 'Hostname: '+esc(data.sys_name||'-')+' | Address: '+esc(data.address)+' | Vendor: '+esc(data.vendor_hint)+' | Location: '+esc(data.sys_location||'-');
                document.getElementById('probeInfo').innerHTML = info;
                document.getElementById('deviceKey').value = (data.existing_device_key || slugify(prefillDeviceKey || data.sys_name || data.address));
                document.getElementById('deviceDescription').value = (data.sys_name || data.sys_descr || '').slice(0,120);
                if(data.already_configured){
                    var w = document.getElementById('configuredWarn');
                    w.style.display='block';
                    w.textContent='This device is already configured as '+data.existing_device_key+'. Commit will patch existing interfaces.';
                } else {
                    document.getElementById('configuredWarn').style.display='none';
                }
                showStep('step2');
                msg('probeMsg','');
            }catch(e){ msg('probeMsg', e.message, true); }
        });

        document.getElementById('backTo1').addEventListener('click', function(){ showStep('step1'); });
        document.getElementById('backTo2').addEventListener('click', function(){ showStep('step2'); });
        document.getElementById('backTo3').addEventListener('click', function(){ showStep('step3'); });

        document.getElementById('walkBtn').addEventListener('click', async function(){
            msg('walkMsg','Walking interface table, this may take a few seconds...');
            try{
                var req = { address: state.probe.address, community: document.getElementById('community').value, port: Number(document.getElementById('port').value||161) };
                var res = await fetch('/api/discovery/walk',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(req)});
                var data = await res.json();
                if(!res.ok){ msg('walkMsg', data.error || 'Walk failed', true); return; }
                state.walk = data;
                state.ifSelections = {};
                (state.walk.interfaces || []).forEach(function(it){
                    var ex = it.existing_config;
                    var defMon = ex ? !!ex.monitor : false;
                    var defAlias = (ex && ex.description) ? ex.description : (it.alias || '');
                    var defDesired = (ex && ex.desired_state) ? ex.desired_state : (it.oper_status === 'up' ? 'up' : 'down');
                    var defSev = (ex && ex.alert_severity) ? ex.alert_severity : 'warning';
                    var defPC = ex ? !!ex.is_port_channel : !!it.is_port_channel;
                    var defMem = (ex && Array.isArray(ex.members) && ex.members.length)
                        ? ex.members.slice()
                        : (Array.isArray(it.channel_members) ? it.channel_members.slice() : []);
                    state.ifSelections[it.name] = {
                        monitor: defMon,
                        alias: defAlias,
                        desired_state: defDesired,
                        alert_severity: defSev,
                        is_port_channel: defPC,
                        members: defMem
                    };
                });
                renderIfRows();
                showStep('step3');
            }catch(e){ msg('walkMsg', e.message, true); }
        });

        function renderIfRows(){
            var q = (document.getElementById('ifSearch').value||'').toLowerCase();
            var only = document.getElementById('onlyMonitored').value === '1';
            var rows = '';
            (state.walk.interfaces||[]).forEach(function(it, idx){
                var sel = state.ifSelections[it.name] || {
                    monitor: false,
                    alias: it.alias || '',
                    desired_state: (it.oper_status === 'up' ? 'up' : 'down'),
                    alert_severity: 'warning',
                    is_port_channel: !!it.is_port_channel,
                    members: Array.isArray(it.channel_members) ? it.channel_members : []
                };
                var isMonitored = !!sel.monitor;
                if(q && (it.name||'').toLowerCase().indexOf(q)===-1 && (it.alias||'').toLowerCase().indexOf(q)===-1) return;
                if(only && !isMonitored) return;
                rows += '<tr data-idx="'+idx+'">'+
                    '<td><input type="checkbox" class="if-mon" '+(isMonitored?'checked':'')+'></td>'+
                    '<td>'+esc(it.name)+' '+(it.already_configured?'<span class="badge">configured</span>':'')+'</td>'+
                    '<td><input class="if-alias" value="'+esc(sel.alias||'')+'" style="width:100%;background:#0d1117;color:#e6edf3;border:1px solid #30363d;border-radius:6px;padding:0.3rem;"></td>'+
                    '<td>'+esc(it.admin_status)+'</td>'+
                    '<td>'+esc(it.oper_status)+'</td>'+
                    '<td><select class="if-desired"><option value="up" '+(sel.desired_state==='up'?'selected':'')+'>up</option><option value="down" '+(sel.desired_state==='down'?'selected':'')+'>down</option></select></td>'+
                    '<td><select class="if-sev"><option value="info" '+(sel.alert_severity==='info'?'selected':'')+'>info</option><option value="warning" '+(sel.alert_severity==='warning'?'selected':'')+'>warning</option><option value="critical" '+(sel.alert_severity==='critical'?'selected':'')+'>critical</option></select></td>'+
                    '</tr>';
            });
            document.getElementById('ifRows').innerHTML = rows || '<tr><td colspan="7" class="small">No interfaces match the current filter.</td></tr>';
            bindRowInputs();
        }
        document.getElementById('ifSearch').addEventListener('input', renderIfRows);
        document.getElementById('onlyMonitored').addEventListener('change', renderIfRows);

        function bindRowInputs() {
            document.querySelectorAll('#ifRows tr[data-idx]').forEach(function(r){
                var idx = Number(r.getAttribute('data-idx'));
                var src = state.walk.interfaces[idx];
                var key = src.name;
                var mon = r.querySelector('.if-mon');
                var alias = r.querySelector('.if-alias');
                var desired = r.querySelector('.if-desired');
                var sev = r.querySelector('.if-sev');
                mon.addEventListener('change', function(){ state.ifSelections[key].monitor = mon.checked; });
                alias.addEventListener('input', function(){ state.ifSelections[key].alias = alias.value || ''; });
                desired.addEventListener('change', function(){ state.ifSelections[key].desired_state = desired.value; });
                sev.addEventListener('change', function(){ state.ifSelections[key].alert_severity = sev.value; });
            });
        }

        function setVisibleMonitor(value){
            document.querySelectorAll('#ifRows tr[data-idx]').forEach(function(r){
                var idx = Number(r.getAttribute('data-idx'));
                var src = state.walk.interfaces[idx];
                state.ifSelections[src.name].monitor = value;
            });
            renderIfRows();
        }
        document.getElementById('selectVisible').addEventListener('click', function(){ setVisibleMonitor(true); });
        document.getElementById('deselectVisible').addEventListener('click', function(){ setVisibleMonitor(false); });

        document.getElementById('toReview').addEventListener('click', function(){
            var payload = [];
            var discoveredCount = (state.walk.interfaces || []).length;
            var action = state.probe.already_configured ? 'patch' : 'add';
            var sync = action === 'patch';
            var monitoredCount = 0;
            (state.walk.interfaces || []).forEach(function(src){
                var sel = state.ifSelections[src.name] || {};
                var ifName = (src.name || '').trim();
                if (!ifName) {
                    return;
                }
                if (!sync && !sel.monitor) {
                    return;
                }
                var mon = !!sel.monitor;
                if (mon) { monitoredCount++; }
                var row = {
                    name: ifName,
                    alias: sel.alias || '',
                    monitor: mon,
                    desired_state: mon ? (sel.desired_state || (src.oper_status === 'up' ? 'up' : 'down')) : 'up',
                    admin_state: 'enabled',
                    alert_severity: mon ? (sel.alert_severity || 'warning') : 'warning',
                    is_port_channel: !!sel.is_port_channel,
                    members: Array.isArray(sel.members) ? sel.members : []
                };
                payload.push(row);
            });
            if (!sync && monitoredCount === 0) {
                msg('walkMsg', 'Select at least one interface to monitor before committing.', true);
                return;
            }
            msg('walkMsg', '');
            state.commitInterfaces = payload;
            var sumExtra = sync
                ? '<br><span class="small">Patch uses full walk sync: interfaces you leave unchecked are <b>removed</b> from desired state (interfaces not seen on this walk are unchanged).</span>'
                : '';
            document.getElementById('summary').innerHTML =
                'Device key: <b>'+esc(document.getElementById('deviceKey').value)+'</b><br>'+
                'Action: <b>'+action+'</b>' + (sync ? ' (sync discovered)' : '') + '<br>'+
                'Address: <b>'+esc(state.probe.address)+'</b><br>'+
                'Interfaces discovered: <b>'+discoveredCount+'</b>, monitored after commit: <b>'+monitoredCount+'</b>' + sumExtra;
            showStep('step4');
        });

        document.getElementById('commitBtn').addEventListener('click', async function(){
            msg('commitMsg','Writing configuration...');
            var action = state.probe.already_configured ? 'patch' : 'add';
            var req = {
                address: state.probe.address,
                community: document.getElementById('community').value || '',
                device_key: document.getElementById('deviceKey').value,
                device_description: document.getElementById('deviceDescription').value,
                existing_device_key: state.probe.existing_device_key || '',
                action: action,
                sync_discovered_interfaces: action === 'patch',
                interfaces: state.commitInterfaces || []
            };
            try{
                var res = await fetch('/api/discovery/commit',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(req)});
                var data = await res.json();
                if(!res.ok){ msg('commitMsg', data.error || 'Commit failed', true); return; }
                document.getElementById('successMsg').textContent = data.message || 'Config written successfully.';
                showStep('step5');
            }catch(e){ msg('commitMsg', e.message, true); }
        });

        document.getElementById('reloadBtn').addEventListener('click', async function(){
            msg('reloadMsg','Reloading...');
            try{
                var res = await fetch('/api/reload',{method:'POST'});
                var data = await res.json();
                if(!res.ok){ msg('reloadMsg', data.error || 'Reload failed', true); return; }
                msg('reloadMsg','Config reloaded. NetSpec is now monitoring your updated device list.');
            }catch(e){ msg('reloadMsg', e.message, true); }
        });

        document.getElementById('againBtn').addEventListener('click', function(){
            state = { probe: null, walk: null, ifSelections: {} };
            document.getElementById('address').value = '';
            document.getElementById('community').value = '';
            document.getElementById('ifRows').innerHTML = '';
            showStep('step1');
        });
    </script>
</body>
</html>
{{end}}

{{define "device"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Device.Name}} - NetSpec</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-primary: #0d1117;
            --bg-secondary: #161b22;
            --bg-tertiary: #21262d;
            --border-color: #30363d;
            --text-primary: #e6edf3;
            --text-secondary: #8b949e;
            --text-muted: #6e7681;
            --accent-green: #3fb950;
            --accent-green-dim: #238636;
            --accent-red: #f85149;
            --accent-yellow: #d29922;
            --accent-blue: #58a6ff;
            --accent-purple: #a371f7;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'Outfit', -apple-system, BlinkMacSystemFont, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.6;
            min-height: 100vh;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 2rem;
        }

        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 2rem;
            padding-bottom: 1.5rem;
            border-bottom: 1px solid var(--border-color);
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }

        .logo-icon {
            width: 40px;
            height: 40px;
            background: linear-gradient(135deg, var(--accent-green) 0%, var(--accent-blue) 100%);
            border-radius: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 700;
            font-size: 1.2rem;
        }

        h1 {
            font-size: 1.75rem;
            font-weight: 600;
        }

        .btn {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.625rem 1.25rem;
            border: none;
            border-radius: 8px;
            font-family: inherit;
            font-size: 0.875rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
            text-decoration: none;
        }

        .btn-secondary {
            background: var(--bg-tertiary);
            color: var(--text-primary);
            border: 1px solid var(--border-color);
        }

        .btn-secondary:hover {
            background: var(--border-color);
        }

        .card {
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            overflow: hidden;
            margin-bottom: 1.5rem;
        }

        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 1rem 1.25rem;
            background: var(--bg-tertiary);
            border-bottom: 1px solid var(--border-color);
        }

        .card-title {
            font-size: 1rem;
            font-weight: 600;
        }

        .card-body {
            padding: 1rem 1.25rem;
        }

        .status-badge {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.375rem 0.75rem;
            border-radius: 6px;
            font-size: 0.8125rem;
            font-weight: 500;
        }

        .status-badge.connected {
            background: rgba(63, 185, 80, 0.15);
            color: var(--accent-green);
        }

        .status-badge.disconnected {
            background: rgba(248, 81, 73, 0.15);
            color: var(--accent-red);
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
        }

        .status-dot.connected {
            background: var(--accent-green);
            animation: pulse 2s infinite;
        }

        .status-dot.disconnected {
            background: var(--accent-red);
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        .info-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 1rem;
        }

        .info-item {
            display: flex;
            flex-direction: column;
            gap: 0.25rem;
        }

        .info-label {
            font-size: 0.8125rem;
            color: var(--text-secondary);
        }

        .info-value {
            font-size: 0.9375rem;
            font-family: 'JetBrains Mono', monospace;
            color: var(--text-primary);
        }

        .interface-list {
            list-style: none;
        }

        .interface-header {
            display: grid;
            grid-template-columns: 2.1fr 0.8fr 1fr 1fr 2.2fr;
            gap: 1rem;
            padding: 0.75rem 1.25rem;
            font-size: 0.75rem;
            color: var(--text-secondary);
            background: var(--bg-tertiary);
            border-bottom: 1px solid var(--border-color);
        }

        .interface-item {
            display: grid;
            grid-template-columns: 2.1fr 0.8fr 1fr 1fr 2.2fr;
            gap: 1rem;
            align-items: center;
            padding: 1rem 1.25rem;
            border-bottom: 1px solid var(--border-color);
        }
        .iface-edit {
            display: grid;
            grid-template-columns: repeat(2, minmax(0, 1fr));
            gap: 0.35rem;
            font-size: 0.72rem;
        }
        .iface-edit label {
            color: var(--text-secondary);
            display: flex;
            align-items: center;
            gap: 0.3rem;
            margin-right: 0.5rem;
        }
        .iface-edit select {
            width: 100%;
            background: var(--bg-primary);
            color: var(--text-primary);
            border: 1px solid var(--border-color);
            border-radius: 6px;
            padding: 0.22rem 0.35rem;
            font-size: 0.72rem;
        }
        .iface-edit .save-btn {
            grid-column: 1 / span 2;
            justify-self: start;
            padding: 0.28rem 0.6rem;
            font-size: 0.72rem;
        }

        .interface-item:last-child {
            border-bottom: none;
        }

        .interface-info h4 {
            font-size: 0.9375rem;
            font-weight: 500;
            margin-bottom: 0.25rem;
            font-family: 'JetBrains Mono', monospace;
        }

        .interface-meta {
            display: flex;
            gap: 1rem;
            font-size: 0.8125rem;
            color: var(--text-secondary);
        }

        .interface-state {
            padding: 0.375rem 0.75rem;
            border-radius: 6px;
            font-size: 0.8125rem;
            font-weight: 500;
        }

        .interface-state.up {
            background: rgba(63, 185, 80, 0.15);
            color: var(--accent-green);
        }

        .interface-state.down {
            background: rgba(248, 81, 73, 0.15);
            color: var(--accent-red);
        }

        .interface-state.unknown {
            background: rgba(210, 153, 34, 0.15);
            color: var(--accent-yellow);
        }

        .iface-ts {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.75rem;
            color: var(--text-secondary);
        }

        .log-container {
            height: 400px;
            overflow-y: auto;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.8125rem;
            background: var(--bg-primary);
        }

        .log-entry {
            padding: 0.5rem 1rem;
            border-bottom: 1px solid var(--bg-tertiary);
            display: flex;
            gap: 1rem;
        }

        .log-time {
            color: var(--text-muted);
            white-space: nowrap;
        }

        .log-level {
            text-transform: uppercase;
            font-weight: 600;
            min-width: 50px;
        }

        .log-info .log-level { color: var(--accent-blue); }
        .log-warn .log-level { color: var(--accent-yellow); }
        .log-error .log-level { color: var(--accent-red); }
        .log-debug .log-level { color: var(--text-muted); }

        .log-message {
            color: var(--text-secondary);
            word-break: break-word;
        }

        .snmp-banner-stack {
            display: flex;
            flex-direction: column;
            gap: 0.75rem;
            margin-bottom: 1.25rem;
        }
        .snmp-banner {
            border-radius: 10px;
            padding: 0.85rem 1.1rem;
            border: 1px solid var(--border-color);
            line-height: 1.45;
        }
        .snmp-banner-title {
            font-weight: 600;
            font-size: 0.95rem;
            margin-bottom: 0.35rem;
        }
        .snmp-banner-body {
            font-size: 0.85rem;
            color: var(--text-secondary);
        }
        .snmp-banner.snmp-banner-warning {
            background: rgba(210, 153, 34, 0.12);
            border-color: rgba(210, 153, 34, 0.45);
        }
        .snmp-banner.snmp-banner-warning .snmp-banner-title {
            color: var(--accent-yellow);
        }
        .snmp-banner.snmp-banner-info {
            background: rgba(88, 166, 255, 0.08);
            border-color: rgba(88, 166, 255, 0.35);
        }
        .snmp-banner.snmp-banner-info .snmp-banner-title {
            color: var(--accent-blue);
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo">
                <div class="logo-icon">N</div>
                <div>
                    <h1>{{.Device.Name}}</h1>
                    <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem;">
                        {{.Device.Address}}
                    </div>
                </div>
            </div>
            <div>
                <button class="btn btn-danger" onclick="deleteDevice()">Delete Device</button>
                <a href="/wizard?device_key={{queryEscape .Device.Name}}&amp;address={{queryEscape .Device.Address}}" class="btn btn-secondary">Re-walk interfaces</a>
                <a href="/api-browser" class="btn btn-secondary">API</a>
                <a href="/" class="btn btn-secondary">← Back to Dashboard</a>
            </div>
        </header>

        {{template "snmp-banner-stack" .}}

        <div class="card">
            <div class="card-header">
                <span class="card-title">📡 Connection Status</span>
                <span class="status-badge connected">
                    <span class="status-dot connected"></span>
                    Active
                </span>
            </div>
            <div class="card-body">
                <div class="info-grid">
                    <div class="info-item">
                        <span class="info-label">Description</span>
                        <span class="info-value">{{.Device.Description}}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Connected Since</span>
                        <span class="info-value">
                            {{if .Device.LastTelemetryValidationAt.IsZero}}No telemetry yet{{else}}<span data-local-ts="{{.Device.LastTelemetryValidationAt.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="datetime"></span>{{end}}
                        </span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">SNMP-Validated Interfaces</span>
                        <span class="info-value">{{len .Device.Interfaces}}</span>
                    </div>
                </div>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <span class="card-title">📊 Subscription Status</span>
                <span class="status-badge connected">
                    Runtime State
                </span>
            </div>
            <div class="card-body">
                <div class="info-grid">
                    <div class="info-item">
                        <span class="info-label">Interfaces</span>
                        <span class="info-value" style="color: var(--accent-green);">{{len .Device.Interfaces}}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Latest Telemetry Validation</span>
                        <span class="info-value">
                            {{if .Device.LastTelemetryValidationAt.IsZero}}Never{{else}}<span data-local-ts="{{.Device.LastTelemetryValidationAt.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="datetime"></span>{{end}}
                        </span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Latest SNMP Validation</span>
                        <span class="info-value">{{if .Device.LastSNMPValidationAt.IsZero}}Never{{else}}<span data-local-ts="{{.Device.LastSNMPValidationAt.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="datetime"></span>{{end}}</span>
                    </div>
                </div>
                <div style="margin-top: 1rem; padding: 0.75rem; background: rgba(210, 153, 34, 0.1); border-left: 3px solid var(--accent-yellow); border-radius: 4px; color: var(--text-secondary);">
                    Interface runtime details are sourced from SNMP validation and push telemetry events.
                </div>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <span class="card-title">🔌 Monitored Interfaces</span>
                <span style="font-size: 0.8125rem; color: var(--text-secondary);">{{len .Device.Interfaces}} interfaces</span>
            </div>
            <div class="card-body" style="padding: 0;">
                {{if .Device.Interfaces}}
                <div class="interface-header">
                    <span>Interface</span>
                    <span>Live State</span>
                    <span>Last SNMP Validation</span>
                    <span>Last Telemetry Validation</span>
                    <span>Interface Policy</span>
                </div>
                <ul class="interface-list">
                    {{range .Device.Interfaces}}
                    <li class="interface-item" data-iface-name="{{.Name}}">
                        <div class="interface-info">
                            <h4>{{.Name}}</h4>
                            <div class="interface-meta">
                                {{if .Description}}<span>{{.Description}}</span>{{end}}
                                <span>Desired: {{.DesiredState}}</span>
                                <span>Admin: {{.AdminState}}</span>
                            </div>
                        </div>
                        <span class="interface-state {{if .OperStatus}}{{.OperStatus}}{{else}}unknown{{end}}">
                            {{if .OperStatus}}{{.OperStatus}}{{else}}unknown{{end}}
                        </span>
                        <span class="iface-ts">
                            {{if .LastSNMPValidationAt.IsZero}}-{{else}}<span data-local-ts="{{.LastSNMPValidationAt.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="datetime"></span>{{end}}
                        </span>
                        <span class="iface-ts">
                            {{if .LastTelemetryValidationAt.IsZero}}-{{else}}<span data-local-ts="{{.LastTelemetryValidationAt.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="datetime"></span>{{end}}
                        </span>
                        <span class="iface-edit">
                            <label><input type="checkbox" class="if-mon" {{if .Monitor}}checked{{end}}>Monitored</label>
                            <select class="if-desired">
                                <option value="up" {{if eq .DesiredState "up"}}selected{{end}}>Desired: up</option>
                                <option value="down" {{if eq .DesiredState "down"}}selected{{end}}>Desired: down</option>
                            </select>
                            <select class="if-admin">
                                <option value="enabled" {{if eq .AdminState "enabled"}}selected{{end}}>Admin: enabled</option>
                                <option value="disabled" {{if eq .AdminState "disabled"}}selected{{end}}>Admin: disabled</option>
                            </select>
                            <select class="if-severity">
                                <option value="info" {{if eq .Alerts.StateMismatch "info"}}selected{{end}}>Alert: info</option>
                                <option value="warning" {{if or (eq .Alerts.StateMismatch "") (eq .Alerts.StateMismatch "warning")}}selected{{end}}>Alert: warning</option>
                                <option value="critical" {{if eq .Alerts.StateMismatch "critical"}}selected{{end}}>Alert: critical</option>
                            </select>
                            <button
                                class="btn btn-secondary save-btn"
                                onclick='saveInterfacePolicy(this)'
                            >Save</button>
                        </span>
                    </li>
                    {{end}}
                </ul>
                {{else}}
                <div style="padding: 2rem; text-align: center; color: var(--text-muted);">
                    No interfaces configured
                </div>
                {{end}}
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <span class="card-title">📋 Device Logs</span>
                <button class="btn btn-secondary" onclick="document.querySelector('.log-container').scrollTop = document.querySelector('.log-container').scrollHeight">↓ Latest</button>
            </div>
            <div class="card-body" style="padding: 0;">
                <div class="log-container">
                    {{range .Device.Logs}}
                    <div class="log-entry log-{{.Level}}">
                        <span class="log-time" data-local-ts="{{.Timestamp.Format "2006-01-02T15:04:05Z07:00"}}" data-local-ts-mode="time"></span>
                        <span class="log-level">{{.Level}}</span>
                        <span class="log-message">{{.Message}}</span>
                    </div>
                    {{else}}
                    <div style="padding: 2rem; text-align: center; color: var(--text-muted);">
                        No logs available
                    </div>
                    {{end}}
                </div>
            </div>
        </div>
    </div>
    <script>
        function formatLocalTimestamp(value, withDate) {
            const d = new Date(value);
            if (Number.isNaN(d.getTime())) return value;
            if (withDate) return d.toLocaleString();
            return d.toLocaleTimeString();
        }

        function localizeTimestamps(root) {
            (root || document).querySelectorAll('[data-local-ts]').forEach(el => {
                const raw = el.getAttribute('data-local-ts');
                const mode = el.getAttribute('data-local-ts-mode') || 'datetime';
                el.textContent = formatLocalTimestamp(raw, mode !== 'time');
            });
        }

        localizeTimestamps(document);

        // Auto-refresh device data every 5 seconds
        setInterval(() => {
            fetch('/api/devices/{{.Device.Name}}')
                .then(r => r.json())
                .then(data => {
                    // Update logs
                    const container = document.querySelector('.log-container');
                    if (container && data.logs) {
                        const wasAtBottom = container.scrollHeight - container.scrollTop <= container.clientHeight + 50;
                        container.innerHTML = data.logs.map(e => 
                            '<div class="log-entry log-' + e.level + '">' +
                            '<span class="log-time">' + formatLocalTimestamp(e.timestamp, false) + '</span>' +
                            '<span class="log-level">' + e.level + '</span>' +
                            '<span class="log-message">' + escapeHtml(e.message) + '</span>' +
                            '</div>'
                        ).join('');
                        if (wasAtBottom) container.scrollTop = container.scrollHeight;
                    }

                    // Update health stats from API response
                    if (data.health) {
                        const h = data.health;
                        // Reload page if connection state changed significantly
                        // (simpler than updating all DOM elements individually)
                        const currentConnected = document.querySelector('.status-dot.connected') !== null;
                        if (h.connected !== currentConnected) {
                            location.reload();
                        }
                    }
                });
        }, 5000);

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        async function saveInterfacePolicy(buttonEl) {
            const row = buttonEl.closest('[data-iface-name]');
            if (!row) return;
            const name = row.getAttribute('data-iface-name');
            const nextDesired = row.querySelector('.if-desired').value;
            const nextAdmin = row.querySelector('.if-admin').value;
            const nextSeverity = row.querySelector('.if-severity').value;
            const nextMonitor = row.querySelector('.if-mon').checked;
            const res = await fetch('/api/devices/{{.Device.Name}}/interfaces/' + encodeURIComponent(name), {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    desired_state: (nextDesired || '').trim(),
                    admin_state: (nextAdmin || '').trim(),
                    alert_severity: (nextSeverity || '').trim(),
                    monitor: nextMonitor
                })
            });
            if (!res.ok) {
                const txt = await res.text();
                alert('Update failed: ' + txt);
                return;
            }
            location.reload();
        }

        async function deleteDevice() {
            const deviceName = '{{.Device.Name}}';
            const ok = confirm('Delete device "' + deviceName + '" from monitoring? This removes it from config.');
            if (!ok) return;
            const res = await fetch('/api/devices/' + encodeURIComponent(deviceName), {
                method: 'DELETE'
            });
            if (!res.ok) {
                const txt = await res.text();
                alert('Delete failed: ' + txt);
                return;
            }
            window.location.href = '/';
        }
    </script>
</body>
</html>
{{end}}
`))
