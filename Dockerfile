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

# Copy source files
COPY . .

# 1. Build the main Telegram bot binary
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot main.go graphing.go

# 2. Build the scraper utility
RUN CGO_ENABLED=0 GOOS=linux go build -o tide-scraper scraper.go

# 3. Clone, compile, and INSTALL the external C solunar tool globally
RUN git clone https://github.com/kevinboone/solunar_cmdline.git /tmp/solunar_src \
    && cd /tmp/solunar_src \
    && make clean \
    && make \
    && make install

# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Prevent interactive configuration prompts from hanging the build and use --no-install-recommends to speed things up
RUN DEBIAN_FRONTEND=noninteractive apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    chromium \
    fonts-liberation \
    fontconfig \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Copy app-specific operational assets from builder
COPY --from=builder /app/template.html .
COPY --from=builder /app/tide_data.json . 
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/tide-scraper .

# Copy BOTH the compiled solunar binary and its structural text assets from standard locations
COPY --from=builder /usr/local/bin/solunar /usr/local/bin/solunar
COPY --from=builder /usr/local/share/solunar /usr/local/share/solunar

# Direct go-rod to target the system-installed Chromium binary
ENV LAUNCHER_BIN=/usr/bin/chromium

# Ensure all application binaries and external utilities have execution permissions
RUN chmod +x /app/telegram-bot /app/tide-scraper /usr/local/bin/solunar

CMD ["./telegram-bot"]