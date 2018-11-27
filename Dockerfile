FROM golang:1.11.2-alpine3.8 AS builder

RUN apk update
RUN apk add make

WORKDIR /go/src/github.com/hawtio/hawtio-operator

COPY . .

RUN make compile

FROM alpine:3.8

USER nobody

COPY --from=builder /go/src/github.com/hawtio/hawtio-operator/build/_output/bin/hawtio-operator /usr/local/bin/hawtio-operator
