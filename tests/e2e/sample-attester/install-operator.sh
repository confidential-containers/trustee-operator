#!/bin/bash

KBS_IMAGE_NAME=$1
CLIENT_IMAGE_NAME=$2
REGISTRY=localhost:5001
export IMG=${REGISTRY}/trustee-operator:test

# project root dir
pushd ../../..

pushd config/manager
kustomize edit set image controller=$IMG
kustomize edit add patch --patch "- op: replace
  path: '/spec/template/spec/containers/0/env/1'
  value:
    name: KBS_IMAGE_NAME
    value: ${KBS_IMAGE_NAME}" --kind Deployment --name controller-manager
popd
make docker-build docker-push
make build-installer
kubectl apply -f dist/install.yaml

pushd tests/e2e/sample-attester
kustomize edit set image quay.io/confidential-containers/kbs-client=$CLIENT_IMAGE_NAME
popd