# --- Build Stage ---
FROM golang:1.26-bookworm AS builder
WORKDIR /app

RUN apt-get update && apt-get install -y \
    git \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-bot main.go graphing.go
RUN CGO_ENABLED=0 GOOS=linux go build -o tide-scraper scraper.go

# Clone, build, and properly INSTALL the solunar tool to system paths
RUN git clone https://github.com/kevinboone/solunar_cmdline.git /tmp/solunar_src \
    && cd /tmp/solunar_src \
    && make clean \
    && make \
    && make install 


# --- Runtime Stage ---
FROM debian:bookworm-slim
WORKDIR /app

# Added tzdata!
RUN apt-get update && apt-get install -y \
    chromium \
    fonts-liberation \
    fontconfig \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Copy operational assets
COPY --from=builder /app/template.html .
COPY --from=builder /app/tide_data.json .
COPY --from=builder /app/telegram-bot .
COPY --from=builder /app/tide-scraper .

# Copy BOTH the solunar binary AND its required text database assets from the builder
COPY --from=builder /usr/local/bin/solunar /usr/local/bin/solunar
COPY --from=builder /usr/local/share/solunar /usr/local/share/solunar

# Direct go-rod to target Chromium
ENV LAUNCHER_BIN=/usr/bin/chromium

RUN chmod +x /app/telegram-bot /app/tide-scraper /usr/local/bin/solunar

CMD ["./telegram-bot"]