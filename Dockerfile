# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

# Install git and C compilation build tools (gcc, make)
RUN apt-get update && apt-get install -y \
    git \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

# Copy source files (Invalidates cache if graphing.go changes)
COPY . .

# 1. Build the main Telegram bot binary
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot main.go graphing.go

# 2. Build the scraper utility
RUN CGO_ENABLED=0 GOOS=linux go build -o tide-scraper scraper.go

# 3. Clone and build the external C solunar CLI tool from source
RUN git clone https://github.com/kevinboone/solunar_cmdline.git /tmp/solunar_src \
    && cd /tmp/solunar_src \
    && make clean \
    && make \
    && mkdir -p /app/compiled_bin \
    && cp solunar /app/compiled_bin/solunar


# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Install Chromium and required linux graphics/font assets
RUN apt-get update && apt-get install -y \
    chromium \
    fonts-liberation \
    fontconfig \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy operational assets
COPY --from=builder /app/template.html .
COPY --from=builder /app/tide_data.json . 
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/tide-scraper .

# Copy solunar binary straight into global linux binary paths
COPY --from=builder /app/compiled_bin/solunar /usr/local/bin/solunar

# Direct go-rod to target Chromium
ENV LAUNCHER_BIN=/usr/bin/chromium

RUN chmod +x /app/telegram-bot /app/tide-scraper /usr/local/bin/solunar

CMD ["./telegram-bot"]