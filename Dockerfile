#
# Global ARGs required by both images
#
ARG RED_HAT_REGISTRY=registry.redhat.io
ARG HAWTIO_NAMESPACE="rhbac-4"
ARG HAWTIO_ONLINE_IMAGE_NAME=${RED_HAT_REGISTRY}/${HAWTIO_NAMESPACE}/hawtio-rhel9
ARG HAWTIO_ONLINE_GATEWAY_IMAGE_NAME=${RED_HAT_REGISTRY}/${HAWTIO_NAMESPACE}/hawtio-gateway-rhel9
ARG HAWTIO_ONLINE_IMAGE_LABEL_NAME=${HAWTIO_NAMESPACE}/hawtio-rhel9
ARG HAWTIO_OPERATOR_IMAGE_LABEL_NAME=${HAWTIO_NAMESPACE}/hawtio-rhel9-operator
ARG HAWTIO_OPERATOR_VERSION=2.1.0
ARG HAWTIO_ONLINE_VERSION=2.5.0
ARG HAWTIO_ONLINE_GATEWAY_VERSION=2.5.0

####################################################################

FROM registry.redhat.io/rhel9/go-toolset:1.26 AS builder

USER root
WORKDIR /hawtio-operator

#
# Redeclaration of args used in this image
#
ARG HAWTIO_ONLINE_VERSION
ARG HAWTIO_ONLINE_IMAGE_NAME
ARG HAWTIO_ONLINE_GATEWAY_VERSION
ARG HAWTIO_ONLINE_GATEWAY_IMAGE_NAME
ARG HAWTIO_OPERATOR_VERSION

ENV IMAGE_VERSION_FLAG="-X main.ImageVersion=${HAWTIO_ONLINE_VERSION}"
ENV IMAGE_REPOSITORY_FLAG="-X main.ImageRepository=${HAWTIO_ONLINE_IMAGE_NAME}"
ENV GATEWAY_IMAGE_VERSION_FLAG="-X main.GatewayImageVersion=${HAWTIO_ONLINE_GATEWAY_VERSION}"
ENV GATEWAY_IMAGE_REPOSITORY_FLAG="-X main.GatewayImageRepository=${HAWTIO_ONLINE_GATEWAY_IMAGE_NAME}"
ENV LEGACY_CERT_VERSION_FLAG="-X 'main.LegacyServingCertificateMountVersion=< 1.5.0'"
ENV ADD_LABELS_FLAG="-X main.AdditionalLabels=com.company=Red_Hat,rht.prod_name=Red_Hat_HawtIO_Operator,rht.prod_ver=${HAWTIO_OPERATOR_VERSION},rht.comp=hawtio-operator-container,rht.comp_ver=${HAWTIO_OPERATOR_VERSION}"
ENV HAWTIO_OPERATOR_VERSION_FLAG="-X main.OperatorVersion=${HAWTIO_OPERATOR_VERSION}"

ENV GOLDFLAGS="${IMAGE_VERSION_FLAG} ${IMAGE_REPOSITORY_FLAG} ${GATEWAY_IMAGE_VERSION_FLAG} ${GATEWAY_IMAGE_REPOSITORY_FLAG} ${LEGACY_CERT_VERSION_FLAG} ${ADD_LABELS_FLAG} ${HAWTIO_OPERATOR_VERSION_FLAG}"

#
# Install tools for building
#
RUN dnf --disableplugin=subscription-manager install -y make && dnf clean all

#
# Copy the source to the builder image
#
COPY . .

#
# Build hawtio operator
#
RUN GOLDFLAGS=${GOLDFLAGS} CI_BUILD=true make build

# ----------------------------------------------------------------------------

#
# Final Image
#
FROM registry.access.redhat.com/ubi9-minimal:9.8

ARG HAWTIO_OPERATOR_VERSION
ARG HAWTIO_OPERATOR_IMAGE_LABEL_NAME

#
# Location of hawtio-operator source repository
# copied into builder image
#
ARG HAWTIO_OPERATOR_SRC=/hawtio-operator

LABEL name="${HAWTIO_OPERATOR_IMAGE_LABEL_NAME}" \
      version="${HAWTIO_OPERATOR_VERSION}" \
      maintainer="Paul Richardson <parichar@redhat.com>" \
      summary="Kubernetes operator that installs the Red Hat build of HawtIO." \
      description="Kubernetes operator that installs the Red Hat build of HawtIO." \
      com.redhat.component="hawtio-operator-container" \
      io.k8s.display-name="Operator for the Red Hat build of HawtIO" \
      io.openshift.tags="hawtio,operator"

#
# Perform an upgrade of all packages to ensure
# any outstanding packages are updates from the
# base images
#
RUN microdnf upgrade -y

USER 998

COPY --from=builder ${HAWTIO_OPERATOR_SRC}/hawtio-operator /usr/local/bin/hawtio-operator

#
# Copies the hawtio-online production config (branding)
#
COPY config/config.yaml /config/config.yaml
