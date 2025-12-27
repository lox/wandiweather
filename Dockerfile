FROM debian:bookworm-slim AS builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends curl git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy dependency files first for better caching
COPY bin/ bin/
COPY go.mod go.sum ./

# Install hermit and download Go dependencies (cached unless go.mod/go.sum change)
RUN ./bin/hermit install && ./bin/go mod download

# Copy source and build
COPY . .
RUN ./bin/go build -o wandiweather ./cmd/wandiweather

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/wandiweather .

EXPOSE 8080

CMD ["./wandiweather", "--db", "/data/wandiweather.db", "--port", "8080"]
