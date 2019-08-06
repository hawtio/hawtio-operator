FROM golang:1.12.7-alpine3.10 AS builder

RUN apk update
RUN apk add make

WORKDIR /go/src/github.com/hawtio/hawtio-operator

COPY . .

RUN make compile

FROM alpine:3.8

USER nobody

COPY --from=builder /go/src/github.com/hawtio/hawtio-operator/hawtio-operator /usr/local/bin/hawtio-operator

COPY --from=builder /go/src/github.com/hawtio/hawtio-operator/templates /templates
