# Build stage
FROM golang:1.24-alpine AS builder

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

# Runtime stage
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S bioverse && adduser -S bioverse -G bioverse

WORKDIR /app

COPY --from=builder /app/bin/bioverse-api /app/bioverse-api
COPY --from=builder /app/bin/bioverse-worker /app/bioverse-worker
COPY --from=builder /app/db/migrations /app/db/migrations

USER bioverse

EXPOSE 8080

CMD ["/app/bioverse-api"]
