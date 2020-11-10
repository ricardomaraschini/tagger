![tagger logo](./assets/tagger.png)

Tagger keeps references to external Docker images internally to a Kubernetes cluster. It
maps remote image `tags` (such as `latest`) into references by `hash`.

### Concepts

Images on remote repositories are tagged using a string (e.g. `latest`), these tags are
not permanent, i.e. the repository owner can push a new version for the tag. Luckily we
can also refer to an image by its hash (generally sha256) so one can either pull an image
by its tag or by its hash.

Tagger takes advantage of this repository feature and creates references to image tags
using their "fixed point in time" hashes. For instance an image `centos:latest` can be
refered as `centos@sha256:012345...`.

Every time one tags an image tagger creates a new generation for that image, making it
easier to downgrade to previous version in case of issues with the tagged image.

### Caching images locally

Tagger allow users to tag and cache images locally within the cluster. You just need to
have a image registry running internall to the cluster and ask tagger to cache the image.
With that a copy of the remote image is going to be made to the internal registry.

### Disclaimer

The private key present on this project is not a problem, this is not being used anywhere
yet and to keep keys in here makes *things* easier (specially at this stage of development).

### Deploying

The deployment of this operator is a little bit cumbersome, as we move forward this process
will get simpler. For now you gonna need to follow the procedure below.

```
$ # you should customize certificates in use. please remember to feed
$ # manifests/02_secret.yaml and manifests/04_webhook.yaml with the
$ # right information.
$ kubectl create namespace tagger
$ kubectl create -f ./manifests/00_crd.yaml
$ kubectl create -f ./manifests/01_rbac.yaml
$ kubectl create -f ./manifests/02_secret.yaml
$ kubectl create -f ./manifests/03_deploy.yaml
$ kubectl create -f ./manifests/04_webhook.yaml
```
