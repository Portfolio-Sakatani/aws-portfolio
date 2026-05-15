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

func main() {
	// ALBヘルスチェック用
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Healthy")
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
