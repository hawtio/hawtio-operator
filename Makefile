ORG = hawtio
NAMESPACE ?= hawtio
PROJECT = operator
DEFAULT_IMAGE := quay.io/${ORG}/${PROJECT}
IMAGE ?= $(DEFAULT_IMAGE)
VERSION ?= 2.0.0
HAWTIO_ONLINE_VERSION ?= 2.4.0
HAWTIO_ONLINE_IMAGE_NAME ?= quay.io/${ORG}/online
HAWTIO_ONLINE_GATEWAY_VERSION ?= 2.4.0
HAWTIO_ONLINE_GATEWAY_IMAGE_NAME ?= quay.io/${ORG}/online-gateway
DEBUG ?= false
LAST_RELEASED_IMAGE_NAME := red-hat-hawtio-operator
LAST_RELEASED_VERSION ?= 1.3.0
BUNDLE_IMAGE_NAME ?= $(IMAGE)-bundle
FORCE_TOOL_UPDATE ?= false

# Is this build part of an automated CI pipeline
CI_BUILD ?= false

# If CI_BUILD is set to true then only want fast testing
# so skip integration tests marked with the integration tag.
ifeq ($(CI_BUILD),true)
TEST_FLAGS :=
TEST_ENV_VARS :=
else
TEST_FLAGS := -tags=integration
TEST_ENV_VARS := GINKGO_ARGS="-ginkgo.v"
endif

# Drop suffix for use with bundle and CSV
OPERATOR_VERSION := $(subst -SNAPSHOT,,$(VERSION))

#
# Versions of tools and binaries
#
CONTROLLER_GEN_VERSION := v0.20.1
KUSTOMIZE_VERSION := v5.8.1
OPERATOR_SDK_VERSION := v1.42.2
OPM_VERSION := v1.65.0
YQ_VERSION := v4.53.2

CRD_OPTIONS ?= crd:crdVersions=v1

INSTALL_ROOT := deploy
GEN_SUFFIX := gen.yaml

#
# Allows for resources to be loaded from outside the root location of
# the kustomize config file. Ensures that resource don't need to be
# copied around the file system.
#
# See https://kubectl.docs.kubernetes.io/faq/kustomize
#
KOPTIONS := --load-restrictor LoadRestrictionsNone

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

.PHONY: image publish-image build compile go-generate test manifests k8s-generate install deploy bundle controller-gen kubectl kustomize check-admin setup operator app

#
# Function for editing kustomize parameters
# Takes single parameter representing the directory
# containing the kustomization to be edited
#
define set-kvars
	cd $(1) && \
	$(KUSTOMIZE) edit set namespace $(NAMESPACE) && \
	$(KUSTOMIZE) edit set image $(DEFAULT_IMAGE)=$(IMAGE):$(VERSION)
endef

container-builder:
ifeq (, $(shell command -v podman 2> /dev/null))
ifeq (, $(shell command -v docker 2> /dev/null))
	$(error "No podman or docker found in PATH. Please install and re-run")
else
CONTAINER_BUILDER=$(shell command -v docker 2> /dev/null)
endif
else
CONTAINER_BUILDER=$(shell command -v podman 2> /dev/null)
endif

#---
#
#@ image
#
#== Compile the operator as a docker image
#
#* PARAMETERS:
#** IMAGE:                            Set a custom image for the container image
#** VERSION:                          Set a custom version for the container image tag
#** HAWTIO_ONLINE_IMAGE_NAME:         Set the operator's target hawtio-online image name
#** HAWTIO_ONLINE_GATEWAY_IMAGE_NAME: Set the operator's target hawtio-online-gateway image name
#** HAWTIO_ONLINE_VERSION:            Set the operator's target hawtio-online image version
#** HAWTIO_ONLINE_GATEWAY_VERSION:    Set the operator's target hawtio-online-gateway image version
#
#---
image: container-builder
	$(CONTAINER_BUILDER) build -t $(IMAGE):$(VERSION) \
	--build-arg HAWTIO_ONLINE_IMAGE_NAME=$(HAWTIO_ONLINE_IMAGE_NAME) \
	--build-arg HAWTIO_ONLINE_GATEWAY_IMAGE_NAME=$(HAWTIO_ONLINE_GATEWAY_IMAGE_NAME) \
	--build-arg HAWTIO_ONLINE_VERSION=$(HAWTIO_ONLINE_VERSION) \
	--build-arg HAWTIO_ONLINE_GATEWAY_VERSION=$(HAWTIO_ONLINE_GATEWAY_VERSION) \
	--build-arg HAWTIO_OPERATOR_VERSION=$(VERSION) \
	.

#---
#
#@ publish-image
#
#== Compile the operator as a docker image then push the image to the repository
#
#* PARAMETERS:
#** IMAGE:                            Set a custom image for the container image
#** VERSION:                          Set a custom version for the container image tag
#** HAWTIO_ONLINE_IMAGE_NAME:         Set the operator's target hawtio-online image name
#** HAWTIO_ONLINE_GATEWAY_IMAGE_NAME: Set the operator's target hawtio-online-gateway image name
#** HAWTIO_ONLINE_VERSION:            Set the operator's target hawtio-online image version
#** HAWTIO_ONLINE_GATEWAY_VERSION:    Set the operator's target hawtio-online-gateway image version
#
#---
publish-image: image
	$(CONTAINER_BUILDER) push $(IMAGE):$(VERSION)

#---
#
#@ build
#== Build and test the operator binary
#
#* PARAMETERS:
#** GOLDFLAGS: Add any go-lang ldflags, eg. -X main.ImageVersion=2.0.0-202312061128 will compile in the operand version
#
#---
build: generate compile test

compile:
	CGO_ENABLED=0 go build $(GOFLAGS) -o hawtio-operator ./cmd/manager/main.go

# Generate Go code
go-generate:
	go generate ./...

# Only use gotestfmt if building / testing locally
test:
ifeq ($(CI_BUILD), false)
# If FORCE_TOOL_UPDATE is true, it intentionally
# returns an empty string to force the install.
# Otherwise, it returns the tool's existing path.
ifeq (, $(if $(filter true,$(FORCE_TOOL_UPDATE)),,$(shell command -v gotestfmt 2> /dev/null)))
	go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@latest
endif
	CGO_ENABLED=0 $(TEST_ENV_VARS) go test $(TEST_FLAGS) -count=1 -json ./... 2>&1 | gotestfmt
else
	CGO_ENABLED=0 $(TEST_ENV_VARS) go test $(TEST_FLAGS) -v -count=1 ./...
endif

# Only instigate re-generation of manifests in non-production builds
manifests: controller-gen
ifeq ($(CI_BUILD), false)
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=$(INSTALL_ROOT)/crd
endif

# Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
k8s-generate: controller-gen
ifeq ($(CI_BUILD), false)
	$(CONTROLLER_GEN) paths="./..." object
endif

generate: k8s-generate go-generate manifests

get-image:
	@echo $(IMAGE)

get-version:
	@echo $(VERSION)

#---
#
#@ deploy-crd
#
#== Deploys only the CRD
#
#=== Can only be executed as a cluster-admin
#
#* PARAMETERS:
#** DEBUG: Print the resources to be applied instead of applying them [true|false]
#
#---
deploy-crd: kubectl
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/crd | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/crd
endif

#---
#
#@ deploy
#
#== Deploy all the resources of the operator to the current cluster
#
#=== Can only be executed as a cluster-admin
#
#* PARAMETERS:
#** IMAGE:     Set a custom image for the deployment
#** VERSION:   Set a custom version for the deployment
#** NAMESPACE: Set the namespace for the resources
#** DEBUG:     Print the resources to be applied instead of applying them [true|false]
#
#---
deploy: kubectl kustomize install
	$(call set-kvars,$(INSTALL_ROOT))
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT) | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)
endif

# Generate bundle manifests and metadata
DEFAULT_CHANNEL ?= $(shell echo "v$(word 1,$(subst ., ,$(lastword $(OPERATOR_VERSION))))")
CHANNELS ?= $(DEFAULT_CHANNEL),latest
PACKAGE := red-hat-hawtio-operator
MANIFESTS := bundle
CSV_VERSION ?= $(OPERATOR_VERSION)
CSV_NAME := $(PACKAGE).v$(CSV_VERSION)
CSV_DISPLAY_NAME := Hawtio Operator
CSV_FILENAME := $(PACKAGE).clusterserviceversion.yaml
CSV_PATH := $(MANIFESTS)/bases/$(CSV_FILENAME)
# Not required for first version to be deployed to Operator Hub
CSV_REPLACES := $(LAST_RELEASED_IMAGE_NAME).v$(LAST_RELEASED_VERSION)
# Ensure all the 1.x releases should upgrade to 2
CSV_SKIP_RANGE := >=1.0.0 <2.0.0
IMAGE_NAME ?= $(DEFAULT_IMAGE)

# Test Bundle Index
BUNDLE_INDEX ?= registry.redhat.io/redhat/redhat-operator-index:v4.21
INDEX_DIR := index
OPM := opm

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
# If there is a replaces version then insert/update it
	@if grep -q replaces $(CSV_PATH); \
		then sed -i "s/^  replaces: .*/  replaces: $(CSV_REPLACES)/" $(CSV_PATH); \
		else sed -i "/  version: ${CSV_VERSION}/a \ \ replaces: $(CSV_REPLACES)" $(CSV_PATH); \
	fi
# If there is a skipRange version range then insert/update it
ifneq ($(CSV_SKIP_RANGE), "")
	@if grep -q olm.skipRange $(CSV_PATH); \
		then sed -i "s/olm.skipRange: .*/olm.skipRange: '$(CSV_SKIP_RANGE)'/" $(CSV_PATH); \
		else sed -i "/  annotations:/a \ \ \ \ olm.skipRange: '$(CSV_SKIP_RANGE)'" $(CSV_PATH); \
	fi
endif


#---
#
#@ bundle
#
#== Create the manifest bundle artifacts
#
#* PARAMETERS:
#** IMAGE:     Set a custom image for the deployment
#** VERSION:   Set a custom version for the deployment
#** NAMESPACE: Set the namespace for the resources
#** DEBUG:     Print the resources to be applied instead of applying them [true|false]
#
#---
bundle: kustomize operator-sdk pre-bundle
	@# Display BUNDLE_METADATA_OPTS for debugging
	$(info BUNDLE_METADATA_OPTS=$(BUNDLE_METADATA_OPTS))
	@# Sets the operator image to the preferred image:tag
	@cd bundle && $(KUSTOMIZE) edit set image $(IMAGE_NAME)=$(IMAGE):$(VERSION)
	@# Build kustomize manifests
	$(KUSTOMIZE) build bundle | $(OPERATOR_SDK) generate bundle \
		--kustomize-dir bundle \
		--version $(OPERATOR_VERSION) -q --overwrite \
		$(BUNDLE_METADATA_OPTS)

#---
#
#@ validate-bundle
#
#== Validate the manifest bundle artifacts generated in bundle directory
#
#---
validate-bundle: operator-sdk
	$(OPERATOR_SDK) bundle validate ./bundle --select-optional suite=operatorframework

#---
#
#@  bundle-build
#
#== Build the bundle image.
#
#* PARAMETERS:
#** IMAGE:   Set the custom image name (suffixed with '-bundle')
#** VERSION: Set the custom version for the bundle image
#
#---
bundle-build: bundle container-builder
	$(CONTAINER_BUILDER) build -f bundle.Dockerfile -t $(BUNDLE_IMAGE_NAME):$(VERSION) .

#---
#
#@ bundle-index
#
#== Builds a test catalog index for installing the operator via an OLM
#
#* PARAMETERS:
#** IMAGE:       Set the custom image name (will be suffixed with '-bundle')
#** VERSION:     Set the custom version for the bundle image
#** CSV_VERSION: Set the CSV version if different from the OPERATOR_VERSION / TAG
#
#---
bundle-index: opm yq container-builder
	BUNDLE_INDEX=$(BUNDLE_INDEX) INDEX_DIR=$(INDEX_DIR) PACKAGE=$(PACKAGE) YQ=$(YQ) \
	OPM=$(OPM) BUNDLE_IMAGE=$(BUNDLE_IMAGE_NAME):$(VERSION) CSV_NAME=$(CSV_NAME) \
	CSV_SKIPS="$(CSV_SKIP_RANGE)" CSV_REPLACES=$(CSV_REPLACES) CHANNELS="$(CHANNELS)" \
	CONTAINER_BUILDER=$(CONTAINER_BUILDER) ./script/build_bundle_index.sh

#
# Checks if the cluster user has the necessary privileges to be a cluster-admin
# In this case if the user can list the CRDs then probably a cluster-admin
#
check-admin: kubectl
	@output=$$(kubectl get crd 2>&1) || (echo "****" && echo "**** ERROR: Cannot continue as user is not a Cluster-Admin ****" && echo "****"; exit 1)

# find or download controller-gen
# download controller-gen if necessary
#
# Only install in non-production builds
#
controller-gen:
ifeq ($(CI_BUILD), false)
# If FORCE_TOOL_UPDATE is true, it intentionally
# returns an empty string to force the install.
# Otherwise, it returns the tool's existing path.
ifeq (, $(if $(filter true,$(FORCE_TOOL_UPDATE)),,$(shell command -v controller-gen 2> /dev/null)))
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell command -v controller-gen 2> /dev/null)
endif
else
CONTROLLER_GEN=controller-gen-not-used-in-ci-build
endif

kubectl:
ifeq (, $(shell command -v kubectl 2> /dev/null))
	$(error "No kubectl found in PATH. Please install and re-run")
endif

kustomize:
# If FORCE_TOOL_UPDATE is true, it intentionally returns an empty string to force the install.
# Otherwise, it returns the tool's existing path.
ifeq (, $(if $(filter true,$(FORCE_TOOL_UPDATE)),,$(shell command -v kustomize 2> /dev/null)))
	go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)
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

opm: detect-os
# If FORCE_TOOL_UPDATE is true, it intentionally returns an empty string to force the install.
# Otherwise, it returns the tool's existing path.
ifeq (, $(if $(filter true,$(FORCE_TOOL_UPDATE)),,$(shell command -v opm 2> /dev/null)))
	@{ \
	set -e ;\
	curl \
		-L https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$(OS_LOWER)-amd64-opm \
		-o opm; \
	chmod +x opm;\
	echo "Expected Version: $(OPM_VERSION)" ;\
	echo "Downloaded Version: $$(./opm version | sed -n 's/.*"\(v[^"]*\)".*/\1/p')" ;\
	if [ "$(OPM_VERSION)" != "$$(./opm version | sed -n 's/.*"\(v[^"]*\)".*/\1/p')" ]; then echo "opm install failure" && exit 1; fi ;\
	mv opm $(GOBIN)/ ;\
	mkdir -p $(GOBIN) ;\
	}
OPM=$(GOBIN)/opm
else
	@{ \
	echo "opm $$(opm version | sed -n 's/.*"\(v[^"]*\)".*/\1/p') installed"; \
	if [ "$(OPM_VERSION)" != "$$(opm version | sed -n 's/.*"\(v[^"]*\)".*/\1/p')" ]; then echo "opm version difference. Move opm version aside and install approved version" && exit 1; fi ;\
	}
OPM=$(shell command -v opm 2> /dev/null)
endif

yq:
# If FORCE_TOOL_UPDATE is true, it intentionally returns an empty string to force the install.
# Otherwise, it returns the tool's existing path.
ifeq (, $(if $(filter true,$(FORCE_TOOL_UPDATE)),,$(shell command -v yq 2> /dev/null)))
	{ \
	go install github.com/mikefarah/yq/v4@$(YQ_VERSION) ;\
	echo "Expected Version: $(YQ_VERSION)" ;\
	echo "Downloaded Version: $$($(GOBIN)/yq --version | sed -n 's/.*version \(v.*\)/\1/p')" ;\
	if [ "$(YQ_VERSION)" != "$$($(GOBIN)/yq --version | sed -n 's/.*version \(v.*\)/\1/p')" ]; then echo "yq install failure" && exit 1; fi ;\
	}
YQ=$(GOBIN)/yq
else
	@{ \
	echo "yq $$(yq --version | sed -n 's/.*version \(v.*\)/\1/p') installed"; \
	if [ "$(YQ_VERSION)" != "$$(yq --version | sed -n 's/.*version \(v.*\)/\1/p')" ]; then echo "yq version difference. Move yq version aside and install approved version" && exit 1; fi ;\
	}
YQ=$(shell command -v yq 2> /dev/null)
endif

#---
#
#@ setup
#
#== Setup the installation by installing crds, roles and granting privileges for the installing user.
#
#=== Calls check-admin
#
#* PARAMETERS:
#** IMAGE:     Set a custom image for the deployment
#** VERSION:   Set a custom version for the deployment
#** NAMESPACE: Set the namespace for the resources
#** DEBUG:     Print the resources to be applied instead of applying them [true|false]
setup: kubectl kustomize check-admin
	#@ Must be invoked by a user with cluster-admin privileges
	$(call set-kvars,$(INSTALL_ROOT)/setup)
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/setup | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/setup
endif

#---
#
#@ operator
#
#== Install just the operator as a normal user
#
#=== (must be granted the privileges by the Cluster-Admin executed `setup` procedure)
#
#* PARAMETERS:
#** IMAGE:     Set a custom image for the deployment
#** VERSION:   Set a custom version for the deployment
#** NAMESPACE: Set the namespace for the resources
#** DEBUG:     Print the resources to be applied instead of applying them [true|false]
#
#---
operator: kubectl kustomize
	#@ Can be invoked by a user with namespace privileges (rather than a cluster-admin)
	$(call set-kvars,$(INSTALL_ROOT)/operator)
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/operator | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/operator
endif

#---
#
#@ cr
#
#== Install the app CR only
#
#* PARAMETERS:
#** NAMESPACE: Set the namespace for the resources
#** DEBUG:     Print the resources to be applied instead of applying them [true|false]
#
#---
cr: kubectl kustomize
	#@ Can be invoked by a user with namespace privileges (rather than a cluster-admin)
	$(call set-kvars,$(INSTALL_ROOT)/app)
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/app | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/app
endif

#---
#
#@ app
#
#== Install the app CR and deploy the operator as a normal user
#
#=== (must be granted the privileges by the Cluster-Admin executed `setup` procedure)
#
#* PARAMETERS:
#** IMAGE:     Set a custom image for the deployment
#** VERSION:   Set a custom version for the deployment
#** NAMESPACE: Set the namespace for the resources
#** DEBUG:     Print the resources to be applied instead of applying them [true|false]
#
#---
app: kubectl kustomize operator
	#@ Can be invoked by a user with namespace privileges (rather than a cluster-admin)
	$(call set-kvars,$(INSTALL_ROOT)/app)
ifeq ($(DEBUG), false)
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/app | kubectl apply -f -
else
	$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/app
endif

UNINSTALLS = .uninstall-app .uninstall-operator .uninstall-setup

$(UNINSTALLS): kubectl kustomize
	# Delete CR instances first while the operator is still running
	kubectl delete hawtios --all -n $(NAMESPACE) --ignore-not-found --wait=true
	@$(call set-kvars,$(INSTALL_ROOT)/$(subst .uninstall-,,$@))
ifeq ($(DEBUG), false)
	@$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/$(subst .uninstall-,,$@) | kubectl delete --ignore-not-found=true -f -
else
	@$(KUSTOMIZE) build $(KOPTIONS) $(INSTALL_ROOT)/$(subst .uninstall-,,$@) | kubectl delete --dry-run=client -f -
endif

#---
#
#@ uninstall
#
#== Uninstalls the app CR, operator and setup resources
#
#=== Calls check-admin
#
#* PARAMETERS:
#** NAMESPACE: Set the namespace for the resources
#** DEBUG:     Print the resources to be deleted instead of deleting them [true|false]
#
#---
uninstall: kubectl kustomize check-admin $(UNINSTALLS)

.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help screen.
	@./script/help.sh
