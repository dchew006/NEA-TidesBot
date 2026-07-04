# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

# Install C compiler (gcc) and make to build the solunar CLI tool from source
RUN apt-get update && apt-get install -y build-essential && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile the solunar C library specifically for Linux
RUN cd solunar && make

# Build the Go bot (the '.' automatically includes main.go, scraper.go, and graphing.go)
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot .

# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Install Chromium, timezone data, and extra dependencies for go-rod
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

# Set Timezone to Singapore (Crucial for time.Now() logic in scraper/graphing)
ENV TZ=Asia/Singapore
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Copy compiled Go binary and necessary files
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/template.html .
COPY --from=builder /app/tide_data.json . 

# Copy the newly compiled Linux solunar binary from the builder stage
COPY --from=builder /app/solunar/solunar ./solunar/solunar
RUN chmod +x ./solunar/solunar

# Set environment variable pointing to the standard Chromium location
ENV LAUNCHER_BIN=/usr/bin/chromium

CMD ["./telegram-bot"]