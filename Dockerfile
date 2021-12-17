FROM golang:alpine as builder

WORKDIR /app

COPY . ./
# static linking
ENV CGO_ENABLED=0
RUN go mod download

RUN go build -trimpath -ldflags "-s -w -buildid=" -o server

FROM busybox:latest

WORKDIR /app

COPY --from=builder /app/server /app/server
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["./server"]
