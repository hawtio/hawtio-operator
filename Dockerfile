FROM golang:1.12.7-alpine3.10 AS builder

RUN apk update
RUN apk add git make mercurial

WORKDIR /hawtio-operator

COPY . .

RUN go mod download
RUN make compile

FROM alpine:3.8

USER nobody

COPY --from=builder /hawtio-operator/hawtio-operator /usr/local/bin/hawtio-operator

COPY --from=builder /hawtio-operator/templates /templates
