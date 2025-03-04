# Trustee operator state management

When deploying Trustee in multiple instances for High Availability (HA), the operator makes sure that every Trustee replica gets exactly the same configuration and keys, by using native Kubernetes objects like ConfigMaps and Secrets.
This mechanism works if there is no direct interaction with the Trustee API (e.g., set-resource, set-policy, rvps-registration, etc)

There is a problem though when dealing with the client sessions, because Trustee keeps them in memory. To overcome any potential issue related to session management, each client should communicate with the same server instance while sending authentication, attestation and get-resource.
The adopted solution is to use Kubernetes [sticky sessions](https://github.com/kubernetes/ingress-nginx/blob/main/docs/examples/affinity/cookie/README.md).


## Hands-on instructions (KIND)

The following instructions provide an example on how to enable sticky sessions in a KIND cluster with NGINX Ingress. Of course, different changes are required for other ingress (or gateway) implementations.

### Install trustee-operator

```
git clone https://github.com/confidential-containers/trustee-operator.git
cd trustee-operator
./tests/scripts/kind-with-registry.sh
./tests/scripts/install-operator.sh quay.io/confidential-containers/trustee:v0.11.0 quay.io/confidential-containers/kbs-client:v0.11.0
```

### Configure trustee-operator

```
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
EOF
```

### Installing Cloud Provider KIND
```
go install sigs.k8s.io/cloud-provider-kind@latest
```

Then run `cloud-provider-kind` in a separate shell.

### Deploy ingress controller
```
kubectl apply -f https://kind.sigs.k8s.io/examples/ingress/deploy-ingress-nginx.yaml
```

### Create Ingress with session affinity/cookies
```
kubectl apply -f - << EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example-ingress
  namespace: trustee-operator-system
  annotations:
    nginx.ingress.kubernetes.io/affinity: "cookie"
    nginx.ingress.kubernetes.io/session-cookie-name: "route"
    nginx.ingress.kubernetes.io/session-cookie-expires: "172800"
    nginx.ingress.kubernetes.io/session-cookie-max-age: "172800"
spec:
  rules:
  - http:
      paths:
      - pathType: Prefix
        path: /
        backend:
          service:
            name: kbs-service
            port:
              number: 8080
EOF
```

Kubernetes/ingress documentation:

- [Sticky sessions](https://github.com/kubernetes/ingress-nginx/blob/main/docs/examples/affinity/cookie/README.md)

- [Annotations](https://github.com/kubernetes/ingress-nginx/blob/main/docs/user-guide/nginx-configuration/annotations.md#session-affinity)



### Testing
First create a client pod:

```
kubectl apply -f - << EOF
apiVersion: v1
kind: Pod
metadata:
  name: kbs-client
  namespace: trustee-operator-system
spec:
  containers:
  - name: kbs-client
    image: quay.io/confidential-containers/kbs-client:v0.11.0
    imagePullPolicy: IfNotPresent
    command:
      - sleep
      - "360000"
    env:
      - name: RUST_LOG
        value:  none
EOF
```

Get the Ingress IP address:

```
LOADBALANCER_IP=$(kubectl get services \
   --namespace ingress-nginx \
   ingress-nginx-controller \
   --output jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

Then run the following command while looking at the 2 trustee logs:

```
for i in $(seq 1 4); do kubectl exec -n trustee-operator-system kbs-client -- kbs-client --url http://$LOADBALANCER_IP/ get-resource --path default/kbsres1/key1; done
```

You should see that both trustee server instances are serving the requests.
Note: the sequence auth/attest/get-resource is sent to the same trustee







