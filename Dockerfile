# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/bin/bioverse-api ./cmd/api

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /app/bin/bioverse-worker ./cmd/worker

RUN CGO_ENABLED=0 go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

# Runtime stage
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S bioverse && adduser -S bioverse -G bioverse

WORKDIR /app

COPY --from=builder /app/bin/bioverse-api /app/bioverse-api
COPY --from=builder /app/bin/bioverse-worker /app/bioverse-worker
COPY --from=builder /go/bin/goose /app/goose
COPY --from=builder /app/db/migrations /app/db/migrations
COPY --from=builder /app/deploy/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh /app/goose /app/bioverse-api /app/bioverse-worker

USER bioverse

EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/bioverse-api"]
