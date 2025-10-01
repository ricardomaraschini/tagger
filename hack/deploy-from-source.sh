#!/usr/bin/env bash
set -euo pipefail

echo "Loading image into kind cluster"
kind load docker-image "$IMAGE"

if kubectl get namespace tagger >/dev/null 2>&1; then
    echo "Namespace tagger already exists, scaling down and back up"
    kubectl scale -n tagger deploy/tagger --replicas 0
    kubectl scale -n tagger deploy/tagger --replicas 1
    exit 0
fi

echo "Creating namespace tagger"
kubectl create namespace tagger > /dev/null 2>&1 || true

echo "Installing tagger"
helm install -n tagger tagger ./chart
