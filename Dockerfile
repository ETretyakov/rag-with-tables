# go-duckdb bundles DuckDB statically and requires CGO + a C/C++ compiler.
# We use the Debian-based Go image for the build stage (gcc pre-installed)
# and a lean Debian image for the final stage (glibc present, no GCC).

FROM golang:1.24-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags="-s -w" -o bin/server ./cmd/server


FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/bin/server .

EXPOSE 8080 8081

ENTRYPOINT ["./server"]
