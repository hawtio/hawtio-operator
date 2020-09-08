
ORG = hawtio
NAMESPACE ?= hawtio
PROJECT = operator
TAG ?= latest

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
build: go-generate
	operator-sdk build docker.io/${ORG}/${PROJECT}:${TAG}

.PHONY: compile
compile: test
	go build -o=build/_output/bin/hawtio-operator ./cmd/manager/main.go

# Generate manifests, e.g. CRDs
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=deploy/crds

.PHONY: generate-csv
generate-csv:
	operator-sdk olm-catalog gen-csv --csv-version ${TAG}

.PHONY: go-generate
go-generate:
	go generate ./...

.PHONY: verify-csv
verify-csv:
	operator-courier verify --ui_validate_io deploy/olm-catalog/hawtio-operator

.PHONY: push-csv
push-csv:
	operator-courier push deploy/olm-catalog/hawtio-operator ${QUAY_NAMESPACE} hawtio-operator ${TAG} "${QUAY_TOKEN}"

.PHONY: install
install: install-crds
	kubectl apply -f deploy/service_account.yaml -n ${NAMESPACE}
	kubectl apply -f deploy/role.yaml -n ${NAMESPACE}
	kubectl apply -f deploy/role_binding.yaml -n ${NAMESPACE}
	kubectl apply -f deploy/cluster_role.yaml
	cat deploy/cluster_role_binding.yaml | sed "s/{{NAMESPACE}}/${NAMESPACE}/g" | kubectl apply -f -

.PHONY: install-crds
install-crds:
	kubectl apply -f deploy/crds/hawtio_v1alpha1_hawtio_crd.yaml

.PHONY: run
run:
	operator-sdk up local --namespace=${NAMESPACE} --operator-flags=""

.PHONY: deploy
deploy:
	kubectl apply -f deploy/operator.yaml -n ${NAMESPACE}

.PHONY: test
test:
	CGO_ENABLED=0 go test -count=1 ./...

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
