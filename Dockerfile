# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot .

# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Install Chromium and system dependencies for go-rod
RUN apt-get update && apt-get install -y \
    chromium \
    fonts-liberation \
    fontconfig \
    ca-certificates \
    tzdata \
    # Extra libs often required by headless Chromium in Debian slim to prevent go-rod crashes
    libnss3 \
    libxss1 \
    libasound2 \
    libatk-bridge2.0-0 \
    libgtk-3-0 \
    libgbm1 \
    && rm -rf /var/lib/apt/lists/*

ENV TZ=Asia/Singapore
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Copy compiled binary and necessary files verbatim
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/template.html .
COPY --from=builder /app/tide_data.json . 

COPY --from=builder /app/solunar ./solunar
RUN chmod +x ./solunar/solunar

# Set environment variable pointing to the standard Chromium location
ENV LAUNCHER_BIN=/usr/bin/chromium

CMD ["./telegram-bot"]
