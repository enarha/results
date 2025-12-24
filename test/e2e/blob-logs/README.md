# Blob Logs Setup for Tekton Results

This directory contains configuration files for setting up blob storage for Tekton logs.

## Files

### Local MinIO Deployment

- **minio-local.yaml**: Kubernetes manifests for deploying a local MinIO instance
  - Creates `minio` namespace
  - Deploys MinIO server with persistent storage
  - Creates the `tekton-logs` bucket automatically
  - Credentials: `minioadmin` / `minioadmin`

- **vector-minio-local-config.yaml**: Tekton Results API configuration for local MinIO
  - Configures `LOGGING_PLUGIN_API_URL` with local MinIO endpoint
  - Sets proper S3 path-style access parameters

### Vector Configuration

- **vector-s3.yaml**: Helm values for Vector DaemonSet
  - Configures Vector to collect logs from Tekton pods
  - Forwards logs to S3-compatible storage (MinIO)
  - Organizes logs by namespace, pipeline run, task run, and container

- **vector-minio-config.yaml**: Legacy configuration (use vector-minio-local-config.yaml instead)

### Testing

- **minio-logs-test.md**: Comprehensive guide for testing and troubleshooting log storage

## Quick Start

### 1. Deploy Local MinIO

```bash
kubectl apply -f minio-local.yaml
kubectl wait --for=condition=available --timeout=120s deployment/minio -n minio
kubectl wait --for=condition=complete --timeout=120s job/minio-create-bucket -n minio
```

### 2. Install Vector

```bash
helm repo add vector https://helm.vector.dev
helm repo update
helm upgrade --install vector vector/vector --namespace logging --create-namespace --values vector-s3.yaml
```

### 3. Configure Tekton Results API

```bash
kubectl apply -f vector-minio-local-config.yaml
kubectl rollout restart deployment/tekton-results-api -n tekton-pipelines
kubectl rollout restart deployment/tekton-results-watcher -n tekton-pipelines
```

### 4. Create RBAC for Vector

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vector-logging
rules:
- apiGroups: [""]
  resources: ["namespaces", "nodes", "pods"]
  verbs: ["list", "watch", "get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vector-logging
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vector-logging
subjects:
- kind: ServiceAccount
  name: vector
  namespace: logging
EOF
```

### 5. Configure S3 Credentials for Tekton Results API

```bash
kubectl create secret generic tekton-results-s3 -n tekton-pipelines \
  --from-literal=AWS_ACCESS_KEY_ID=minioadmin \
  --from-literal=AWS_SECRET_ACCESS_KEY=minioadmin \
  --from-literal=AWS_REGION=us-east-1 \
  --dry-run=client -o yaml | kubectl apply -f -
```

Then patch the Tekton Results API deployment to use these credentials:

```bash
kubectl patch deployment tekton-results-api -n tekton-pipelines --type=json -p='[
  {
    "op": "add",
    "path": "/spec/template/spec/containers/0/env/-",
    "value": {
      "name": "AWS_ACCESS_KEY_ID",
      "valueFrom": {
        "secretKeyRef": {
          "name": "tekton-results-s3",
          "key": "AWS_ACCESS_KEY_ID"
        }
      }
    }
  },
  {
    "op": "add",
    "path": "/spec/template/spec/containers/0/env/-",
    "value": {
      "name": "AWS_SECRET_ACCESS_KEY",
      "valueFrom": {
        "secretKeyRef": {
          "name": "tekton-results-s3",
          "key": "AWS_SECRET_ACCESS_KEY"
        }
      }
    }
  },
  {
    "op": "add",
    "path": "/spec/template/spec/containers/0/env/-",
    "value": {
      "name": "AWS_REGION",
      "valueFrom": {
        "secretKeyRef": {
          "name": "tekton-results-s3",
          "key": "AWS_REGION"
        }
      }
    }
  },
  {
    "op": "add",
    "path": "/spec/template/spec/containers/0/env/-",
    "value": {
      "name": "AWS_S3_FORCE_PATH_STYLE",
      "value": "true"
    }
  },
  {
    "op": "add",
    "path": "/spec/template/spec/containers/0/env/-",
    "value": {
      "name": "AWS_S3_ENDPOINT",
      "value": "http://minio.minio.svc.cluster.local:9000"
    }
  }
]'
```

## Automated Setup

Use the `02-logs-setup.sh` script to automate the entire setup:

```bash
cd /path/to/results
./test/e2e/02-logs-setup.sh
```

## Verification

See [minio-logs-test.md](minio-logs-test.md) for detailed testing instructions.

Quick check:

```bash
# Run a test PipelineRun
kubectl create -f - <<EOF
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  generateName: test-logs-
  namespace: default
spec:
  pipelineSpec:
    tasks:
    - name: hello
      taskSpec:
        steps:
        - name: echo
          image: alpine
          script: echo "Hello from Tekton!"
EOF

# Wait for completion and check logs in MinIO
kubectl run -n minio minio-check --rm -i --restart=Never --image=alpine --command -- sh -c "
apk add --no-cache curl > /dev/null 2>&1
curl -sS -o /tmp/mc https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x /tmp/mc
/tmp/mc alias set local http://minio.minio.svc.cluster.local:9000 minioadmin minioadmin > /dev/null 2>&1
/tmp/mc ls local/tekton-logs/logs/ --recursive
"
```

## Architecture

```
┌─────────────────┐
│  Tekton Pods    │
│  (TaskRuns)     │
└────────┬────────┘
         │ logs
         ▼
┌─────────────────┐
│  Vector         │
│  (DaemonSet)    │
└────────┬────────┘
         │ S3 API
         ▼
┌─────────────────┐
│  MinIO          │
│  (Local)        │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Tekton Results  │
│ API             │
└─────────────────┘
```

## Troubleshooting

See [minio-logs-test.md](minio-logs-test.md) for detailed troubleshooting steps.

Common issues:

1. **Vector not collecting logs**: Check RBAC permissions
2. **301 redirect errors**: Ensure endpoint is configured in LOGGING_PLUGIN_API_URL
3. **Authentication errors**: Verify S3 credentials secret and environment variables
4. **Empty logs**: Wait for Vector to batch and upload (may take 10-30 seconds)

