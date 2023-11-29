FROM golang:1.20-alpine3.18 AS builder

ARG HAWTIO_ONLINE_VERSION=latest
ARG HAWTIO_ONLINE_IMAGE_NAME=quay.io/hawtio/online

ENV IMAGE_VERSION_FLAG="-X main.ImageVersion=${HAWTIO_ONLINE_VERSION}"
ENV IMAGE_REPOSITORY_FLAG="-X main.ImageRepository=${HAWTIO_ONLINE_IMAGE_NAME}"

ENV GOLDFLAGS="${IMAGE_VERSION_FLAG} ${IMAGE_REPOSITORY_FLAG}"

RUN apk update
RUN apk add git make

WORKDIR /hawtio-operator

COPY . .

RUN GOLDFLAGS=${GOLDFLAGS} make build

FROM alpine:3.18

USER nobody

COPY --from=builder /hawtio-operator/hawtio-operator /usr/local/bin/hawtio-operator

COPY --from=builder /hawtio-operator/config /config
