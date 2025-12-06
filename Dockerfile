# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 \
    go build -ldflags="-w -s" -o tempestwx-utilities main.go

# Final stage
FROM cgr.dev/chainguard/static:latest

# Copy binary
COPY --from=builder /app/tempestwx-utilities /tempestwx-utilities

# Use non-root user
USER 65532:65532

ENTRYPOINT ["/tempestwx-utilities"]
