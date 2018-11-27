
ORG=hawtio
NAMESPACE ?= hawtio
PROJECT=operator
TAG=latest

.PHONY: setup
setup:
	@echo Installing dep
	curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
	@echo setup complete run make build deploy to build and deploy the operator to a local cluster

.PHONY: build-image
build-image: compile build

.PHONY: build
build:
	operator-sdk build docker.io/${ORG}/${PROJECT}:${TAG}

.PHONY: compile
compile:
	go build -o=hawtio-operator ./cmd/manager/main.go

.PHONY: deploy
deploy:
	-kubectl create -f deploy/operator.yaml -n ${NAMESPACE}
