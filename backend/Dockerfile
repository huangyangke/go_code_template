FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/server ./cmd/server

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata curl

WORKDIR /app
COPY --from=builder /app/bin/server .
COPY configs/ configs/

ENV APP_ENV=prod
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -fsS http://localhost:8080/healthz || exit 1

CMD ["./server"]
