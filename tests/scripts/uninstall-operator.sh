#!/bin/bash

# delete CR KbsConfig (if any remain; may already be gone via owner-reference cascade)
kubectl delete kbsconfig --all -n trustee-operator-system --ignore-not-found

# cd to project root dir
parent_path=$( cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P )
pushd $parent_path/../..
kubectl delete -f dist/install.yaml
popd
