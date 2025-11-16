# Build Go binaries
FROM golang:1.24.4-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .

# Build all three binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/irc_collector ./cmd/irc_collector
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/oauth_server  ./cmd/oauth_server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/kafka_consumer ./cmd/kafka_consumer


# Runtime image
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && update-ca-certificates

WORKDIR /app

# Copy built binaries from builder stage
COPY --from=builder /app/bin/irc_collector /app/irc_collector
COPY --from=builder /app/bin/oauth_server  /app/oauth_server
COPY --from=builder /app/bin/kafka_consumer /app/kafka_consumer

# mount config/token paths as volumes.
# create dirs so the paths exist.
RUN mkdir -p /app/accounts /app/tokens /app/internal/channel_record

# Default command: do nothing by default.
CMD ["/bin/sh", "-c", "echo 'Set command in docker-compose.yml (irc_collector / oauth_server / kafka_consumer)' && sleep 3600"]
