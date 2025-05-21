#!/bin/bash

# delete CR KbsConfig
export CR_NAME=$(kubectl get kbsconfig -n trustee-operator-system -o=jsonpath='{.items[0].metadata.name}')
kubectl delete KbsConfig -n trustee-operator-system $CR_NAME

# cd to project root dir
parent_path=$( cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P )
pushd $parent_path/../..
kubectl delete -f dist/install.yaml
popd
