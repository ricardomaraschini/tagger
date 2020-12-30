![Tagger logo](./assets/tagger.png)

![lint](https://github.com/ricardomaraschini/Tagger/workflows/lint/badge.svg?branch=main)
![unit](https://github.com/ricardomaraschini/Tagger/workflows/unit/badge.svg?branch=main)
![build](https://github.com/ricardomaraschini/Tagger/workflows/build/badge.svg?branch=main)

<div style="text-align: justify">
Tagger keeps references to externally hosted Docker images internally in a Kubernetes cluster
by mapping their `tags` (such as `latest`) into their references by `hash`. Allow Kubernetes
administrators to mirror these images internally if needed and provides integration with
docker and quay webhooks.
</div>

### About

Images in remote repositories are tagged using a string (e.g. `latest`), these tags are not
permanent, i.e. the repository owner can push a new version for the tag anytime. Luckily we
can also refer to an image by its hash (generally sha256) so one can either pull an image by
its tag (`docker.io/fedora:latest`) or by its hash (`docker.io/fedora@sha256:0123...`).

Tagger takes advantage of this registry feature and creates references to image tags using
their "fixed point in time" hashes. For instance an image `centos:latest` can be referred by
its hash as well, such as `centos@sha256:012345...`.

Every time one Tags an image Tagger creates a new generation for that image, making it easier
to downgrade to previously tagged versions in case of issues with the new generation. It also
allows users to mirror these images to an internal registry.

### Caching images locally

Tagger allow administrators to Tag and cache images locally within the cluster. You just need
to have a image registry running in the cluster (or anywhere else) and ask `Tagger` to do the
cache.  By doing so a copy of the remote image is going to be made into the internal registry
(aka mirroring) and all deployments leveraging such image will automatically start to use the
cached copy.

### Webhooks from quay.io and Docker hub

Upon deployment Tagger creates two internal services, one for Quay and one for Docker. These
services can then be exposed externally if you want to accept webhooks coming in from these
registries. Support for these webhooks is still under development but it should work for most
of the use cases.

### How to use

Tagger leverages a _custom resource definition_ called Tag. A Tag represents an image tag in a
remote registry. For instance a Tag called `myapp-devel` may be created to keep track of image
`quay.io/company/myapp:latest`. A Tag _custom resource_ layout looks like:

```yaml
apiVersion: images.io/v1
kind: Tag
metadata:
  name: myapp-devel
spec:
  from: quay.io/company/myapp:latest
  generation: 0
  cache: false
```

Once such _custom resource_ is created (with `kubectl create`, for example) Tagger will act
and check what is the current `hash` for the image `quay.io/company/myapp:latest`, storing
the reference for the hash on the Tag status property. From this point on the user can then
use the Tag name (i.e. `image: myapp-devel`) in a Kubernetes Deployment and Tagger will
automatically populate the pods with the right image location and hash. A deployment, leveraging
a Tag looks like this:

```yaml
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
this annotation informs Tagger that this Deployment leverages Tags and need to be processed.
The second difference here is the `image` property for the container, if it points to a Tag
it is going to be translated properly, both Tag and Deployment belong in the same namespace.

Follows a brief hands on presentation of this feature:

[![asciicast](https://asciinema.org/a/372131.png)](https://asciinema.org/a/372131)


### Tag structure

On a Tag `.spec` property these fiels are valid:

| Property   | Description                                                                       |
| ---------- | --------------------------------------------------------------------------------- |
| from       | Indicates the source of the image (from where Tagger should import it)            |
| generation | Points to the desired generation for the Tag, more on this below                  |
| cache      | Informs if a Tag should be mirrored to another registry, more on this below       |

#### Tag generation

Any Tag may contain multiple generations, each generation is identified by an integer and
points to a specific image hash. For example, once a Tag is created Tagger imports its hash
and stores it in Tag's status with generation `0`. One may, later on, need to reimport the
Tag (assuming someone pushed a new version of the image to the registry), on this case a
bump on `spec.generation` will do the job. By simply increasing the generation to `1` will
inform Tagger that a new import of the image needs to be made thus creating a new generation
on Tag's status.

Tagger provides a `kubectl` plugin that allows to import a new generation by simply issuing
a `kubectl tag upgrade <tagname>`. Similar to `upgrade` one can also `downgrade` a Tag by
running `kubectl tag downgrade <tagname>`, thus making the Tag point to an older generation.
All `Deployments` using the upgraded or downgraded Tag will get automatically updated thus
trigerring a new rollout of the pods, pointing to the new (upgraded) or old (downgraded)
image hash.

#### Caching images locally

For all purposes caching means mirroring, if set in a Tag Tagger will mirror the image into
another registry provided by the user. `Deployments` leveraging a cached Tag will be
automatically updated to point to the cached image. To cache (mirror) a Tag simply set its
`spec.cache` property to `true`.

In order to cache images locally one needs to inform Tagger about the registry location. There
are two ways of doing so, the first one is by following a Kubernetes enhancement proposed laid
down [here](https://bit.ly/3rxCRqH). This enhancement proposal still does not covers things 
as authentication thus should not be used in production. Tagger can also be informed of the
internal registry location by means of environment variables, as follow:


| Name                    | Description                                                          |
| ----------------------- | -------------------------------------------------------------------- |
| CACHE_REGISTRY_ADDRESS  | The internal registry URL                                            |
| CACHE_REGISTRY_USERNAME | Username Tagger should use when acessing the internal registry       |
| CACHE_REGISTRY_PASSWORD | The password to be used by Tagger                                    |
| CACHE_REGISTRY_INSECURE | Allows Tagger to access insecure registry if set to `true`           |

Cached Tags are stored in a repository with the namespace name used for the Tag, for example
a Tag living in the `development` namespace will be cached in `internal.regisry/development/`
repository.

#### Importing images from private registries

Tagger supports imports from private registries, for that to work one needs to define a secret
with the registry credentials on the same namespace where the Tag lives. This secret must be of
type `kubernetes.io/dockerconfigjson`. You can find more information about these secrets at
https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/

### Tag status

Follow below the properties found on a Tag `.status` property and their meaning:

| Name              | Description                                                                |
| ----------------- | -------------------------------------------------------------------------- |
| generation        | The current generation. Deployments using the Tag will use this generation |
| references        | A list of all imported references (aka generations)                        |
| lastImportAttempt | Information about the last import attempt for the Tag, see below           |

The property `.status.references` is an array of imported generations, Tagger currently holds
up to five generations, every item on the array is composed by the following properties:

| Name           | Description                                                                   |
| -------------- | ----------------------------------------------------------------------------- |
| generation     | Indicate to which generation the reference belongs to                         |
| from           | Keeps a reference from where the reference got imported                       |
| importedAt     | Date and time of the import                                                   |
| imageReference | Where this reference points to (by hash), may point to the internal registry  |

You can also find information about the last import attempt for a Tag

| Name    | Description                                                                          |
| ------- | ------------------------------------------------------------------------------------ |
| when    | Date and time of last import attempt                                                 |
| succeed | A boolean indicating if the last import was successful                               |
| reason  | In case of failure (succeed = false), what was the error                             |

### Configuring webhooks for docker.io and quay.io

One can also configure webhooks on quay.io or docker.io to point to Tagger, thus allowing 
automatic imports to happen everytime a new version of the image is pushed to the registry.

Upon deployment Tagger will create two services, one for each of docker.io and quay.io, you
can then expose these services externally, as needed, through a load balancer and properly
configure the webhooks in the chosen registry to fully activate this feature.

For quay.io you just need to configure a notification, for further info refer to
https://docs.quay.io/guides/notifications.html for further information.

Bare in mind that a Tag that wants to leverage webhooks must point its `from` property to
the full registry path as Tagger does not take into account unqualified registry searches.
For example, a Tag that wants to use docker.io webhooks should have its `from` property set
to `docker.io/repository/image:tag` instead of solely `repository/image:tag`.

Everytime Tagger receives a push notification (webhook) it will create a new `generation` for
the Tag thus trigerring first a new Tag import and later Deployment automatic rollouts. With
this feature properly configured everytime an image is pushed to the registry all Deployments
leveraging it will be automatically updated.


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

### Notes on updating Tagger

Tagger creates a [Mutating Webhook](https://bit.ly/2WSlvH0) that intercepts new pod creations, if
you need to update Tagger you first need to remove this webhook otherwise new pods are not going
to be created.

```
$ kubectl delete -f ./manifests/04_webhook.yaml
```

I am still working on a workaround for this caveat.
