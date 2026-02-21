# Multi-stage build: build then minimal runtime
# Build context is repository root; build from there so go.mod is available.

# Stage 1: build
FROM golang:1.24-alpine AS builder
WORKDIR /app
# Copy go mod files first for layer caching (context is repo root)
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# Stage 2: minimal runtime
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
ENV PORT=8080
COPY --from=builder /app/server .
# Do not use variable substitution in CMD; app reads PORT from env
CMD ["./server"]
