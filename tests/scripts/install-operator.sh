#!/bin/bash

ALL_IN_ONE="${ALL_IN_ONE:-true}"

if [[ "$ALL_IN_ONE" == "true" && $# -ne 2 ]]
  then
    echo "Usage: install-operator.sh <KBS_IMAGE> <CLIENT_IMAGE>"
    exit 1
fi

if [[ "$ALL_IN_ONE" != "true" && $# -ne 4 ]]
  then
    echo "Usage: install-operator.sh <KBS_IMAGE> <CLIENT_IMAGE> < AS_IMAGE> <RVPS_IMAGE>"
    exit 1
fi

# cd to project root dir
parent_path=$( cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P )
pushd $parent_path/../..

# install the trustee-operator 
KBS_IMAGE_NAME=$1
CLIENT_IMAGE_NAME=$2
AS_IMAGE_NAME=$3
RVPS_IMAGE_NAME=$4

REGISTRY=localhost:5001
export IMG=${REGISTRY}/trustee-operator:test

pushd config/manager
kustomize edit add patch --patch "- op: replace
  path: '/spec/template/spec/containers/0/env/1'
  value:
    name: KBS_IMAGE_NAME
    value: ${KBS_IMAGE_NAME}" --kind Deployment --name controller-manager

kustomize edit add patch --patch "- op: replace
  path: '/spec/template/spec/containers/0/image'
  value: localhost:5001/trustee-operator:test" --kind Deployment --name controller-manager

if [[ "$ALL_IN_ONE" != "true" ]] ; then
  kustomize edit add patch --patch "- op: replace
  path: '/spec/template/spec/containers/0/env/2'
  value:
    name: AS_IMAGE_NAME
    value: ${AS_IMAGE_NAME}" --kind Deployment --name controller-manager

  kustomize edit add patch --patch "- op: replace
  path: '/spec/template/spec/containers/0/env/3'
  value:
    name: RVPS_IMAGE_NAME
    value: ${RVPS_IMAGE_NAME}" --kind Deployment --name controller-manager
fi
popd

make docker-build docker-push
make build-installer
kubectl apply -f dist/install.yaml

popd
