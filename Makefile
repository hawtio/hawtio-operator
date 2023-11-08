ORG = hawtio
NAMESPACE ?= hawtio
PROJECT = operator
DEFAULT_IMAGE := docker.io/${ORG}/${PROJECT}
IMAGE ?= $(DEFAULT_IMAGE)
DEFAULT_TAG := latest
TAG ?= $(DEFAULT_TAG)
VERSION ?= 0.5.0
HAWTIO_ONLINE_VERSION ?= latest
HAWTIO_ONLINE_IMAGE_NAME ?= docker.io/${ORG}/hawtio
DEBUG ?= false

CONTROLLER_GEN_VERSION := v0.6.1
KUSTOMIZE_VERSION := v3.5.4

CRD_OPTIONS ?= crd:crdVersions=v1,preserveUnknownFields=false

INSTALL_ROOT := deploy
GEN_SUFFIX := gen.yaml

#
# Allows for resources to be loaded from outside the root location of
# the kustomize config file. Ensures that resource don't need to be
# copied around the file system.
#
# See https://kubectl.docs.kubernetes.io/faq/kustomize
#
KOPTIONS := --load_restrictor LoadRestrictionsNone

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

GOFLAGS = -ldflags "$(GOLDFLAGS)" -trimpath

ifeq ($(DEBUG),true)
GOFLAGS += -gcflags="all=-N -l"
endif

.PHONY: image build compile go-generate test manifests k8s-generate install deploy bundle controller-gen kustomize setup operator app

#
# Function for editing kustomize parameters
# Takes single parameter representing the directory
# containing the kustomization to be edited
#
define set-kvars
	cd $(1) && \
	$(KUSTOMIZE) edit set namespace $(NAMESPACE) && \
	$(KUSTOMIZE) edit set image $(DEFAULT_IMAGE)=$(IMAGE):$(TAG)
endef

default: image

image:
	docker build -t ${IMAGE}:${TAG} \
	--build-arg HAWTIO_ONLINE_IMAGE_NAME=${HAWTIO_ONLINE_IMAGE_NAME} \
	--build-arg HAWTIO_ONLINE_VERSION=${HAWTIO_ONLINE_VERSION} \
	.

build: go-generate compile test

compile:
	CGO_ENABLED=0 go build $(GOFLAGS) -o hawtio-operator ./cmd/manager/main.go

# Generate Go code
go-generate:
	go generate ./...

test:
	CGO_ENABLED=0 go test -count=1 ./...

# Generate manifests, e.g. CRDs
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=$(INSTALL_ROOT)/crd

# Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
k8s-generate: controller-gen
	$(CONTROLLER_GEN) paths="./..." object

generate: k8s-generate go-generate manifests

#
# Installation of just the CRD
# Can only be executed as a cluster-admin
#
install:
	kubectl apply -f $(INSTALL_ROOT)/crd/hawtio_v1alpha1_hawtio_crd.yaml

#
# Full deploy of all resources.
# Can only be executed as a cluster-admin
#
deploy: install kustomize
	$(call set-kvars,$(INSTALL_ROOT))
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT) | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)
endif

# Generate bundle manifests and metadata

bundle: kustomize
	$(KUSTOMIZE) build bundle | operator-sdk generate bundle --kustomize-dir bundle --version $(VERSION)

validate-bundle:
	operator-sdk bundle validate ./bundle --select-optional suite=operatorframework

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION) ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@$(KUSTOMIZE_VERSION) ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

#
# Cluster-Admin install step that configures cluster roles and
# installs the CRD. Grants a user the necessary privileges to
# install the operator.
#
# Setup the installation by installing crds, roles and granting
# privileges for the installing user.
#
setup: kustomize
	#@ Must be invoked by a user with cluster-admin privileges
	$(call set-kvars,$(INSTALL_ROOT)/setup)
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/setup | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/setup
endif

#
# Install just the operator as a normal user
# (must be granted the privileges by the Cluster-Admin
# executed `setup` procedure)
#
operator: kustomize
	#@ Can be invoked by a user with namespace privileges (rather than a cluster-admin)
	$(call set-kvars,$(INSTALL_ROOT)/operator)
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/operator | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/operator
endif

#
# Install the app CR and deploy the operator as a normal user
# (must be granted the privileges by the Cluster-Admin
# executed `setup` procedure)
#
app: operator kustomize
	#@ Can be invoked by a user with namespace privileges (rather than a cluster-admin)
	$(call set-kvars,$(INSTALL_ROOT)/app)
ifeq ($(DEBUG), false)
		$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/app | kubectl apply -f -
else
		$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/app
endif
