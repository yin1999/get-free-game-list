FROM golang:latest as builder

WORKDIR /app

COPY . ./
RUN go mod download

RUN go build -ldflags "-s -w" -v -o server

FROM debian:bullseye-slim

WORKDIR /app

COPY --from=builder /app/server /app/server
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["./server"]
