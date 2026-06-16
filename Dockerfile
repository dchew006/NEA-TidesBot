# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot main.go graphing.go

# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Install Chromium and system dependencies for go-rod
RUN apt-get update && apt-get install -y \
    chromium \
    fonts-liberation \
    fontconfig \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy compiled binary and necessary files verbatim
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/template.html .
COPY --from=builder /app/tide_data.json . 

# Set environment variable pointing to the standard Chromium location
ENV LAUNCHER_BIN=/usr/bin/chromium

CMD ["./telegram-bot"]
