#!/bin/bash

KBS_IMAGE_NAME="${KBS_IMAGE_NAME:-quay.io/confidential-containers/trustee:290fd0eb64ab20f50efbd27cf80542851c0ee17f}"
REGISTRY=localhost:5001
export IMG=${REGISTRY}/trustee-operator:test

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

popd
