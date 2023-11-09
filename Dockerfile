FROM golang:1.20-alpine3.18 AS builder

RUN apk update
RUN apk add git make

WORKDIR /hawtio-operator

COPY . .

RUN make build

FROM alpine:3.18

USER nobody

COPY --from=builder /hawtio-operator/hawtio-operator /usr/local/bin/hawtio-operator

COPY --from=builder /hawtio-operator/config /config
