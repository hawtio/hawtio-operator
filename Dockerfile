FROM golang:1.24.6-alpine3.22 AS builder

ARG HAWTIO_ONLINE_VERSION=latest
ARG HAWTIO_ONLINE_IMAGE_NAME=quay.io/hawtio/online
ARG HAWTIO_ONLINE_GATEWAY_VERSION=latest
ARG HAWTIO_ONLINE_GATEWAY_IMAGE_NAME=quay.io/hawtio/online-gateway

ENV IMAGE_VERSION_FLAG="-X main.ImageVersion=${HAWTIO_ONLINE_VERSION}"
ENV IMAGE_REPOSITORY_FLAG="-X main.ImageRepository=${HAWTIO_ONLINE_IMAGE_NAME}"
ENV GATEWAY_IMAGE_VERSION_FLAG="-X main.GatewayImageVersion=${HAWTIO_ONLINE_GATEWAY_VERSION}"
ENV GATEWAY_IMAGE_REPOSITORY_FLAG="-X main.GatewayImageRepository=${HAWTIO_ONLINE_GATEWAY_IMAGE_NAME}"

ENV GOLDFLAGS="${IMAGE_VERSION_FLAG} ${IMAGE_REPOSITORY_FLAG} ${GATEWAY_IMAGE_VERSION_FLAG} ${GATEWAY_IMAGE_REPOSITORY_FLAG}"

RUN apk update
RUN apk add git make

WORKDIR /hawtio-operator

COPY . .

RUN GOLDFLAGS=${GOLDFLAGS} CI_BUILD=true make build

FROM alpine:3.22

USER nobody

COPY --from=builder /hawtio-operator/hawtio-operator /usr/local/bin/hawtio-operator

COPY --from=builder /hawtio-operator/config /config
