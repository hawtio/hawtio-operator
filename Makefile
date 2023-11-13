ORG = hawtio
NAMESPACE ?= hawtio
PROJECT = operator
DEFAULT_IMAGE := docker.io/hawtio/operator
IMAGE ?= $(DEFAULT_IMAGE)
DEFAULT_TAG := latest
TAG ?= $(DEFAULT_TAG)
VERSION ?= 1.0.0
DEBUG ?= false
LAST_RELEASED_IMAGE_NAME := hawtio-operator
LAST_RELEASED_VERSION ?= 0.5.0

#
# Versions of tools and binaries
#
CONTROLLER_GEN_VERSION := v0.6.1
KUSTOMIZE_VERSION := v4.5.4
OPERATOR_SDK_VERSION := v1.28.0

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
	docker build -t docker.io/${ORG}/${PROJECT}:${TAG} .

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

DEFAULT_CHANNEL ?= $(shell echo "stable-v$(word 1,$(subst ., ,$(lastword $(VERSION))))")
CHANNELS ?= $(DEFAULT_CHANNEL),latest
PACKAGE := hawtio-operator
MANIFESTS := bundle
CSV_VERSION := $(VERSION)
CSV_NAME := $(PACKAGE).v$(CSV_VERSION)
CSV_DISPLAY_NAME := Hawtio Operator
CSV_FILENAME := $(PACKAGE).clusterserviceversion.yaml
CSV_PATH := $(MANIFESTS)/bases/$(CSV_FILENAME)
CSV_REPLACES := $(LAST_RELEASED_IMAGE_NAME).v$(LAST_RELEASED_VERSION)
IMAGE_NAME ?= docker.io/hawtio/operator

# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
ifneq ($(origin PACKAGE), undefined)
BUNDLE_PACKAGE := --package=$(PACKAGE)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL) $(BUNDLE_PACKAGE)

#
# Tailor the manifest according to default values for this project
# Note. to successfully make the bundle the name must match that specified in the PROJECT file
#
pre-bundle:
	# bundle name must match that which appears in PROJECT file
	@sed -i 's/projectName: .*/projectName: $(PACKAGE)/' PROJECT
	@sed -i 's~^    containerImage: .*~    containerImage: $(IMAGE):$(VERSION)~' $(CSV_PATH)
	@sed -i 's/^  name: .*.\(v.*\)/  name: $(CSV_NAME)/' $(CSV_PATH)
	@sed -i 's/^  displayName: .*/  displayName: $(CSV_DISPLAY_NAME)/' $(CSV_PATH)
	@sed -i 's/^  version: .*/  version: $(CSV_VERSION)/' $(CSV_PATH)
	@if grep -q replaces $(CSV_PATH); \
		then sed -i 's/^  replaces: .*/  replaces: $(CSV_REPLACES)/' $(CSV_PATH); \
		else sed -i '/  version: ${CSV_VERSION}/a \ \ replaces: $(CSV_REPLACES)' $(CSV_PATH); \
	fi

bundle: kustomize operator-sdk pre-bundle
	@# Display BUNDLE_METADATA_OPTS for debugging
	$(info BUNDLE_METADATA_OPTS=$(BUNDLE_METADATA_OPTS))
	@# Sets the operator image to the preferred image:tag
	@cd bundle && $(KUSTOMIZE) edit set image $(IMAGE_NAME)=$(IMAGE):$(VERSION)
	@# Build kustomize manifests
	$(KUSTOMIZE) build bundle | $(OPERATOR_SDK) generate bundle \
		--kustomize-dir bundle \
		--version $(VERSION) -q --overwrite \
		$(BUNDLE_METADATA_OPTS)

validate-bundle: operator-sdk
	$(OPERATOR_SDK) bundle validate ./bundle --select-optional suite=operatorframework

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell command -v controller-gen 2> /dev/null))
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell command -v controller-gen 2> /dev/null)
endif

kustomize:
ifeq (, $(shell command -v kustomize 2> /dev/null))
	go install sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION)
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell command -v kustomize 2> /dev/null)
endif

detect-os:
ifeq '$(findstring ;,$(PATH))' ';'
OS := Windows
OS_LOWER := windows
else
OS := $(shell echo $$(uname 2>/dev/null) || echo Unknown)
OS := $(patsubst CYGWIN%,Cygwin,$(OS))
OS := $(patsubst MSYS%,MSYS,$(OS))
OS := $(patsubst MINGW%,MSYS,$(OS))
OS_LOWER := $(shell echo $(OS) | tr '[:upper:]' '[:lower:]')
endif

operator-sdk: detect-os
	@echo "####### Installing operator-sdk version $(OPERATOR_SDK_VERSION)..."
	@curl \
		-s -L https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(OS_LOWER)_amd64 \
		-o operator-sdk ; \
		chmod +x operator-sdk ;\
		mkdir -p $(GOBIN) ;\
		mv operator-sdk $(GOBIN)/ ;
OPERATOR_SDK=$(GOBIN)/operator-sdk

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
