# Rebuild trigger: 1.15.4 release 2026-01-19
ARG GO_BUILDER=registry.access.redhat.com/ubi9/go-toolset:1.25
ARG RUNTIME=registry.access.redhat.com/ubi9/ubi-minimal:latest

FROM $GO_BUILDER AS builder

WORKDIR /go/src/github.com/tektoncd/results
COPY upstream .
COPY .konflux/patches patches/
RUN set -e; for f in patches/*.patch; do echo ${f}; [[ -f ${f} ]] || continue; git apply ${f}; done
COPY head HEAD
ENV GODEBUG="http2server=0"
ENV GOEXPERIMENT=strictfipsruntime
RUN go build -ldflags="-X 'knative.dev/pkg/changeset.rev=$(cat HEAD)'" -mod=vendor -tags disable_gcp -tags strictfipsruntime -v -o /tmp/results-api \
    ./cmd/api

FROM $RUNTIME
ARG VERSION=1.15

ENV API=/usr/local/bin/results-api \
    KO_APP=/ko-app \
    KO_DATA_PATH=/kodata

COPY --from=builder /tmp/results-api ${API}
COPY --from=builder /tmp/results-api ${KO_APP}/api
COPY head ${KO_DATA_PATH}/HEAD

LABEL \
    com.redhat.component="openshift-pipelines-results-api-rhel9-container" \
    cpe="cpe:/a:redhat:openshift_pipelines:1.15::el9" \
    description="Red Hat OpenShift Pipelines tektoncd-results api" \
    io.k8s.description="Red Hat OpenShift Pipelines tektoncd-results api" \
    io.k8s.display-name="Red Hat OpenShift Pipelines tektoncd-results api" \
    io.openshift.tags="tekton,openshift,tektoncd-results,api" \
    maintainer="pipelines-extcomm@redhat.com" \
    name="openshift-pipelines/pipelines-results-api-rhel9" \
    summary="Red Hat OpenShift Pipelines tektoncd-results api" \
    version="v1.15.5"

RUN groupadd -r -g 65532 nonroot && useradd --no-log-init -r -u 65532 -g nonroot nonroot
USER 65532

ENTRYPOINT ["/usr/local/bin/results-api"]# trigger rebuild 2026-02-14
