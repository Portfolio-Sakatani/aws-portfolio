package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 累積リクエスト数（インスタンス単位）
// pageViewCount: "/" への実際のページ遷移（ブラウザで開いた回数）
// apiPollCount:  ダッシュボードJSが5秒ごとに送る自動更新のfetch回数
// healthCheckCount: ALBヘルスチェックの回数
var pageViewCount int64
var apiPollCount int64
var healthCheckCount int64

// 直近1件の「実ユーザーアクセス」「ALBヘルスチェック」の情報を保持
// 複数goroutine（複数リクエスト）から同時に読み書きされるためmutexで保護する
var (
	lastAccessMu     sync.Mutex
	lastUserAccess   *RequestInfo
	lastHealthCheck  *RequestInfo
)

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

// ALB(X-Forwarded-For)を考慮して実際のクライアントIPを取得する関数
// X-Forwarded-For は "クライアントIP, プロキシ1のIP, プロキシ2のIP..." の形式で
// 複数のIPが入ることがあるため、一番左（最初にリクエストしたクライアント）を採用する
func getClientIP(r *http.Request) string {
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}
	// X-Forwarded-Forが無い場合（直接アクセスなど）はRemoteAddrにフォールバック
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ALBのヘルスチェックかどうかを判定する関数
// ALBはヘルスチェック時にUser-Agentへ "ELB-HealthChecker/2.0" のような文字列を付与するため、
// それを手がかりに実ユーザーのアクセスと区別する
func isHealthCheck(r *http.Request) bool {
	return strings.Contains(r.UserAgent(), "ELB-HealthChecker")
}

// レスポンス全体の構造体
type SystemResponse struct {
	Service         string       `json:"service"`
	Time            string       `json:"time"`
	ServerInfo      ServerInfo   `json:"server_info"`
	LastUserAccess  *RequestInfo `json:"last_user_access"`
	LastHealthCheck *RequestInfo `json:"last_health_check"`
}

// サーバー側の実行環境情報
type ServerInfo struct {
	Hostname         string `json:"hostname"`
	Region           string `json:"region"`
	AZ               string `json:"az"`
	PageViewCount    int64  `json:"page_view_count"`
	ApiPollCount     int64  `json:"api_poll_count"`
	HealthCheckCount int64  `json:"health_check_count"`
}

// ALB経由で渡されるクライアント接続情報
type RequestInfo struct {
	ClientIP      string `json:"client_ip"`
	ForwardedFor  string `json:"forwarded_for"`
	TraceID       string `json:"trace_id"`
	UserAgent     string `json:"ua"`
	IsHealthCheck bool   `json:"is_health_check"`
	ObservedAt    string `json:"observed_at"`
}

// 現在のリクエストからRequestInfoを組み立てるヘルパー関数
func buildRequestInfo(r *http.Request, healthCheck bool) RequestInfo {
	return RequestInfo{
		ClientIP:      getClientIP(r),
		ForwardedFor:  r.Header.Get("X-Forwarded-For"),
		TraceID:       r.Header.Get("X-Amzn-Trace-Id"),
		UserAgent:     r.UserAgent(),
		IsHealthCheck: healthCheck,
		ObservedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

// 直近のアクセス情報を種別ごとに更新する
func recordAccess(info RequestInfo) {
	lastAccessMu.Lock()
	defer lastAccessMu.Unlock()
	if info.IsHealthCheck {
		lastHealthCheck = &info
	} else {
		lastUserAccess = &info
	}
}

// 直近のアクセス情報のスナップショットを取得する
func snapshotLastAccess() (userAccess *RequestInfo, healthCheck *RequestInfo) {
	lastAccessMu.Lock()
	defer lastAccessMu.Unlock()
	return lastUserAccess, lastHealthCheck
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
    </div>

    <div class="card">
      <h2>Traffic</h2>
      <div class="row"><span class="label">Page Views(実訪問)</span><span class="value" id="pageViewCount">-</span></div>
      <div class="row"><span class="label">API Polls(自動更新分)</span><span class="value" id="apiPollCount">-</span></div>
      <div class="row"><span class="label">ALB Health Checks</span><span class="value" id="healthCount">-</span></div>
    </div>

    <div class="card">
      <h2>Last User Access</h2>
      <div class="row"><span class="label">Client IP</span><span class="value" id="userClientIp">-</span></div>
      <div class="row"><span class="label">X-Forwarded-For</span><span class="value" id="userXff">-</span></div>
      <div class="row"><span class="label">Trace ID</span><span class="value" id="userTraceId">-</span></div>
      <div class="row"><span class="label">User-Agent</span><span class="value" id="userUa">-</span></div>
      <div class="row"><span class="label">Observed At (UTC)</span><span class="value" id="userObservedAt">-</span></div>
    </div>

    <div class="card">
      <h2>Last ALB Health Check</h2>
      <div class="row"><span class="label">Client IP</span><span class="value" id="healthClientIp">-</span></div>
      <div class="row"><span class="label">X-Forwarded-For</span><span class="value" id="healthXff">-</span></div>
      <div class="row"><span class="label">Trace ID</span><span class="value" id="healthTraceId">-</span></div>
      <div class="row"><span class="label">User-Agent</span><span class="value" id="healthUa">-</span></div>
      <div class="row"><span class="label">Observed At (UTC)</span><span class="value" id="healthObservedAt">-</span></div>
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
      document.getElementById('pageViewCount').textContent = data.server_info.page_view_count;
      document.getElementById('apiPollCount').textContent = data.server_info.api_poll_count;
      document.getElementById('healthCount').textContent = data.server_info.health_check_count;

      const setCard = (prefix, info) => {
        if (!info) {
          document.getElementById(prefix + 'ClientIp').textContent = '(まだ記録がありません)';
          document.getElementById(prefix + 'Xff').textContent = '-';
          document.getElementById(prefix + 'TraceId').textContent = '-';
          document.getElementById(prefix + 'Ua').textContent = '-';
          document.getElementById(prefix + 'ObservedAt').textContent = '-';
          return;
        }
        document.getElementById(prefix + 'ClientIp').textContent = info.client_ip || '-';
        document.getElementById(prefix + 'Xff').textContent = info.forwarded_for || '-';
        document.getElementById(prefix + 'TraceId').textContent = info.trace_id || '-';
        document.getElementById(prefix + 'Ua').textContent = info.ua || '-';
        document.getElementById(prefix + 'ObservedAt').textContent = info.observed_at || '-';
      };
      setCard('user', data.last_user_access);
      setCard('health', data.last_health_check);

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
		// ALBのヘルスチェックはこのパスに定期アクセスしてくるため、ここで種別ごとに記録する
		healthCheck := isHealthCheck(r)
		if healthCheck {
			atomic.AddInt64(&healthCheckCount, 1)
		} else {
			// "/" への直接アクセスは「実際にページを開いた」とみなす
			atomic.AddInt64(&pageViewCount, 1)
		}
		recordAccess(buildRequestInfo(r, healthCheck))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, indexHTML)
	})

	// システム情報API（ブラウザのJSが5秒ごとにfetchする。ヘルスチェックからは通常呼ばれない想定だが念のため同様に分岐）
	// ブラウザで直接このURLを開いた場合（Accept: text/htmlを含む場合）は、
	// "/" と同じダッシュボードHTMLを返す。JSからのfetch呼び出し等はJSONを返す。
	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		healthCheck := isHealthCheck(r)
		wantsHTML := strings.Contains(r.Header.Get("Accept"), "text/html")

		if healthCheck {
			atomic.AddInt64(&healthCheckCount, 1)
		} else if wantsHTML {
			// ブラウザで直接 /api/info を開いた場合も「実際にページを開いた」とみなす
			atomic.AddInt64(&pageViewCount, 1)
		} else {
			// JSからの自動更新fetchはポーリングとしてカウント
			atomic.AddInt64(&apiPollCount, 1)
		}
		recordAccess(buildRequestInfo(r, healthCheck))

		if wantsHTML {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, indexHTML)
			return
		}

		hostname, _ := os.Hostname()
		lastUser, lastHealth := snapshotLastAccess()
		response := SystemResponse{
			Service: "portfolio-app",
			Time:    time.Now().UTC().Format(time.RFC3339),
			ServerInfo: ServerInfo{
				Hostname:         hostname,
				Region:           os.Getenv("AWS_REGION"),
				AZ:               getAZ(),
				PageViewCount:    atomic.LoadInt64(&pageViewCount),
				ApiPollCount:     atomic.LoadInt64(&apiPollCount),
				HealthCheckCount: atomic.LoadInt64(&healthCheckCount),
			},
			LastUserAccess:  lastUser,
			LastHealthCheck: lastHealth,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
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
