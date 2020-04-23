FROM golang:1.13.5-alpine3.11 AS builder

RUN apk update
RUN apk add git make

WORKDIR /hawtio-operator

COPY . .

RUN make compile

FROM alpine:3.11.2

USER nobody

COPY --from=builder /hawtio-operator/build/_output/bin/hawtio-operator /usr/local/bin/hawtio-operator

COPY --from=builder /hawtio-operator/templates /templates
