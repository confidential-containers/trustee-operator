# KBS kvstorage `dir_path` configuration

KBS v0.20+ uses the unified `storage_backend` with the resource plugin
`storage_backend_type = "kvstorage"`. The KV namespace for secrets is
`repository`, so files are stored under:

```
{dir_path}/repository/{repo}/{type}/{tag}
```

## Correct configuration

Set **only the base path** in `[storage_backend.backends.local_fs]`:

```toml
[storage_backend]
storage_type = "LocalFs"

[storage_backend.backends.local_fs]
dir_path = "/opt/confidential-containers/storage"

[[plugins]]
name = "resource"
storage_backend_type = "kvstorage"
```

A secret at KBS path `default/nvcr-credentials/nvcr-auth.json` is stored at:

```
/opt/confidential-containers/storage/repository/default/nvcr-credentials/nvcr-auth.json
```

This matches the trustee-operator `secret-converter` init container layout.

## Common misconfiguration

Do **not** set `dir_path` to `.../storage/repository` when using `kvstorage`.
That double-nests paths to `.../storage/repository/repository/...` and breaks
NVCR credential release. Guest image pull then fails with CDH errors such as
`ttrpc request error` or `Initialize resource provider failed` even when
attestation succeeds.

The legacy `LocalFs` plugin form (`type = "LocalFs"` with `dir_path = ".../repository"`)
is deprecated for new deployments; use `storage_backend` + `kvstorage` instead.

## Post-deploy verification

After applying or updating KBS configuration:

```bash
# Confirm dir_path in the running KBS container
kubectl exec -n trustee-operator-system deploy/trustee-deployment -c kbs -- \
  grep dir_path /etc/kbs-config/kbs-config.toml

# During a confidential pod start, KBS logs should show:
# POST /kbs/v0/attest 200
# GET .../default/nvcr-credentials/nvcr-auth.json 200
kubectl logs -n trustee-operator-system deploy/trustee-deployment -c kbs --tail=50
```

See also [KBS storage documentation](https://github.com/confidential-containers/trustee/blob/main/kbs/docs/storage.md).
