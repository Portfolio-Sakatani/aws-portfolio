package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// 累積リクエスト数（ECSタスク単位）
var requestCount int64

var httpClient = &http.Client{
	Timeout: 2 * time.Second,
}

// ECSタスクメタデータ
type TaskMetadataV4 struct {
	AvailabilityZone string `json:"AvailabilityZone"`
}

// APIレスポンス
type SystemResponse struct {
	Service     string      `json:"service"`
	Time        string      `json:"time"`
	ServerInfo  ServerInfo  `json:"server_info"`
	RequestInfo RequestInfo `json:"request_info"`
}

type ServerInfo struct {
	Hostname     string `json:"hostname"`
	Region       string `json:"region"`
	AZ           string `json:"az"`
	RequestCount int64  `json:"instance_request_count"`
}

type RequestInfo struct {
	ClientIP     string `json:"client_ip"`
	ForwardedFor string `json:"forwarded_for"`
	TraceID      string `json:"trace_id"`
	UserAgent    string `json:"ua"`
}

var pageTemplate = template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html lang="ja">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Portfolio App</title>

	<style>
		:root {
			color-scheme: light dark;

			--background: #f5f5f5;
			--surface: #ffffff;
			--text: #1f2328;
			--subtext: #656d76;
			--border: #d0d7de;
			--success: #1a7f37;
			--error: #cf222e;
		}

		@media (prefers-color-scheme: dark) {
			:root {
				--background: #0d1117;
				--surface: #161b22;
				--text: #e6edf3;
				--subtext: #8b949e;
				--border: #30363d;
				--success: #3fb950;
				--error: #f85149;
			}
		}

		* {
			box-sizing: border-box;
		}

		body {
			margin: 0;
			padding: 32px 16px;
			background: var(--background);
			color: var(--text);
			font-family:
				-apple-system,
				BlinkMacSystemFont,
				"Segoe UI",
				sans-serif;
			line-height: 1.5;
		}

		main {
			width: 100%;
			max-width: 760px;
			margin: 0 auto;
		}

		header {
			display: flex;
			align-items: center;
			justify-content: space-between;
			gap: 16px;
			margin-bottom: 20px;
		}

		h1 {
			margin: 0;
			font-size: 24px;
			font-weight: 600;
		}

		.status {
			display: flex;
			align-items: center;
			gap: 7px;
			color: var(--subtext);
			font-size: 14px;
		}

		.status-dot {
			width: 9px;
			height: 9px;
			border-radius: 50%;
			background: var(--success);
		}

		.status-dot.error {
			background: var(--error);
		}

		.card {
			margin-bottom: 16px;
			padding: 20px;
			background: var(--surface);
			border: 1px solid var(--border);
			border-radius: 8px;
		}

		h2 {
			margin: 0 0 16px;
			font-size: 16px;
			font-weight: 600;
		}

		dl {
			display: grid;
			grid-template-columns: 180px minmax(0, 1fr);
			margin: 0;
		}

		dt,
		dd {
			margin: 0;
			padding: 9px 0;
			border-bottom: 1px solid var(--border);
		}

		dt {
			color: var(--subtext);
		}

		dd {
			overflow-wrap: anywhere;
		}

		dt:last-of-type,
		dd:last-of-type {
			border-bottom: none;
		}

		.actions {
			display: flex;
			align-items: center;
			justify-content: space-between;
			gap: 16px;
			margin-top: 16px;
		}

		button {
			padding: 8px 14px;
			background: var(--surface);
			color: var(--text);
			border: 1px solid var(--border);
			border-radius: 6px;
			font: inherit;
			cursor: pointer;
		}

		button:hover {
			background: var(--background);
		}

		button:disabled {
			cursor: wait;
			opacity: 0.6;
		}

		.updated-at {
			color: var(--subtext);
			font-size: 13px;
		}

		.error-message {
			display: none;
			margin-bottom: 16px;
			padding: 12px 16px;
			color: var(--error);
			background: var(--surface);
			border: 1px solid var(--error);
			border-radius: 8px;
		}

		@media (max-width: 600px) {
			body {
				padding-top: 20px;
			}

			header {
				align-items: flex-start;
				flex-direction: column;
			}

			dl {
				display: block;
			}

			dt {
				padding-bottom: 2px;
				border-bottom: none;
				font-size: 13px;
			}

			dd {
				padding-top: 0;
			}

			.actions {
				align-items: flex-start;
				flex-direction: column;
			}
		}
	</style>
</head>

<body>
	<main>
		<header>
			<h1>Portfolio App</h1>

			<div class="status">
				<span id="status-dot" class="status-dot"></span>
				<span id="status-text">Running</span>
			</div>
		</header>

		<div id="error-message" class="error-message"></div>

		<section class="card">
			<h2>Server</h2>

			<dl>
				<dt>Service</dt>
				<dd id="service">-</dd>

				<dt>Hostname</dt>
				<dd id="hostname">-</dd>

				<dt>Region</dt>
				<dd id="region">-</dd>

				<dt>Availability Zone</dt>
				<dd id="az">-</dd>

				<dt>Request Count</dt>
				<dd id="request-count">-</dd>

				<dt>Server Time</dt>
				<dd id="server-time">-</dd>
			</dl>
		</section>

		<section class="card">
			<h2>Request</h2>

			<dl>
				<dt>Client IP</dt>
				<dd id="client-ip">-</dd>

				<dt>Forwarded For</dt>
				<dd id="forwarded-for">-</dd>

				<dt>Trace ID</dt>
				<dd id="trace-id">-</dd>

				<dt>User Agent</dt>
				<dd id="user-agent">-</dd>
			</dl>

			<div class="actions">
				<button id="refresh-button" type="button">
					Refresh
				</button>

				<span id="updated-at" class="updated-at"></span>
			</div>
		</section>
	</main>

	<script>
		const valueOrDash = (value) => {
			if (value === null || value === undefined || value === "") {
				return "-";
			}

			return String(value);
		};

		const setText = (id, value) => {
			document.getElementById(id).textContent = valueOrDash(value);
		};

		const setStatus = (healthy, message) => {
			const dot = document.getElementById("status-dot");
			const text = document.getElementById("status-text");

			dot.classList.toggle("error", !healthy);
			text.textContent = message;
		};

		const showError = (message) => {
			const element = document.getElementById("error-message");

			element.textContent = message;
			element.style.display = "block";
		};

		const hideError = () => {
			document.getElementById("error-message").style.display = "none";
		};

		async function loadSystemInfo() {
			const button = document.getElementById("refresh-button");

			button.disabled = true;
			setStatus(true, "Loading");
			hideError();

			try {
				const response = await fetch("/api/info", {
					cache: "no-store"
				});

				if (!response.ok) {
					throw new Error("HTTP " + response.status);
				}

				const data = await response.json();

				setText("service", data.service);
				setText("hostname", data.server_info.hostname);
				setText("region", data.server_info.region);
				setText("az", data.server_info.az);
				setText(
					"request-count",
					data.server_info.instance_request_count
				);
				setText(
					"server-time",
					new Date(data.time).toLocaleString()
				);

				setText("client-ip", data.request_info.client_ip);
				setText(
					"forwarded-for",
					data.request_info.forwarded_for
				);
				setText("trace-id", data.request_info.trace_id);
				setText("user-agent", data.request_info.ua);

				document.getElementById("updated-at").textContent =
					"Updated: " + new Date().toLocaleTimeString();

				setStatus(true, "Running");
			} catch (error) {
				setStatus(false, "Unavailable");
				showError("Failed to load system information.");
				console.error(error);
			} finally {
				button.disabled = false;
			}
		}

		document
			.getElementById("refresh-button")
			.addEventListener("click", loadSystemInfo);

		loadSystemInfo();
	</script>
</body>
</html>
`))

func main() {
	mux := http.NewServeMux()

	// ブラウザ用画面
	mux.HandleFunc("/", handleIndex)

	// ALBヘルスチェック用
	mux.HandleFunc("/health", handleHealth)

	// システム情報API
	mux.HandleFunc("/api/info", handleInfo)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Println("Server starting on :8080")

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := pageTemplate.Execute(w, nil); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Healthy"))
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	count := atomic.AddInt64(&requestCount, 1)

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	response := SystemResponse{
		Service: "portfolio-app",
		Time:    time.Now().UTC().Format(time.RFC3339),
		ServerInfo: ServerInfo{
			Hostname:     hostname,
			Region:       environmentOrDefault("AWS_REGION", "unknown"),
			AZ:           getAZ(),
			RequestCount: count,
		},
		RequestInfo: RequestInfo{
			ClientIP:     getClientIP(r),
			ForwardedFor: r.Header.Get("X-Forwarded-For"),
			TraceID:      r.Header.Get("X-Amzn-Trace-Id"),
			UserAgent:    r.UserAgent(),
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(response); err != nil {
		log.Printf("JSON encode error: %v", err)
	}
}

func getAZ() string {
	endpoint := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if endpoint == "" {
		return "unknown"
	}

	resp, err := httpClient.Get(endpoint + "/task")
	if err != nil {
		log.Printf("metadata request error: %v", err)
		return "unknown"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("metadata returned status: %d", resp.StatusCode)
		return "unknown"
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("metadata read error: %v", err)
		return "unknown"
	}

	var metadata TaskMetadataV4

	if err := json.Unmarshal(body, &metadata); err != nil {
		log.Printf("metadata JSON error: %v", err)
		return "unknown"
	}

	if metadata.AvailabilityZone == "" {
		return "unknown"
	}

	return metadata.AvailabilityZone
}

func getClientIP(r *http.Request) string {
	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}

func environmentOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()

		next.ServeHTTP(w, r)

		log.Printf(
			"%s %s remote=%s duration=%s",
			r.Method,
			r.URL.Path,
			getClientIP(r),
			time.Since(startedAt),
		)
	})
}
