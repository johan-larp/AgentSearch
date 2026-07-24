package report

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/johan-larp/agentsearch/internal/models"
)

// HTMLReport генерирует интерактивный HTML-отчёт.
type HTMLReport struct{}

func NewHTMLReport() *HTMLReport {
	return &HTMLReport{}
}

func (h *HTMLReport) Generate(target string, results []models.Result, duration time.Duration) (string, error) {
	summary := BuildSummary(target, results, duration)
	path := filepath.Join("output", fmt.Sprintf("%s_report.html", sanitizeFilename(target)))

	if err := os.MkdirAll("output", 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data := struct {
		Target    string
		Duration  string
		Summary   Summary
		Results   []models.Result
		Timestamp string
	}{
		Target:    target,
		Duration:  duration.Round(time.Second).String(),
		Summary:   summary,
		Results:   results,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
	}

	tmpl := template.Must(template.New("report").Parse(htmlTemplate))
	if err := tmpl.Execute(f, data); err != nil {
		return "", err
	}
	return path, nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AgentSearch Report — {{.Target}}</title>
    <style>
        :root { --bg: #0d1117; --card: #161b22; --border: #30363d; --text: #c9d1d9; --muted: #8b949e; --green: #3fb950; --red: #f85149; --yellow: #d29922; --blue: #58a6ff; }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; background: var(--bg); color: var(--text); line-height: 1.6; padding: 2rem; }
        .container { max-width: 1200px; margin: 0 auto; }
        header { text-align: center; margin-bottom: 2rem; padding-bottom: 2rem; border-bottom: 1px solid var(--border); }
        h1 { color: var(--blue); font-size: 2rem; margin-bottom: 0.5rem; }
        .meta { color: var(--muted); font-size: 0.9rem; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
        .stat-card { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 1.5rem; text-align: center; }
        .stat-value { font-size: 2rem; font-weight: bold; }
        .stat-label { color: var(--muted); font-size: 0.85rem; margin-top: 0.25rem; }
        .found { color: var(--green); }
        .blocked { color: var(--yellow); }
        .error { color: var(--red); }
        .notfound { color: var(--muted); }
        table { width: 100%; border-collapse: collapse; margin-top: 1rem; font-size: 0.9rem; }
        th, td { padding: 0.75rem; text-align: left; border-bottom: 1px solid var(--border); }
        th { color: var(--muted); font-weight: 600; text-transform: uppercase; font-size: 0.75rem; letter-spacing: 0.05em; }
        tr:hover { background: rgba(88, 166, 255, 0.05); }
        .badge { display: inline-block; padding: 0.2rem 0.5rem; border-radius: 12px; font-size: 0.75rem; font-weight: 600; }
        .badge-found { background: rgba(63, 185, 80, 0.15); color: var(--green); }
        .badge-blocked { background: rgba(210, 153, 34, 0.15); color: var(--yellow); }
        .badge-error { background: rgba(248, 81, 73, 0.15); color: var(--red); }
        .badge-notfound { background: rgba(139, 148, 158, 0.15); color: var(--muted); }
        .url { color: var(--blue); text-decoration: none; word-break: break-all; }
        .url:hover { text-decoration: underline; }
        .section { margin-bottom: 2rem; }
        .section h2 { color: var(--blue); margin-bottom: 1rem; font-size: 1.25rem; }
        .confidence-bar { height: 6px; background: var(--border); border-radius: 3px; overflow: hidden; width: 100px; display: inline-block; vertical-align: middle; }
        .confidence-fill { height: 100%; background: var(--green); border-radius: 3px; }
        input[type="text"] { width: 100%; padding: 0.5rem; background: var(--card); border: 1px solid var(--border); color: var(--text); border-radius: 6px; margin-bottom: 1rem; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🔍 AgentSearch Report</h1>
            <p class="meta">Target: <strong>{{.Target}}</strong> | Duration: {{.Duration}} | Generated: {{.Timestamp}}</p>
        </header>

        <div class="stats">
            <div class="stat-card">
                <div class="stat-value">{{.Summary.Total}}</div>
                <div class="stat-label">Total Checks</div>
            </div>
            <div class="stat-card">
                <div class="stat-value found">{{.Summary.Found}}</div>
                <div class="stat-label">Found</div>
            </div>
            <div class="stat-card">
                <div class="stat-value notfound">{{.Summary.NotFound}}</div>
                <div class="stat-label">Not Found</div>
            </div>
            <div class="stat-card">
                <div class="stat-value blocked">{{.Summary.Blocked}}</div>
                <div class="stat-label">Blocked</div>
            </div>
            <div class="stat-card">
                <div class="stat-value error">{{.Summary.Errors}}</div>
                <div class="stat-label">Errors</div>
            </div>
        </div>

        <div class="section">
            <h2>Found Profiles</h2>
            <input type="text" id="searchFound" placeholder="Filter by site name..." onkeyup="filterTable('foundTable', this.value)">
            <table id="foundTable">
                <thead>
                    <tr><th>Site</th><th>Confidence</th><th>Status</th><th>Latency</th><th>URL</th></tr>
                </thead>
                <tbody>
                {{range .Results}}{{if eq .Status "found"}}
                    <tr>
                        <td>{{.SiteName}}</td>
                        <td>
                            <div class="confidence-bar"><div class="confidence-fill" style="width:{{.Confidence}}%"></div></div>
                            {{.Confidence}}%
                        </td>
                        <td><span class="badge badge-found">found</span></td>
                        <td>{{.Duration}}</td>
                        <td><a class="url" href="{{.URL}}" target="_blank">{{.URL}}</a></td>
                    </tr>
                {{end}}{{end}}
                </tbody>
            </table>
        </div>

        <div class="section">
            <h2>Blocked by WAF</h2>
            <table>
                <thead>
                    <tr><th>Site</th><th>URL</th></tr>
                </thead>
                <tbody>
                {{range .Results}}{{if eq .Status "blocked"}}
                    <tr>
                        <td>{{.SiteName}}</td>
                        <td><a class="url" href="{{.URL}}" target="_blank">{{.URL}}</a></td>
                    </tr>
                {{end}}{{end}}
                </tbody>
            </table>
        </div>

        <div class="section">
            <h2>Errors</h2>
            <table>
                <thead>
                    <tr><th>Site</th><th>Error</th><th>URL</th></tr>
                </thead>
                <tbody>
                {{range .Results}}{{if eq .Status "error"}}
                    <tr>
                        <td>{{.SiteName}}</td>
                        <td style="color:var(--red)">{{.Error}}</td>
                        <td><a class="url" href="{{.URL}}" target="_blank">{{.URL}}</a></td>
                    </tr>
                {{end}}{{end}}
                </tbody>
            </table>
        </div>
    </div>

    <script>
        function filterTable(tableId, query) {
            const rows = document.querySelectorAll('#' + tableId + ' tbody tr');
            const q = query.toLowerCase();
            rows.forEach(row => {
                row.style.display = row.textContent.toLowerCase().includes(q) ? '' : 'none';
            });
        }
    </script>
</body>
</html>`
