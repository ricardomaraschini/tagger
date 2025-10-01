#!/usr/bin/env bash
set -euo pipefail

echo "Creating temporary directory to hold kubeconfig"
mkdir -p output/auth

echo "Getting kubeconfig from kind and using it"
kind get kubeconfig --name kind > output/auth/kind.kubeconfig
export KUBECONFIG=output/auth/kind.kubeconfig

echo "Cleaning up previously created resources if any"
kubectl delete serviceaccount cluster-admin-service-account > /dev/null 2>&1 || true
kubectl delete clusterrolebinding cluster-admin-sevice-account-binding > /dev/null 2>&1 || true
kubectl delete token cluster-admin-service-account > /dev/null 2>&1 || true

echo "Creating cluster admin service account in kube-system namespace"
kubectl create serviceaccount     \
    cluster-admin-service-account

echo "Binding cluster-admin role to the service account"
kubectl create clusterrolebinding                          \
    --clusterrole=cluster-admin                            \
    --serviceaccount=default:cluster-admin-service-account \
    cluster-admin-sevice-account-binding

echo "Creating token"
TOKEN=$(kubectl create token cluster-admin-service-account)

echo "Extracting cluster info from kind kubeconfig"
CLUSTER_NAME=$(kubectl config view --minify -o jsonpath='{.contexts[0].context.cluster}')
CLUSTER_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CLUSTER_CA=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')

echo "Creating kubeconfig for the service account"
cat > output/auth/kubeconfig <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: ${CLUSTER_CA}
    server: ${CLUSTER_SERVER}
  name: ${CLUSTER_NAME}
contexts:
- context:
    cluster: ${CLUSTER_NAME}
    user: cluster-admin-service-account
  name: cluster-admin-service-account@${CLUSTER_NAME}
current-context: cluster-admin-service-account@${CLUSTER_NAME}
users:
- name: cluster-admin-service-account
  user:
    token: "$TOKEN"
EOF

echo "Cleaning up temporary kind kubeconfig"
rm -rf output/auth/kind.kubeconfig

echo
echo "########################################################################"
echo "# Kubeconfig for the service account created at output/auth/kubeconfig #"
echo "# You need now to set KUBECONFIG=output/auth/kubeconfig to use it.     #"
echo "########################################################################"
echo
