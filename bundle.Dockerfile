FROM scratch

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=trustee-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.36.1
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v4

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY bundle/manifests /manifests/
COPY bundle/metadata /metadata/
COPY bundle/tests/scorecard /tests/scorecard/

# Red Hat labels.

ARG NAME=trustee-operator-bundle
ARG DESCRIPTION="The Trustee operator bundle."

LABEL com.redhat.component=$NAME
LABEL description=$DESCRIPTION
LABEL io.k8s.description=$DESCRIPTION
LABEL io.k8s.display-name=$NAME
LABEL name="build-of-trustee/trustee-operator-bundle"
LABEL summary=$DESCRIPTION
LABEL distribution-scope=public
LABEL release="1"
LABEL url="https://access.redhat.com/"
LABEL vendor="Red Hat, Inc."
LABEL version="1"
LABEL maintainer="Red Hat"
LABEL cpe="cpe:/a:redhat:confidential_compute_attestation:1.0::el9"

# Licenses

COPY LICENSE /licenses/LICENSE
