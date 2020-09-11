ORG = hawtio
NAMESPACE ?= hawtio
PROJECT = operator
TAG ?= latest
VERSION ?= 0.3.0

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

default: build-image

.PHONY: build-image
build-image: compile build

.PHONY: build
build: go-generate k8s-generate
	operator-sdk build docker.io/${ORG}/${PROJECT}:${TAG}

compile: test
	go build -o=build/_output/bin/hawtio-operator ./cmd/manager/main.go

# Generate Go code
go-generate:
	go generate ./...

test:
	CGO_ENABLED=0 go test -count=1 ./..

# Generate manifests, e.g. CRDs
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=deploy/crd

# Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
k8s-generate: controller-gen
	$(CONTROLLER_GEN) paths="./..." object

install:
	kubectl apply -f deploy/crd/hawtio_v1alpha1_hawtio_crd.yaml

deploy: install kustomize
	cd deploy && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
	$(KUSTOMIZE) build deploy | kubectl apply -f -

# Generate bundle manifests and metadata
.PHONY: bundle
bundle: kustomize
	$(KUSTOMIZE) build bundle | operator-sdk generate bundle --kustomize-dir bundle --version $(VERSION)
	#operator-sdk bundle validate ./bundle

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
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
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.5.4 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif
