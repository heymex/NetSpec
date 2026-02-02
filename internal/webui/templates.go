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

        .interface-item {
            display: flex;
            justify-content: space-between;
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
                <span class="status-badge {{if .Device.Connected}}connected{{else}}disconnected{{end}}">
                    <span class="status-dot {{if .Device.Connected}}connected{{else}}disconnected{{end}}"></span>
                    {{if .Device.Connected}}Connected{{else}}Disconnected{{end}}
                </span>
            </div>
            <div class="card-body">
                <div class="info-grid">
                    <div class="info-item">
                        <span class="info-label">Last Update</span>
                        <span class="info-value">
                            {{if .Device.LastUpdate.IsZero}}Never{{else}}{{.Device.LastUpdate.Format "2006-01-02 15:04:05"}}{{end}}
                        </span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Reconnect Count</span>
                        <span class="info-value">{{.Device.ReconnectCount}}</span>
                    </div>
                    <div class="info-item">
                        <span class="info-label">Description</span>
                        <span class="info-value">{{.Device.Description}}</span>
                    </div>
                </div>
                {{if .Device.LastError}}
                <div style="margin-top: 1rem; padding: 0.75rem; background: rgba(248, 81, 73, 0.1); border-left: 3px solid var(--accent-red); border-radius: 4px;">
                    <strong style="color: var(--accent-red);">Last Error:</strong>
                    <span style="color: var(--text-secondary); margin-left: 0.5rem;">{{.Device.LastError}}</span>
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
                        <span class="interface-state {{.DesiredState}}">{{.DesiredState}}</span>
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
        // Auto-refresh logs every 5 seconds
        setInterval(() => {
            fetch('/api/devices/{{.Device.Name}}')
                .then(r => r.json())
                .then(data => {
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
`))
