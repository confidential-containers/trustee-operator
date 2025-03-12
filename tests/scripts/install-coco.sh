#!/bin/sh

kubectl label node "kind-control-plane" "node.kubernetes.io/worker="
kubectl apply -k github.com/confidential-containers/operator/config/release?ref=v0.11.0
kubectl apply -k github.com/confidential-containers/operator/config/samples/ccruntime/default?ref=v0.11.0
