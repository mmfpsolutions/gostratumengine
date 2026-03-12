"""
Copyright 2026 Scott Walter, MMFP Solutions LLC

This program is free software; you can redistribute it and/or modify it
under the terms of the GNU General Public License as published by the Free
Software Foundation; either version 3 of the License, or (at your option)
any later version.  See LICENSE for more details.
"""
from http.server import BaseHTTPRequestHandler, HTTPServer
import urllib.request
import json
import os

API_BASE = os.environ.get("API_BASE", "http://127.0.0.1:8080")
LISTEN_PORT = int(os.environ.get("LISTEN_PORT", "8000"))

HTML_PAGE = """<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GoStratumEngine Dashboard</title>
<style>
  :root {
    --bg: #0f1117;
    --surface: #1a1d27;
    --border: #2a2d3a;
    --text: #e0e0e0;
    --text-dim: #888;
    --accent: #4a9eff;
    --green: #34d399;
    --red: #f87171;
    --yellow: #fbbf24;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace;
    background: var(--bg);
    color: var(--text);
    padding: 20px;
    max-width: 1200px;
    margin: 0 auto;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 24px;
    padding-bottom: 16px;
    border-bottom: 1px solid var(--border);
  }
  header h1 { font-size: 1.4em; font-weight: 600; }
  #health-badge {
    padding: 4px 12px;
    border-radius: 12px;
    font-size: 0.85em;
    font-weight: 600;
  }
  .health-ok { background: #0d3320; color: var(--green); }
  .health-err { background: #3b1111; color: var(--red); }
  .health-loading { background: #2a2210; color: var(--yellow); }

  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; margin-bottom: 24px; }
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 16px;
  }
  .card-label { font-size: 0.75em; text-transform: uppercase; color: var(--text-dim); margin-bottom: 4px; }
  .card-value { font-size: 1.5em; font-weight: 700; }

  h2 { font-size: 1.1em; margin-bottom: 12px; color: var(--text-dim); }
  .section { margin-bottom: 28px; }

  table {
    width: 100%;
    border-collapse: collapse;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    overflow: hidden;
  }
  th, td { padding: 10px 14px; text-align: left; }
  th {
    background: var(--border);
    font-size: 0.75em;
    text-transform: uppercase;
    color: var(--text-dim);
    font-weight: 600;
  }
  td { font-size: 0.9em; border-top: 1px solid var(--border); }
  tr:hover td { background: rgba(74, 158, 255, 0.04); }

  .coin-tag {
    display: inline-block;
    background: var(--accent);
    color: #fff;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.8em;
    font-weight: 600;
  }
  .refresh-info { font-size: 0.75em; color: var(--text-dim); }
  .empty-msg { color: var(--text-dim); font-style: italic; padding: 20px; text-align: center; }
</style>
</head>
<body>

<header>
  <h1>GoStratumEngine</h1>
  <div>
    <span id="health-badge" class="health-loading">checking...</span>
    <span class="refresh-info" style="margin-left:12px">auto-refresh 10s</span>
  </div>
</header>

<div id="pool-info" class="cards"></div>

<div class="section">
  <h2>Coin Statistics</h2>
  <div id="coin-stats"><p class="empty-msg">Loading...</p></div>
</div>

<div class="section">
  <h2>Connected Miners</h2>
  <div id="miners"><p class="empty-msg">Loading...</p></div>
</div>

<script>
function fmt(n) { return Number(n).toLocaleString(); }

function fmtUptime(secs) {
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = Math.floor(secs % 60);
  const parts = [];
  if (d) parts.push(d + "d");
  if (h) parts.push(h + "h");
  if (m) parts.push(m + "m");
  parts.push(s + "s");
  return parts.join(" ");
}

function fmtTime(ts) {
  if (!ts || ts.startsWith("0001-")) return "-";
  const d = new Date(ts);
  if (isNaN(d)) return "-";
  return d.toLocaleString();
}

function proxy(path) {
  return fetch("/api/" + path).then(r => { if (!r.ok) throw new Error(r.status); return r.json(); });
}

async function refresh() {
  // Health
  const badge = document.getElementById("health-badge");
  try {
    const h = await proxy("health");
    badge.textContent = h.status || "ok";
    badge.className = "health-ok";
  } catch {
    badge.textContent = "unreachable";
    badge.className = "health-err";
  }

  // Stats
  try {
    const stats = await proxy("stats");
    // Pool info cards
    const coins = stats.coins || {};
    const coinKeys = Object.keys(coins);
    let totalAccepted = 0, totalRejected = 0, totalBlocks = 0;
    coinKeys.forEach(c => {
      totalAccepted += coins[c].shares_accepted || 0;
      totalRejected += coins[c].shares_rejected || 0;
      totalBlocks += coins[c].blocks_found || 0;
    });
    document.getElementById("pool-info").innerHTML =
      card("Pool Name", stats.pool_name || "-") +
      card("Uptime", fmtUptime(stats.uptime_seconds || 0)) +
      card("Coins", coinKeys.length) +
      card("Total Shares", fmt(totalAccepted + totalRejected)) +
      card("Blocks Found", fmt(totalBlocks));

    // Coin table
    if (coinKeys.length === 0) {
      document.getElementById("coin-stats").innerHTML = '<p class="empty-msg">No coins configured</p>';
    } else {
      let html = "<table><tr><th>Coin</th><th>Accepted</th><th>Rejected</th><th>Stale</th><th>Blocks</th><th>Last Block</th></tr>";
      coinKeys.forEach(c => {
        const s = coins[c];
        html += "<tr>" +
          '<td><span class="coin-tag">' + esc(c) + "</span></td>" +
          "<td>" + fmt(s.shares_accepted || 0) + "</td>" +
          "<td>" + fmt(s.shares_rejected || 0) + "</td>" +
          "<td>" + fmt(s.shares_stale || 0) + "</td>" +
          "<td>" + fmt(s.blocks_found || 0) + "</td>" +
          "<td>" + fmtTime(s.last_block_time) + "</td></tr>";
      });
      html += "</table>";
      document.getElementById("coin-stats").innerHTML = html;
    }
  } catch {
    document.getElementById("pool-info").innerHTML = "";
    document.getElementById("coin-stats").innerHTML = '<p class="empty-msg">Could not load stats</p>';
  }

  // Miners
  try {
    const data = await proxy("miners");
    const miners = data.miners || {};
    const coinKeys = Object.keys(miners);
    let rows = [];
    coinKeys.forEach(c => {
      (miners[c] || []).forEach(m => { rows.push({coin: c, ...m}); });
    });
    if (rows.length === 0) {
      document.getElementById("miners").innerHTML = '<p class="empty-msg">No miners connected</p>';
    } else {
      let html = "<table><tr><th>Coin</th><th>Worker</th><th>Address</th><th>Difficulty</th><th>Best Diff</th><th>Accepted</th><th>Rejected</th><th>Stale</th><th>Blocks</th><th>Connected</th><th>Last Share</th></tr>";
      rows.forEach(m => {
        html += "<tr>" +
          '<td><span class="coin-tag">' + esc(m.coin) + "</span></td>" +
          "<td>" + esc(m.worker_name || "-") + "</td>" +
          "<td>" + esc(m.remote_addr || "-") + "</td>" +
          "<td>" + fmt(m.difficulty || 0) + "</td>" +
          "<td>" + fmt(m.best_difficulty || 0) + "</td>" +
          "<td>" + fmt(m.shares_accepted || 0) + "</td>" +
          "<td>" + fmt(m.shares_rejected || 0) + "</td>" +
          "<td>" + fmt(m.shares_stale || 0) + "</td>" +
          "<td>" + fmt(m.blocks_found || 0) + "</td>" +
          "<td>" + fmtTime(m.connected_at) + "</td>" +
          "<td>" + fmtTime(m.last_share_time) + "</td></tr>";
      });
      html += "</table>";
      document.getElementById("miners").innerHTML = html;
    }
  } catch {
    document.getElementById("miners").innerHTML = '<p class="empty-msg">Could not load miners</p>';
  }
}

function card(label, value) {
  return '<div class="card"><div class="card-label">' + esc(String(label)) + '</div><div class="card-value">' + esc(String(value)) + "</div></div>";
}

function esc(s) {
  const d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

refresh();
setInterval(refresh, 10000);
</script>
</body>
</html>"""


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/':
            self.send_response(200)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(HTML_PAGE.encode())

        elif self.path.startswith('/api/'):
            # Proxy /api/<endpoint> to the Go API at /api/v1/<endpoint>
            endpoint = self.path[len('/api/'):]
            try:
                url = f"{API_BASE}/api/v1/{endpoint}"
                resp = urllib.request.urlopen(url, timeout=5)
                data = resp.read()
                self.send_response(200)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(data)
            except Exception:
                self.send_response(502)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"error": "upstream unavailable"}).encode())

        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        # Quieter logging: just method + path + status
        pass


if __name__ == '__main__':
    print(f"Dashboard running at http://localhost:{LISTEN_PORT}")
    print(f"Proxying API requests to {API_BASE}")
    server = HTTPServer(('', LISTEN_PORT), Handler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down.")
        server.server_close()
