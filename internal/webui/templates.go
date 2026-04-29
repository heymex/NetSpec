package webui

import (
	"html/template"
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

        .grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1.5rem;
            margin-bottom: 1.5rem;
        }

        @media (max-width: 1024px) {
            .grid { grid-template-columns: 1fr; }
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
            grid-template-columns: repeat(3, minmax(0, 1fr));
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
            padding: 1rem 1.5rem;
            background: var(--bg-secondary);
            border: 1px solid var(--accent-green);
            border-radius: 8px;
            display: none;
            animation: slideIn 0.3s ease;
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
    </style>
</head>
<body>
    <div class="container">
        {{template "content" .}}
    </div>
    <div id="toast" class="toast"></div>
    <script>
        function showToast(message, isError) {
            const toast = document.getElementById('toast');
            toast.textContent = message;
            toast.className = 'toast show' + (isError ? ' error' : '');
            setTimeout(() => toast.className = 'toast', 3000);
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
                            '<span class="log-time">' + new Date(e.timestamp).toLocaleTimeString() + '</span>' +
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
                <a class="btn btn-secondary" href="/wizard">+ Add Device</a>
                <button class="btn btn-primary" onclick="reloadConfig()">↻ Reload Config</button>
            </div>
        </header>

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
                        </div>
                        {{if .Telemetry.LastEventAt.IsZero}}
                        <div style="font-size:0.75rem;color:var(--accent-yellow);">No accepted telemetry events yet.</div>
                        {{else}}
                        <div style="font-size:0.75rem;color:var(--text-secondary);">Last accepted: {{.Telemetry.LastEventAt.Format "2006-01-02 15:04:05"}}</div>
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
                            <span class="config-key">gNMI Port</span>
                            <span class="config-value">{{.Config.GNMIPort}}</span>
                        </div>
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
                            <span class="log-time">{{.Timestamp.Format "15:04:05"}}</span>
                            <span class="log-level">{{.Level}}</span>
                            <span class="log-message">{{.Message}}</span>
                        </div>
                        {{end}}
                    </div>
                </div>
            </div>
        </div>
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
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1 style="font-size:1.45rem;">Device Discovery Wizard</h1>
                <div class="small">{{if .Version}}{{.Version}}{{else}}dev{{end}}</div>
            </div>
            <a href="/" class="btn">← Back to Dashboard</a>
        </header>

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
                document.getElementById('deviceKey').value = slugify(prefillDeviceKey || data.sys_name || data.address);
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
                    state.ifSelections[it.name] = {
                        monitor: false, // default to monitoring nothing until explicitly selected
                        alias: it.alias || '',
                        desired_state: (it.oper_status === 'up' ? 'up' : 'down'),
                        alert_severity: 'warning'
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
                    alert_severity: 'warning'
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
            (state.walk.interfaces || []).forEach(function(src){
                var sel = state.ifSelections[src.name] || {};
                var ifName = (src.name || '').trim();
                if (!ifName || !sel.monitor) {
                    return;
                }
                payload.push({
                    name: ifName,
                    alias: sel.alias || '',
                    monitor: true,
                    desired_state: sel.desired_state || (src.oper_status === 'up' ? 'up' : 'down'),
                    admin_state: 'enabled',
                    alert_severity: sel.alert_severity || 'warning'
                });
            });
            if (payload.length === 0) {
                msg('walkMsg', 'Select at least one interface to monitor before committing.', true);
                return;
            }
            msg('walkMsg', '');
            state.commitInterfaces = payload;
            var action = state.probe.already_configured ? 'patch' : 'add';
            document.getElementById('summary').innerHTML =
                'Device key: <b>'+esc(document.getElementById('deviceKey').value)+'</b><br>'+
                'Action: <b>'+action+'</b><br>'+
                'Address: <b>'+esc(state.probe.address)+'</b><br>'+
                'Interfaces discovered: <b>'+discoveredCount+'</b>, selected for monitoring: <b>'+payload.length+'</b>';
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
            grid-template-columns: 2.2fr 0.8fr 1fr 1fr;
            gap: 1rem;
            padding: 0.75rem 1.25rem;
            font-size: 0.75rem;
            color: var(--text-secondary);
            background: var(--bg-tertiary);
            border-bottom: 1px solid var(--border-color);
        }

        .interface-item {
            display: grid;
            grid-template-columns: 2.2fr 0.8fr 1fr 1fr;
            gap: 1rem;
            align-items: center;
            padding: 1rem 1.25rem;
            border-bottom: 1px solid var(--border-color);
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
                <a href="/" class="btn btn-secondary">← Back to Dashboard</a>
            </div>
        </header>

        <div class="card">
            <div class="card-header">
                <span class="card-title">📡 Connection Status</span>
                <div style="display: flex; gap: 0.75rem; align-items: center;">
                    <span class="status-badge {{if .Device.Connected}}connected{{else}}disconnected{{end}}">
                        <span class="status-dot {{if .Device.Connected}}connected{{else}}disconnected{{end}}"></span>
                        {{if .Device.Connected}}Connected{{else}}Disconnected{{end}}
                    </span>
                    <button class="btn btn-secondary" onclick="testConnection()" id="test-btn">🔍 Test Connection</button>
                </div>
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
                            {{if .Device.ConnectedSince.IsZero}}Never{{else}}{{.Device.ConnectedSince.Format "2006-01-02 15:04:05"}}{{end}}
                        </span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Reconnect Count</span>
                        <span class="info-value">{{.Device.ReconnectCount}}</span>
                    </div>
                </div>
                {{if .Device.LastError}}
                <div style="margin-top: 1rem; padding: 0.75rem; background: rgba(248, 81, 73, 0.1); border-left: 3px solid var(--accent-red); border-radius: 4px;">
                    <strong style="color: var(--accent-red);">Last Error:</strong>
                    <span style="color: var(--text-secondary); margin-left: 0.5rem;">{{.Device.LastError}}</span>
                </div>
                {{end}}
                <div id="test-result" style="display: none; margin-top: 1rem; padding: 0.75rem; border-left: 3px solid var(--accent-blue); border-radius: 4px;"></div>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <span class="card-title">📊 Subscription Status</span>
                <span class="status-badge {{if .Device.SyncReceived}}connected{{else}}disconnected{{end}}">
                    {{if .Device.SyncReceived}}Synced{{else}}Waiting{{end}}
                </span>
            </div>
            <div class="card-body">
                <div class="info-grid">
                    <div class="info-item">
                        <span class="info-label">Updates Received</span>
                        <span class="info-value" style="{{if gt .Device.UpdateCount 0}}color: var(--accent-green);{{else}}color: var(--accent-yellow);{{end}}">{{.Device.UpdateCount}}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Last Update</span>
                        <span class="info-value">
                            {{if .Device.LastUpdate.IsZero}}Never{{else}}{{.Device.LastUpdate.Format "2006-01-02 15:04:05"}}{{end}}
                        </span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Sync Received</span>
                        <span class="info-value">{{if .Device.SyncReceived}}Yes{{else}}No{{end}}</span>
                    </div>
                </div>
                {{if .Device.LastPath}}
                <div style="margin-top: 1rem; padding: 0.75rem; background: var(--bg-primary); border-radius: 4px; font-family: 'JetBrains Mono', monospace; font-size: 0.8125rem;">
                    <div style="color: var(--text-secondary); margin-bottom: 0.25rem;">Last received path:</div>
                    <div style="color: var(--accent-blue);">{{.Device.LastPath}}</div>
                    <div style="color: var(--accent-green); margin-top: 0.25rem;">= {{.Device.LastValue}}</div>
                </div>
                {{else}}
                <div style="margin-top: 1rem; padding: 0.75rem; background: rgba(210, 153, 34, 0.1); border-left: 3px solid var(--accent-yellow); border-radius: 4px; color: var(--text-secondary);">
                    No gNMI updates received yet. If the connection is established, interface state changes will appear here.
                </div>
                {{end}}
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
                </div>
                <ul class="interface-list">
                    {{range .Device.Interfaces}}
                    <li class="interface-item">
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
                            {{if .LastSNMPValidationAt.IsZero}}-{{else}}{{.LastSNMPValidationAt.Format "2006-01-02 15:04:05"}}{{end}}
                        </span>
                        <span class="iface-ts">
                            {{if .LastTelemetryValidationAt.IsZero}}-{{else}}{{.LastTelemetryValidationAt.Format "2006-01-02 15:04:05"}}{{end}}
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
                        <span class="log-time">{{.Timestamp.Format "15:04:05"}}</span>
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
                            '<span class="log-time">' + new Date(e.timestamp).toLocaleTimeString() + '</span>' +
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

        // Test connection button handler
        async function testConnection() {
            const btn = document.getElementById('test-btn');
            const result = document.getElementById('test-result');
            btn.disabled = true;
            btn.textContent = '⏳ Testing...';
            result.style.display = 'block';
            result.style.background = 'rgba(88, 166, 255, 0.1)';
            result.innerHTML = '<span style="color: var(--accent-blue);">Sending gNMI Capabilities request...</span>';

            try {
                const res = await fetch('/api/test/{{.Device.Name}}', { method: 'POST' });
                const data = await res.json();
                if (data.success) {
                    result.style.background = 'rgba(63, 185, 80, 0.1)';
                    result.style.borderColor = 'var(--accent-green)';
                    result.innerHTML = '<strong style="color: var(--accent-green);">✓ Connection test passed</strong>' +
                        '<div style="margin-top: 0.5rem; font-family: JetBrains Mono, monospace; font-size: 0.8125rem; color: var(--text-secondary);">' +
                        'gNMI Version: ' + escapeHtml(data.gnmi_version) + '<br>' +
                        'Supported Models: ' + data.model_count +
                        '</div>';
                } else {
                    result.style.background = 'rgba(248, 81, 73, 0.1)';
                    result.style.borderColor = 'var(--accent-red)';
                    result.innerHTML = '<strong style="color: var(--accent-red);">✗ Connection test failed</strong>' +
                        '<div style="margin-top: 0.5rem; font-size: 0.8125rem; color: var(--text-secondary);">' +
                        escapeHtml(data.error) + '</div>';
                }
            } catch (e) {
                result.style.background = 'rgba(248, 81, 73, 0.1)';
                result.style.borderColor = 'var(--accent-red)';
                result.innerHTML = '<strong style="color: var(--accent-red);">✗ Request failed</strong>' +
                    '<div style="margin-top: 0.5rem; font-size: 0.8125rem; color: var(--text-secondary);">' +
                    escapeHtml(e.message) + '</div>';
            }
            btn.disabled = false;
            btn.textContent = '🔍 Test Connection';
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
    </script>
</body>
</html>
{{end}}
`))
