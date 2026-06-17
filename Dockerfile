# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

# Install git inside the builder so 'go install' can fetch the external repo
RUN apt-get update && apt-get install -y git && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 1. Build your main bot application
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot .

# 2. Compile the solunar tool globally (defaults to /go/bin/solunar inside the builder)
RUN CGO_ENABLED=0 GOOS=linux go install https://github.com/kevinboone/solunar_cmdline.git

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

# 3. Copy the compiled solunar binary into the system PATH of the runtime image
COPY --from=builder /go/bin/solunar /usr/local/bin/solunar

# Set environment variable pointing to the standard Chromium location
ENV LAUNCHER_BIN=/usr/bin/chromium

CMD ["./telegram-bot"]