# Integration tests

## Continuous integration

These [tests](../tests/e2e) are executed as part of the CI when submitting a pull request.
The test suite brings up an ephemeral kind cluster and performs the attestation using the sample-attester.

### Run the test suite manually

Prerequisites:

- [kuttl](https://kuttl.dev/docs/cli.html#setup-the-kuttl-kubectl-plugin) plugin installed
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) installed

Optional: set the env variables KBS_IMAGE_NAME and CLIENT_IMAGE_NAME to override the default trustee/client images

  ```sh
  KBS_IMAGE_NAME=<trustee-image> CLIENT_IMAGE_NAME=<client-image> make test-e2e
  ```

## Kata tests

These [tests](../tests/tee) are supposed to be run against an existing cluster where both coco and trustee operators are deployed.

The test suite comprises 3 tests:
- attestation with secret retrieval
- signed images
- encrypted images

For example:

```sh
KATA_RUNTIME=kata-tdx kubectl kuttl test --parallel 1 ./tests/tee
```

### Run the kata test suite in kind

Sometimes it is convenient to run the kata test suite in a kind ephemeral cluster.

```sh
./tests/scripts/kind-with-registry.sh
./tests/scripts/install-operator.sh quay.io/confidential-containers/trustee:v0.11.0 quay.io/confidential-containers/kbs-client:v0.11.0
./tests/scripts/config-trustee.sh
./tests/scripts/install-coco.sh
```

Wait for all the components to be up:

```sh
kubectl get runtimeclass -A
NAME                 HANDLER              AGE
kata                 kata-qemu            15s
kata-clh             kata-clh             15s
kata-qemu            kata-qemu            15s
kata-qemu-coco-dev   kata-qemu-coco-dev   15s
kata-qemu-sev        kata-qemu-sev        15s
kata-qemu-snp        kata-qemu-snp        15s
kata-qemu-tdx        kata-qemu-tdx        15s
```

Run the tests:

```sh
KATA_RUNTIME=kata-qemu-coco-dev kubectl kuttl test --parallel 1 ./tests/tee
```
