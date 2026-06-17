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

COPY . .

# 1. Build the main Telegram bot binary (explicitly compiling main and graphing together)
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot main.go graphing.go

# 2. Build the scraper utility as its own standalone production binary
RUN CGO_ENABLED=0 GOOS=linux go build -o tide-scraper scraper.go

# 3. Clone and build the external C solunar CLI tool from source
RUN git clone https://github.com/kevinboone/solunar_cmdline.git /tmp/solunar_src \
    && cd /tmp/solunar_src \
    && make clean \
    && make \
    && mkdir -p /app/bin \
    && cp solunar /app/bin/solunar


# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Install Chromium and required linux graphics/font assets for headless go-rod screenshots
RUN apt-get update && apt-get install -y \
    chromium \
    fonts-liberation \
    fontconfig \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy your operational assets verbatim
COPY --from=builder /app/template.html .
COPY --from=builder /app/tide_data.json . 

# Copy BOTH of your compiled Go application binaries
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/tide-scraper .

# Copy the compiled C solunar binary straight into the global system PATH
COPY --from=builder /app/bin/solunar /usr/local/bin/solunar

# Direct go-rod to target the cloud instance's Chromium layout 
ENV LAUNCHER_BIN=/usr/bin/chromium

CMD ["./telegram-bot"]