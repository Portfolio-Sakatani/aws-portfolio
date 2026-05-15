# ビルド用のステージ
FROM golang:1.26.1-alpine AS builder

# gitをインストール
RUN apk add --no-cache git

# 作業ディレクトリを/appに設定
WORKDIR /app

# 依存関係ファイルをコピーしてライブラリをダウンロード
COPY go.mod go.sum ./
RUN go mod download

# ソースコードをコピーしてビルドを実行
# CGO_ENABLED=0により、外部ライブラリに依存しない静的バイナリを作成
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /main .

# 実行用の軽量ステージ
FROM alpine:3.21

# セキュリティのため一般ユーザーを作成して切り替え
RUN adduser -D appuser
USER appuser

# ビルドステージから実行ファイルのみをコピー
COPY --from=builder /main /main

# コンテナが使用するポート
EXPOSE 8080

# アプリケーションを実行
CMD ["/main"]
