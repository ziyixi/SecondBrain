# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /frontal_lobe ./cmd/server

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -u 1001 appuser

COPY --from=builder /frontal_lobe /usr/local/bin/frontal_lobe

USER appuser

EXPOSE 50052

ENTRYPOINT ["frontal_lobe"]
