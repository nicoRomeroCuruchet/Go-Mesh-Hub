package dashboard

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"go-mesh-hub/internal/router"
)

// Start launches the HTTP server in a blocking manner (call it with 'go')
func Start(port int, table *router.Table) {
	// Register Handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		renderHome(w, table)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	log.Printf("[WEB] Dashboard running at http://%s", addr)
	
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Printf("[ERR] Dashboard stopped: %v", err)
	}
}

func renderHome(w http.ResponseWriter, table *router.Table) {
	// 1. Get Data Snapshot
	peers := table.Snapshot()

	// 2. Prepare View Data
	type Row struct {
		VirtualIP string
		RealIP    string
		Status    string // Online/Offline
		RowClass  string // Bootstrap class (success, danger)
		LastSeen  string
		Rx        string
		Tx        string
	}

	var rows []Row
	now := time.Now()

	for _, p := range peers {
		timeDiff := now.Sub(p.LastSeen)
		status := "Online"
		rowClass := "table-success"

		// Logic to determine health
		if timeDiff > 60*time.Second {
			status = "Offline"
			rowClass = "table-danger"
		} else if timeDiff > 25*time.Second {
			status = "Idle/Lag"
			rowClass = "table-warning"
		}

		rows = append(rows, Row{
			VirtualIP: p.VirtualIP,
			RealIP:    p.RealAddr,
			Status:    status,
			RowClass:  rowClass,
			LastSeen:  fmt.Sprintf("%.0fs ago", timeDiff.Seconds()),
			Rx:        formatBytes(p.RxBytes),
			Tx:        formatBytes(p.TxBytes),
		})
	}

	// 3. Render Template
	tmpl, err := template.New("index").Parse(htmlTemplate)
	if err != nil {
		http.Error(w, "Internal Template Error", 500)
		return
	}
	tmpl.Execute(w, rows)
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Embedded HTML for single-binary distribution
const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="3"> <title>VPN Dashboard</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
    <style>
        body { background-color: #f0f2f5; padding-top: 30px; }
        .card { border: none; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }
    </style>
</head>
<body>
<div class="container">
    <div class="card">
        <div class="card-header bg-dark text-white p-3">
            <h4 class="mb-0">🛰️ VPN Mesh Control Center</h4>
        </div>
        <div class="card-body">
            <table class="table table-hover align-middle">
                <thead class="table-light">
                    <tr>
                        <th>Virtual IP</th>
                        <th>Real Address (WAN)</th>
                        <th>Status</th>
                        <th>Last Seen</th>
                        <th>Data In (Rx)</th>
                        <th>Data Out (Tx)</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .}}
                    <tr class="{{.RowClass}}">
                        <td class="fw-bold">{{.VirtualIP}}</td>
                        <td>{{.RealIP}}</td>
                        <td><span class="badge bg-secondary">{{.Status}}</span></td>
                        <td>{{.LastSeen}}</td>
                        <td>{{.Rx}}</td>
                        <td>{{.Tx}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
            {{if not .}}
                <div class="text-center text-muted py-4">No peers connected yet. Waiting for heartbeats...</div>
            {{end}}
        </div>
        <div class="card-footer text-muted text-end">
            <small>System Active • Auto-refreshing</small>
        </div>
    </div>
</div>
</body>
</html>
`