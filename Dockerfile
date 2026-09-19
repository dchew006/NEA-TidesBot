# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

# Install git and C compiler (gcc) to clone and build the solunar CLI tool
RUN apt-get update && apt-get install -y build-essential git && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Clone the solunar source code directly from GitHub and compile it for Linux
RUN git clone https://github.com/kevinboone/solunar_cmdline.git /tmp/solunar_src && \
    cd /tmp/solunar_src && \
    gcc -O2 -DVERSION=\"1.0\" -o solunar *.c -lm

# Build the Go bot (pure Go native, embedded fonts)
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot .

# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Install only essential certificates and timezone data (No Chromium or GUI libraries needed)
RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Set Timezone to Singapore (Crucial for time.Now() logic in scraper/graphing)
ENV TZ=Asia/Singapore
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Copy compiled Go binary and data cache
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/tide_data.json .

# Copy the compiled Linux solunar binary from the builder stage: ./solunar/solunar
COPY --from=builder /tmp/solunar_src/solunar ./solunar/solunar
RUN chmod +x ./solunar/solunar

CMD ["./telegram-bot"]