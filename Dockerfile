FROM golang:1.21.1-alpine AS builder
RUN apk add build-base git openssh-client openssl-dev librdkafka-dev librdkafka pkgconf

RUN mkdir /app

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download -x
COPY . .
RUN go build -tags musl -o /app/torrent-api cmd/main.go

FROM alpine:3.19
LABEL MAINTAINER="xochilpili <xochilpili@gmail.com>"
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/torrent-api .

CMD ["./torrent-api"]