
ORG = hawtio
NAMESPACE ?= hawtio
PROJECT = operator
TAG = latest

.PHONY: build-image
build-image: compile build

.PHONY: build
build:
	operator-sdk build docker.io/${ORG}/${PROJECT}:${TAG}

.PHONY: compile
compile:
	go build -o=hawtio-operator ./cmd/manager/main.go

.PHONY: generate-csv
generate-csv:
    operator-sdk olm-catalog gen-csv --csv-version ${TAG}

.PHONY: verify-csv
verify-csv:
    operator-courier verify --ui_validate_io deploy/olm-catalog/hawtio-operator

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
