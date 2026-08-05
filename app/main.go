package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// 累積リクエスト数（インスタンス単位）
var requestCount int64

// ECSタスクメタデータ構造体
type TaskMetadataV4 struct {
	AvailabilityZone string `json:"AvailabilityZone"`
}

// 動的にAZを取得する関数
func getAZ() string {
	endpoint := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if endpoint == "" {
		return "unknown"
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint + "/task")
	if err != nil {
		return "error-fetching-az"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var meta TaskMetadataV4
	json.Unmarshal(body, &meta)
	return meta.AvailabilityZone
}

// レスポンス全体の構造体
type SystemResponse struct {
	Service     string      `json:"service"`
	Time        string      `json:"time"`
	ServerInfo  ServerInfo  `json:"server_info"`
	RequestInfo RequestInfo `json:"request_info"`
}

// サーバー側の実行環境情報
type ServerInfo struct {
	Hostname     string `json:"hostname"`
	Region       string `json:"region"`
	AZ           string `json:"az"`
	RequestCount int64  `json:"instance_request_count"`
}

// ALB経由で渡されるクライアント接続情報
type RequestInfo struct {
	ClientIP     string `json:"client_ip"`
	ForwardedFor string `json:"forwarded_for"`
	TraceID      string `json:"trace_id"`
	UserAgent    string `json:"ua"`
}

// トップページ（ダッシュボードUI）のHTML
const indexHTML = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Portfolio App - Status</title>
<style>
  :root {
    --bg: #f4f6f8;
    --card-bg: #ffffff;
    --border: #e2e6ea;
    --text: #1f2933;
    --text-sub: #6b7280;
    --accent: #2563eb;
    --ok: #16a34a;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Hiragino Kaku Gothic ProN", Meiryo, sans-serif;
    background: var(--bg);
    color: var(--text);
    padding: 24px;
  }
  .container {
    max-width: 720px;
    margin: 0 auto;
  }
  header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 20px;
  }
  h1 {
    font-size: 20px;
    margin: 0;
  }
  .status-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--ok);
    background: #ecfdf3;
    border: 1px solid #b7ebc6;
    padding: 4px 10px;
    border-radius: 999px;
  }
  .status-pill .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--ok);
  }
  .card {
    background: var(--card-bg);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 20px 24px;
    margin-bottom: 16px;
    box-shadow: 0 1px 2px rgba(0,0,0,0.03);
  }
  .card h2 {
    font-size: 14px;
    color: var(--text-sub);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin: 0 0 14px 0;
  }
  .row {
    display: flex;
    justify-content: space-between;
    padding: 8px 0;
    border-bottom: 1px solid #f0f2f4;
    font-size: 14px;
  }
  .row:last-child { border-bottom: none; }
  .row .label { color: var(--text-sub); }
  .row .value {
    font-weight: 600;
    text-align: right;
    word-break: break-all;
    max-width: 60%;
  }
  .footer {
    text-align: center;
    color: var(--text-sub);
    font-size: 12px;
    margin-top: 16px;
  }
  .error {
    color: #b91c1c;
    background: #fef2f2;
    border: 1px solid #fecaca;
    padding: 12px 16px;
    border-radius: 8px;
    font-size: 14px;
  }
</style>
</head>
<body>
  <div class="container">
    <header>
      <h1>Portfolio App</h1>
      <span class="status-pill"><span class="dot"></span>Running</span>
    </header>

    <div class="card">
      <h2>Server Info</h2>
      <div class="row"><span class="label">Hostname</span><span class="value" id="hostname">-</span></div>
      <div class="row"><span class="label">Region</span><span class="value" id="region">-</span></div>
      <div class="row"><span class="label">Availability Zone</span><span class="value" id="az">-</span></div>
      <div class="row"><span class="label">Request Count</span><span class="value" id="count">-</span></div>
    </div>

    <div class="card">
      <h2>Request Info</h2>
      <div class="row"><span class="label">Client IP</span><span class="value" id="clientIp">-</span></div>
      <div class="row"><span class="label">X-Forwarded-For</span><span class="value" id="xff">-</span></div>
      <div class="row"><span class="label">Trace ID</span><span class="value" id="traceId">-</span></div>
      <div class="row"><span class="label">User-Agent</span><span class="value" id="ua">-</span></div>
    </div>

    <div class="card">
      <h2>Timestamp</h2>
      <div class="row"><span class="label">Server Time (UTC)</span><span class="value" id="time">-</span></div>
      <div class="row"><span class="label">Last Updated</span><span class="value" id="lastUpdated">-</span></div>
    </div>

    <div id="errorBox"></div>
    <div class="footer">5秒ごとに自動更新されます</div>
  </div>

<script>
  async function refresh() {
    const errorBox = document.getElementById('errorBox');
    try {
      const res = await fetch('/api/info', { cache: 'no-store' });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();

      document.getElementById('hostname').textContent = data.server_info.hostname || '-';
      document.getElementById('region').textContent = data.server_info.region || '-';
      document.getElementById('az').textContent = data.server_info.az || '-';
      document.getElementById('count').textContent = data.server_info.instance_request_count;

      document.getElementById('clientIp').textContent = data.request_info.client_ip || '-';
      document.getElementById('xff').textContent = data.request_info.forwarded_for || '-';
      document.getElementById('traceId').textContent = data.request_info.trace_id || '-';
      document.getElementById('ua').textContent = data.request_info.ua || '-';

      document.getElementById('time').textContent = data.time || '-';
      document.getElementById('lastUpdated').textContent = new Date().toLocaleTimeString('ja-JP');

      errorBox.innerHTML = '';
    } catch (e) {
      errorBox.innerHTML = '<div class="error">情報の取得に失敗しました: ' + e.message + '</div>';
    }
  }

  refresh();
  setInterval(refresh, 5000);
</script>
</body>
</html>`

func main() {
	// トップページ：シンプルなステータス確認用Web UI（ALBヘルスチェックにも200を返す）
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, indexHTML)
	})

	// システム情報API
	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		hostname, _ := os.Hostname()
		response := SystemResponse{
			Service: "portfolio-app",
			Time:    time.Now().UTC().Format(time.RFC3339),
			ServerInfo: ServerInfo{
				Hostname:     hostname,
				Region:       os.Getenv("AWS_REGION"),
				AZ:           getAZ(),
				RequestCount: atomic.LoadInt64(&requestCount),
			},
			RequestInfo: RequestInfo{
				ClientIP:     r.RemoteAddr,
				ForwardedFor: r.Header.Get("X-Forwarded-For"),
				TraceID:      r.Header.Get("X-Amzn-Trace-Id"),
				UserAgent:    r.UserAgent(),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		encoder.Encode(response)
	})

	// ポート8080でサーバーを起動
	fmt.Println("Server starting on :8080...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Error starting server: %s\n", err)
	}
}
