#!/usr/bin/env bash
set -euo pipefail

# these are the default values using for the docker network. these values match
# the values used to configure the metal load balancer ip pools. if you change
# here then you also need to change there.
DOCKER_NETWORK_CIDR=172.18.100.0/24
DOCKER_NETWORK_DEF_GW=172.18.100.1

# if the kind network does not exist we create it using a specific subnet and
# gateway. we need to guarantee the right values here in order to properly
# configure the metal load balancer later on.
if ! docker network ls | grep -q kind; then
    echo "Creating kind network"
    docker network create                  \
        --scope local                      \
        --gateway "$DOCKER_NETWORK_DEF_GW" \
        --driver bridge                    \
        --subnet "$DOCKER_NETWORK_CIDR"    \
        kind
elif docker network inspect kind | jq -r .[].IPAM.Config[].Gateway | egrep -q "^$DOCKER_NETWORK_DEF_GW$"; then
    echo "Reusing existing kind network"
    if kind get clusters | egrep -q '^kind$'; then
        echo "kind cluster already exists"
        exit 1
    fi
else
    echo "Network 'kind' exists with a different gateway, remove it and try again"
    exit 1
fi


echo "Creating kind cluster"
kind create cluster                                  \
	--config .github/workflows/etc/kind-cluster.yaml \
	--name kind

echo "Fixing kubeconfig permissions"
chmod 600 ~/.kube/config > /dev/null 2>&1 || true

echo "Installing metallb"
kubectl create namespace metallb
helm repo add metallb https://metallb.github.io/metallb
helm install --wait -n metallb metallb metallb/metallb

echo "Configuring metallb"
kubectl apply -f ./.github/workflows/etc/metallb.yaml

echo "All done, cluster is ready"
