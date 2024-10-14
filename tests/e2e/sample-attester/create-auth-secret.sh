#!/bin/bash

openssl genpkey -algorithm ed25519 > privateKey
openssl pkey -in privateKey -pubout -out publicKey
kubectl create secret generic kbs-auth-public-key --from-file=publicKey -n trustee-operator-system