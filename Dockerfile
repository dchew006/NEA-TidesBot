# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

# Install git and compilation tools
RUN apt-get update && apt-get install -y \
    git \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

# Copy application files
COPY . .

# 1. Build the main Telegram bot binary
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot main.go graphing.go

# 2. Build the scraper utility as its own standalone production binary
RUN CGO_ENABLED=0 GOOS=linux go build -o tide-scraper scraper.go

# 3. Install the standard Go-based solunar CLI that outputs "Peak times : HH:MM" 
# This matches the regex parser expectations inside graphing.go
RUN GOBIN=/app/bin go install github.com/ScreamingHawk/solunar@latest


# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Install Chromium, fonts, and CRITICAL tzdata for timezone alignment
RUN apt-get update && apt-get install -y \
    chromium \
    fonts-liberation \
    fontconfig \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Copy operational assets
COPY --from=builder /app/template.html .
# Note: If tide_data.json is missing initially, ensure your main.go successfully creates it
# COPY --from=builder /app/tide_data.json . 

# Copy compiled Go application binaries
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/tide-scraper .

# Copy the compiled solunar binary straight into global system PATH
COPY --from=builder /app/bin/solunar /usr/local/bin/solunar

# Adjust Go-Rod configuration
# Note: Your main.go uses launcher.New(), but look at your ENV flag. 
# Go-Rod looks for ROD_BIN, not LAUNCHER_BIN!
ENV ROD_BIN=/usr/bin/chromium

CMD ["./telegram-bot"]