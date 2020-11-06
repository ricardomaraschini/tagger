# Tagger

Tagger helps keeping references to external Docker images internally  
to a Kubernetes cluster. It maps remote image `tags` into references
by `hash` and keeps track of them.

# What is a tag

TBD.

### Disclaimer

The private key present on this project is not a problem, this is
not being used anywhere yet and to keep keys in here makes *things*
easier (specially at this stage of development).


### Deploying

```
$ # you can customize certificates in use. please remember to update
$ # manifests/secret.yaml and manifests/mutatingwebhook.yaml with
$ # the right information.
$ kubectl create namespace tagger
$ kubectl create -f ./manifests/crd.yaml
$ kubectl create -f ./manifests/rbac.yaml
$ kubectl create -f ./manifests/secret.yaml
$ kubectl create -f ./manifests/deploy.yaml
$ kubectl create -f ./manifests/service.yaml
$ kubectl create -f ./manifests/mutatingwebhook.yaml
```
