FROM cs-hub.imgo.tv/library/golang:golang1.25 AS builder

ENV GOPROXY=https://goproxy.cn,direct
ENV GO111MODULE=on

WORKDIR /data/app/

COPY . .
RUN go mod download && go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /data/app/bin/server ./cmd/server

FROM cs-hub.imgo.tv/library/centos-hb:v1

WORKDIR /data/app/

COPY --from=builder /data/app/bin/server /data/app/configs/ ./

ENV APP_ENV=prod
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -fsS http://localhost:8080/healthz || exit 1

CMD ["./server"]
