FROM golang:1.27-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go generate
RUN CGO_ENABLED=0 GOOS=linux go build -o tgweb main.go

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    ffmpeg \
    curl \
    wget \
    unzip \
    libstdc++6 \
    libssl3 \
    zlib1g \
    && rm -rf /var/lib/apt/lists/*

RUN wget -O /usr/local/bin/yt-dlp \
    https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download/yt-dlp_linux \
    && chmod +x /usr/local/bin/yt-dlp

RUN curl -fsSL https://deno.land/install.sh | sh \
    && ln -sf /root/.deno/bin/deno /usr/local/bin/deno

ENV DENO_INSTALL="/root/.deno" \
    PATH="/root/.deno/bin:${PATH}" \
    PORT="8080" \
    LD_LIBRARY_PATH="/app:/usr/local/lib"

COPY --from=builder /app/tgweb .
COPY --from=builder /app/libtdjson.so* ./
RUN if [ -f libtdjson.so.1.8.69 ] && [ ! -e libtdjson.so ]; then ln -s libtdjson.so.1.8.69 libtdjson.so; fi

EXPOSE 8080

CMD ["./tgweb"]
