#!/bin/bash

SECRET="$(kubectl exec -it -n trustee-operator-system kbs-client -- kbs-client --url http://kbs-service:8080 get-resource --path default/kbsres1/key1)"
retVal=$?
if [ $retVal -ne 0 ]; then
    echo "Error when retrieving the secret"
else
    echo "Attestation completed successfully: secret="$SECRET
    # this secret is created only to check the secret retrieval has been successful
    kubectl create secret generic trustee-secret --from-literal key1=$SECRET -n trustee-operator-system
fi
