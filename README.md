![tagger logo](./assets/tagger.png)

![lint](https://github.com/ricardomaraschini/tagger/workflows/lint/badge.svg?branch=main)
![unit](https://github.com/ricardomaraschini/tagger/workflows/unit/badge.svg?branch=main)
![build](https://github.com/ricardomaraschini/tagger/workflows/build/badge.svg?branch=main)

Tagger keeps references to externally hosted Docker images internally in a Kubernetes cluster
by mapping their `tags` (such as `latest`) into their references by `hash`. Allow Kubernetes
administrators to host these images internally if needed.

### Concepts

Images in remote repositories are tagged using a string (e.g. `latest`), these tags are not
permanent, i.e. the repository owner can push a new version for the tag. Luckily we can also
refer to an image by its hash (generally sha256) so one can either pull an image by its tag
or by its hash.

Tagger takes advantage of this repository feature and creates references to image tags using
their "fixed point in time" hashes. For instance an image `centos:latest` can be pulled by
its hash as well, such as `centos@sha256:012345...`.

Every time one tags an image `tagger` creates a new generation for that image, making it easier
to downgrade to previously tagged versions in case of issues with the new generation.

### Caching images locally

Tagger allow administrators to tag and cache images locally within the cluster. You just need
to have a image registry running internal to the cluster and ask `tagger` to cache the image.
By doing so a copy of the remote image is going to be made into the internal registry and all
deployments leveraging such image will automatically pass to use the cached copy.

### Webhooks from quay.io and Docker hub

After the `tagger` deployment two internal services will be created, one for Quay and one for
Docker. These services can then be exposed externally if you want to accept webhooks coming in
from either quay.io or docker.io. Support for these webhooks is still under development but it
should work for most of the use cases.

### Use

A brief hands on presentation

[![asciicast](https://asciinema.org/a/372131.png)](https://asciinema.org/a/372131)

### Disclaimer

The private key present on this project does not represent a problem, it is not being used
anywhere yet and to keep keys in here makes *things* easier (specially at this stage).

### Deploying

The deployment of this operator is a little bit cumbersome, as I move forward this process will
get simpler. For now you gonna need to follow the procedure below.

```
$ # you should customize certs and keys in use. please remember to feed
$ # manifests/02_secret.yaml and manifests/04_webhook.yaml with the keys
$ kubectl create namespace tagger
$ kubectl create -f ./manifests/00_crd.yaml
$ kubectl create -f ./manifests/01_rbac.yaml
$ kubectl create -f ./manifests/02_secret.yaml
$ kubectl create -f ./manifests/03_deploy.yaml
$ kubectl create -f ./manifests/04_webhook.yaml
```
