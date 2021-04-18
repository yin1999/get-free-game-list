FROM golang:latest as builder

WORKDIR /app

COPY . ./
RUN go mod download

RUN go build -ldflags "-s -w" -v -o server

FROM debian:buster-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/server /app/server

CMD ["/app/server"]
