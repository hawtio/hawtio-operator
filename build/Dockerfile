FROM alpine:3.8

RUN apk upgrade --update --no-cache

USER nobody

COPY build/_output/bin/hawtio-operator /usr/local/bin/hawtio-operator
