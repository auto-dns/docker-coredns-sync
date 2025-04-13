# ===== Stage 1: Builder =====
FROM golang:1.24 AS builder
WORKDIR /app

# Copy go.mod and go.sum first to leverage caching for dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code.
COPY . .

# Build the binary with optimizations (statically linked and small).
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o docker-coredns-sync ./cmd/docker-coredns-sync


# ===== Stage 2: Development Environment =====
FROM golang:1.24 AS dev
WORKDIR /workspace
RUN groupadd --gid 1000 vscode && \
    useradd --uid 1000 --gid vscode -m vscode
RUN mkdir -p /home/vscode/.shell_history && chown -R vscode:vscode /home/vscode/.shell_history
ENV GOPATH=/home/vscode/go
ENV GOCACHE=/home/vscode/.cache/go-build
RUN apt-get update && apt-get install -y \
    bash \
    curl \
    gcc \
    git \
    make \
    musl-dev \
    procps \
    pv \
    sudo \
    vim \
    && rm -rf /var/lib/apt/lists/*
RUN go install golang.org/x/tools/gopls@latest && \
    go install github.com/go-delve/delve/cmd/dlv@latest && \
    go install honnef.co/go/tools/cmd/staticcheck@latest
USER vscode
CMD ["sleep", "infinity"]


# ===== Stage 3: Release (Runtime) =====
FROM alpine:3.21 AS release
RUN apk add --no-cache ca-certificates
WORKDIR /app

# Copy the statically built binary from the builder stage.
COPY --from=builder /app/docker-coredns-sync .

# Expose the application port.
EXPOSE 5678

# Entrypoint that starts your application.
ENTRYPOINT ["/app/docker-coredns-sync"]