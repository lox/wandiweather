FROM debian:bookworm-slim AS builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends bash curl git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

ENV MISE_DATA_DIR="/mise"
ENV MISE_CONFIG_DIR="/mise"
ENV MISE_CACHE_DIR="/mise/cache"
ENV MISE_INSTALL_PATH="/usr/local/bin/mise"
ENV MISE_VERSION="v2026.2.11"
ENV PATH="/mise/shims:${PATH}"

WORKDIR /app

# Copy dependency files first for better caching
COPY mise.toml ./
COPY go.mod go.sum ./

# Install mise, install the pinned Go toolchain, and download Go dependencies.
RUN curl https://mise.run | sh && \
    mise trust -a && \
    mise install go@1.25.5 && \
    mise exec go@1.25.5 -- go mod download

# Copy source and build
COPY . .
RUN mise exec go@1.25.5 -- go build -o wandiweather ./cmd/wandiweather

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/wandiweather .

EXPOSE 8080

CMD ["./wandiweather", "--db", "/data/wandiweather.db", "--port", "8080"]
