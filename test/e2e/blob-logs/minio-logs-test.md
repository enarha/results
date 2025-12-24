# Testing MinIO Logs Storage

This document describes how to verify that logs are being stored correctly in the local MinIO deployment.

## Prerequisites

- Local MinIO deployed using `minio-local.yaml`
- Vector configured to forward logs to MinIO
- Tekton Results API configured to read from MinIO

## Checking Logs in MinIO

### Method 1: Using kubectl run with mc (MinIO Client)

```bash
kubectl run -n minio minio-check --rm -i --restart=Never --image=alpine --command -- sh -c "
apk add --no-cache curl > /dev/null 2>&1
curl -sS -o /tmp/mc https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x /tmp/mc
/tmp/mc alias set local http://minio.minio.svc.cluster.local:9000 minioadmin minioadmin > /dev/null 2>&1
echo '=== Contents of tekton-logs bucket ==='
/tmp/mc ls local/tekton-logs/logs/ --recursive
"
```

### Method 2: Using a dedicated pod

Create a pod with mc installed:

```bash
kubectl run -n minio mc-client --image=quay.io/minio/mc:latest --restart=Never -- sleep 3600
```

Then exec into it:

```bash
kubectl exec -n minio mc-client -it -- sh
```

Inside the pod, configure and use mc:

```bash
mc alias set local http://minio.minio.svc.cluster.local:9000 minioadmin minioadmin
mc ls local/tekton-logs/logs/ --recursive
```

### Method 3: Port-forward to MinIO Console

Forward the MinIO console port:

```bash
kubectl port-forward -n minio svc/minio 9001:9001
```

Then open your browser to: http://localhost:9001

Login credentials:
- Username: `minioadmin`
- Password: `minioadmin`

## Expected Log Structure

Logs are stored in the following path structure:

```
logs/<namespace>/<pipelineRunUID>/<taskRunUID>/<container-name>.log
```

Example:
```
logs/foo/9beb07bb-0083-4033-8782-64a442465412/07779008-e961-4011-8755-f26ceb53325a/step-step1.log
logs/foo/9beb07bb-0083-4033-8782-64a442465412/07779008-e961-4011-8755-f26ceb53325a/prepare.log
logs/foo/9beb07bb-0083-4033-8782-64a442465412/07779008-e961-4011-8755-f26ceb53325a/place-scripts.log
```

## Viewing Log Contents

To view the contents of a specific log file:

```bash
kubectl run -n minio minio-check --rm -i --restart=Never --image=alpine --command -- sh -c "
apk add --no-cache curl > /dev/null 2>&1
curl -sS -o /tmp/mc https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x /tmp/mc
/tmp/mc alias set local http://minio.minio.svc.cluster.local:9000 minioadmin minioadmin > /dev/null 2>&1
/tmp/mc cat local/tekton-logs/logs/<namespace>/<pipelineRunUID>/<taskRunUID>/step-step1.log
"
```

Replace `<namespace>`, `<pipelineRunUID>`, and `<taskRunUID>` with actual values.

## Troubleshooting

### No logs appearing in MinIO

1. Check Vector is running and has proper RBAC permissions:
   ```bash
   kubectl get pods -n logging -l app.kubernetes.io/name=vector
   kubectl logs -n logging -l app.kubernetes.io/name=vector --tail=50
   ```

2. Verify Vector configuration:
   ```bash
   kubectl get configmap vector -n logging -o yaml | grep -A 10 "aws_s3:"
   ```

3. Check if Vector can access MinIO:
   ```bash
   kubectl logs -n logging -l app.kubernetes.io/name=vector | grep -i "error\|s3"
   ```

### Tekton Results API cannot read logs

1. Check the API configuration:
   ```bash
   kubectl get configmap tekton-results-api-config -n tekton-pipelines -o yaml | grep LOGGING_PLUGIN_API_URL
   ```

   Should show:
   ```
   LOGGING_PLUGIN_API_URL=s3://tekton-logs?endpoint=http://minio.minio.svc.cluster.local:9000&region=us-east-1
   ```

2. Check API logs:
   ```bash
   kubectl logs -n tekton-pipelines deployment/tekton-results-api --tail=50 | grep -i "error\|s3"
   ```

3. Verify S3 credentials are set:
   ```bash
   kubectl get secret tekton-results-s3 -n tekton-pipelines -o yaml
   ```

## Cleaning Up

To delete all logs from MinIO:

```bash
kubectl run -n minio minio-cleanup --rm -i --restart=Never --image=quay.io/minio/mc:latest -- sh -c "
mc alias set local http://minio.minio.svc.cluster.local:9000 minioadmin minioadmin
mc rm --recursive --force local/tekton-logs/logs/
"
```

