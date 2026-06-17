# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

# 1. Install git AND C build tools (gcc, make, libc-dev) so we can compile solunar
RUN apt-get update && apt-get install -y \
    git \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 2. Build your main Go bot application
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot main.go graphing.go

# 3. Clone and build solunar from source using standard Linux make
RUN git clone https://github.com/kevinboone/solunar_cmdline.git /tmp/solunar_src \
    && cd /tmp/solunar_src \
    && make clean \
    && make \
    && mkdir -p /app/bin \
    && cp solunar /app/bin/solunar

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

# 4. Copy the compiled C solunar binary into the system PATH of the runtime image
COPY --from=builder /app/bin/solunar /usr/local/bin/solunar

# Set environment variable pointing to the standard Chromium location
ENV LAUNCHER_BIN=/usr/bin/chromium

CMD ["./telegram-bot"]