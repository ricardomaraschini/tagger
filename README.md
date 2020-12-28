![tagger logo](./assets/tagger.png)

![lint](https://github.com/ricardomaraschini/tagger/workflows/lint/badge.svg?branch=main)
![unit](https://github.com/ricardomaraschini/tagger/workflows/unit/badge.svg?branch=main)
![build](https://github.com/ricardomaraschini/tagger/workflows/build/badge.svg?branch=main)

Tagger keeps references to externally hosted Docker images internally in a Kubernetes cluster
by mapping their `tags` (such as `latest`) into their references by `hash`. Allow Kubernetes
administrators to host these images internally if needed and provides integration with docker
and quay webhooks.

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

### How to use

Tag _custom resource definition_ represents an image tag in a remote registry. For instance a
tag called `myapp-devel` can be created to keep track of `quay.io/company/myapp:latest`. Tags
_custom resource_ layout looks like:

```
apiVersion: images.io/v1
kind: Tag
metadata:
  name: myapp-devel
spec:
  from: quay.io/company/myapp:latest
  generation: 0
  cache: false
```

Once such _custom resource_ is created (with `kubectl create`, for example) tagger will act
and check what is the current `hash` for the image `quay.io/company/myapp:latest` and store
the reference for the hash on the tag status property. From this point on the user can then
refer to `image: myapp-devel` in a Kubernetes Deployment and tagger will automatically populate
the pods with the right image location and hash. A deployment, leveraging a Tag looks like this:

```
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    image-tag: "true"
  name: myapp 
  labels:
    app: myapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
      - name: myapp
        image: myapp-devel
```

Two things are different here, the first one is a special annotation (`image-tag: "true"`),
this annotation informs tagger that this Deployment leverages Tags and need to be processed.
The second difference here is the `image` property for the container, if it points to a tag
it is going to be translated properly.

Follows a brief hands on presentation of this feature:

[![asciicast](https://asciinema.org/a/372131.png)](https://asciinema.org/a/372131)


### Tag structure

| property        | description                                                                 |
| --------------- | --------------------------------------------------------------------------- |
| spec.from       | Indicates the source of the image, from where tagger should import it       |
| spec.generation | Points to the desired generation for the tag, more on this below            |
| spec.cache      | Informs if a tag should be mirrored to another registry, more on this below |

#### Tag generation

Every tag may contain multiple generations, each generation is identified by an integer and
points to a specific image hash. For example, once a tag is created tagger imports its hash
and store it on tag status with generation `0`. One may, later on, need to reimport the tag,
on this case a bump on `spec.generation` will do the job. By simply increasing the generation
to `1` will inform tagger that a new import of the image needs to be made thus creating a new
generation on tag status.

Tagger provides a `kubectl` plugin that allows to import a new generation by simply issuing
a `kubectl tag upgrade <tagname>`. Similar to `upgrade` one can also `downgrade` a tag by
running `kubectl tag downgrade <tagname>`, thus making the tag point to an older version.
All `Deployments` using the upgraded or downgraded tag will get automatically updated thus
trigerring a new rollout of the pods, pointing to the new (upgraded) or old (downgraded)
image hash.

#### Caching images locally

Caching means mirroring, if set in an tag tagger will mirror the image into another registry, 
`Deployments` leveraging a cached tag will be automatically updated to point to the cached
image automatically. To cache (mirror) an tag simply set its `spec.cache` property to `true`.

In order to cache images locally one needs to inform tagger about the registry location and
there are two ways of doing so, the first one is by following Kubernetes
[enhancement proposal](https://bit.ly/3rxCRqH) on having an internal registry. This enhancement
proposal still does not support authentication, thus should not be used in production. Tagger
can also be informed of the internal registry location through environment variables:


| Variable                | Description                                                    |
| ----------------------- | -------------------------------------------------------------- |
| CACHE_REGISTRY_ADDRESS  | The internal registry URL                                      |
| CACHE_REGISTRY_USERNAME | Username tagger should use when acessing the internal registry |
| CACHE_REGISTRY_PASSWORD | The password to be used by tagger                              |
| CACHE_REGISTRY_INSECURE | Allows tagger to access insecure registry if set to `true`     |

Cached tags are stored in a repository with the namespace name used for the tag, for example
a tag in the `development` namespace will be cached to `internal.regisry/development/tag`
repository in the registry.

#### Importing images from private registries

Tagger supports imports from private registries, for that to work one needs to define a secret
with the registry credentials on the same namespace where the tag lives. This secret must be of
type `kubernetes.io/dockerconfigjson`. You can find more information on these secrets at
https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/

### Configuring webhooks for docker.io and quay.io

One can also configure webhooks on quay.io or docker.io to point to tagger, thus allowing 
automatic imports to happen everytime a new version of the image is pushed to the registry.

Upon deployment tagger will create two services, one for each of docker.io and quay.io, you
can then expose these services aexternally through a load balancer and properly configure the
webhooks in the chosen registry.

For quay.io you just need to configure a notification, please follow
https://docs.quay.io/guides/notifications.html for further information.

Bare in mind that a tag that wants to leverage webhooks must point its `from` property to
the full registry path as tagger doesn't take into account unqualified registry searches.
For example, a tag that wants to use docker.io webhooks should have its `from` property set to
`docker.io/repository/image:tag` instead of only `repository/image:tag`.

Everytime tagger receives a push notification it will create a new `generation` for the tag thus
trigerring first a new tag import and later Deployment automatic rollouts. With this feature
properly configured everytime an image is pushed to the registry all Deployments leveraging it
will be automatically updated. One can also issue a `kubectl tag downgrade <tagname>` to 
rollback to the previous image.


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
