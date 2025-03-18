#!/bin/sh

openssl genpkey -algorithm ed25519 > privateKey
openssl pkey -in privateKey -pubout -out publicKey
kubectl create secret generic kbs-auth-public-key --from-file=publicKey -n trustee-operator-system

kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: kbs-config
  namespace: trustee-operator-system
data:
  kbs-config.toml: |
    [http_server]
    sockets = ["0.0.0.0:8080"]
    insecure_http = true

    [admin]
    insecure_api = true
    auth_public_key = "/etc/auth-secret/publicKey"

    [attestation_token]
    insecure_key = true
    attestation_token_type = "CoCo"

    [attestation_service]
    type = "coco_as_builtin"
    work_dir = "/opt/confidential-containers/attestation-service"
    policy_engine = "opa"

      [attestation_service.attestation_token_broker]
      type = "Ear"
      policy_dir = "/opt/confidential-containers/attestation-service/policies"

      [attestation_service.attestation_token_config]
      duration_min = 5

      [attestation_service.rvps_config]
      type = "BuiltIn"
      
        [attestation_service.rvps_config.storage]
        type = "LocalJson"
        file_path = "/opt/confidential-containers/rvps/reference-values/reference-values.json"

    [[plugins]]
    name = "resource"
    type = "LocalFs"
    dir_path = "/opt/confidential-containers/kbs/repository"

    [policy_engine]
    policy_path = "/opt/confidential-containers/opa/policy.rego"
EOF

kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: rvps-reference-values
  namespace: trustee-operator-system
data:
  reference-values.json: |
    [
    ]
EOF

kubectl create secret generic kbsres1 --from-literal key1=res1val1 --from-literal key2=res1val2 -n trustee-operator-system

kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: resource-policy
  namespace: trustee-operator-system
data:
  policy.rego:
    package policy
    default allow = true
EOF

kubectl apply -f - << EOF
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:
  labels:
    app.kubernetes.io/name: kbsconfig
    app.kubernetes.io/instance: kbsconfig-sample
    app.kubernetes.io/part-of: kbs-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: kbs-operator
  name: kbsconfig-sample
  namespace: trustee-operator-system
spec:
  kbsConfigMapName: kbs-config
  kbsAuthSecretName: kbs-auth-public-key
  kbsDeploymentType: AllInOneDeployment
  kbsRvpsRefValuesConfigMapName: rvps-reference-values
  kbsSecretResources: ["kbsres1"]
  kbsResourcePolicyConfigMapName: resource-policy
  kbsServiceType: LoadBalancer
EOF

