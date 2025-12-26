FROM debian:bookworm-slim AS builder

RUN apt-get update && apt-get install -y curl git ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY bin/ bin/
COPY go.mod go.sum ./

# Bootstrap Hermit and download Go
RUN ./bin/hermit install
RUN ./bin/go mod download

COPY . .
RUN ./bin/go build -o wandiweather ./cmd/wandiweather

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates tzdata sqlite3 && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/wandiweather .

EXPOSE 8080

CMD ["./wandiweather", "--db", "/data/wandiweather.db", "--port", "8080"]
