![Tagger logo](./assets/tagger.png)

![lint](https://github.com/ricardomaraschini/Tagger/workflows/lint/badge.svg?branch=main)
![unit](https://github.com/ricardomaraschini/Tagger/workflows/unit/badge.svg?branch=main)
![build](https://github.com/ricardomaraschini/Tagger/workflows/build/badge.svg?branch=main)
![image](https://github.com/ricardomaraschini/Tagger/workflows/image/badge.svg?branch=main)

Tagger keeps references to externally hosted Docker images internally in a Kubernetes cluster
by mapping their `tags` (such as `latest`) into their respective `hash` references. It also
allows Kubernetes administrators to automatically mirror these images if needed. If wanted
Tagger also provides integration with Docker Hub and Quay webhooks.

### TLDR

I have recorded a presentation (hands-on) about some of the features implemented by Tagger.
You can find it at https://youtu.be/CBbfZqLDL3o, please check it out.

### Some concepts

Images in remote repositories are tagged using a string (e.g. `latest`), these tags are not
permanent (i.e. the repository owner may push a new version for a tag at any given time).
Luckily users can also refer to images by their hashes (generally sha256) therefore one can
either pull an image using its tag (`docker.io/fedora:latest`) or utilizing its hash
(`docker.io/fedora@sha256:0123...`). Tagger takes advantage of this registry feature and
creates references to image tags using their respective hashes (a hash, in such a scenario,
may be considered a "fixed point in time" for a given image tag).

Every time one "tags" an image Tagger creates a new Generation for that image tag, making it
easier for the user, later on, to downgrade to previously tagged Generations of the same image.

### Caching (mirroring) images

Tagger allows administrators to Tag and cache images locally within the cluster. You need to
have an image registry running inside the cluster (or anywhere else) and ask `Tagger` to do
the mirror.  By doing so a copy of the remote image is going to be made into the internal
registry and all Deployments leveraging such image will automatically start to use the mirrored
copy.

### Webhooks from quay.io and Docker hub

Upon Deployment, Tagger creates two internal services, one for Quay and one for Docker Hub.
These services can then be exposed externally if you want to accept webhooks coming in from
these registries.

### How to use

Tagger leverages a _custom resource definition_ called Tag. A Tag represents an image tag in
a remote registry. For instance, a Tag called `myapp-devel` may be created to keep track of
the image `quay.io/company/myapp:latest`. A Tag _custom resource_ layout looks like this:

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
use the Tag name (i.e. `image: myapp-devel`) in a Kubernetes Deployment. Tagger will
automatically populate the pods with the right image location using the image reference by
hash. A Deployment, leveraging a Tag seems like this:

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

Two things are different here, the first one is a special Annotation (`image-tag: "true"`),
this Annotation informs Tagger that this Deployment leverages Tags and needs to be processed.
The second difference is the `image` property for the container, if it points to a Tag (by its
name), it is going to be translated properly, both Tag and Deployment on this scenario must live
in the same Namespace.

### Tag structure

On a Tag `.spec` property these fields are valid:

| Property   | Description                                                                       |
| ---------- | --------------------------------------------------------------------------------- |
| from       | Indicates the source of the image (from where Tagger should import it)            |
| generation | Points to the desired generation for the Tag, more on this below                  |
| cache      | Informs if a Tag should be mirrored to another registry, more on this below       |

#### Tag generation

Any Tag may contain multiple Generations, each Generation is identified by an integer and points
to a specific image hash. For example, once a Tag is created Tagger imports its hash and stores
it in Tag's status with generation `0`. One may, later on, need to reimport the Tag (assuming
someone pushed a new version of the image to the registry), in this case, a bump on
`spec.generation` property will do the job. By simply increasing the generation to `1` will
inform Tagger that a new import of the image needs to happen thus creating a new generation on
Tag's status (generation 1).

Tagger provides a `kubectl` plugin that allows importing a new Generation by simply issuing
`kubectl tag upgrade <tagname>`. Similar to `upgrade` one can also `downgrade` a Tag by running
`kubectl tag downgrade <tagname>`, thus making the Tag point to an older generation. All
`Deployments` using the upgraded or downgraded Tag will get automatically updated thus
triggering a new rollout of the pods, pointing to the new (upgraded) or old (downgraded) image
hash.

#### Caching images locally

For all purposes caching means mirroring, if set in a tag Tagger will mirror the image content
into another registry provided by the user. `Deployments` leveraging a cached Tag will be
automatically updated to point to the cached image. To cache (mirror) a Tag simply set its
`spec.cache` property to `true`.

To cache images locally one needs to inform Tagger about the mirror registry location. There are
two ways of doing so, the first one is by following a Kubernetes enhancement proposed laid down
[here](https://bit.ly/3rxCRqH). This enhancement proposal still does not cover things such as
authentication thus should not be used in production. Tagger can also be informed of the mirror
registry location through environment variables, as follow:


| Name                    | Description                                                          |
| ----------------------- | -------------------------------------------------------------------- |
| CACHE_REGISTRY_ADDRESS  | The internal registry URL                                            |
| CACHE_REGISTRY_USERNAME | Username Tagger should use when accessing the internal registry      |
| CACHE_REGISTRY_PASSWORD | The password to be used by Tagger                                    |
| CACHE_REGISTRY_INSECURE | Allows Tagger to access insecure registry if set to `true`           |

Cached Tags are stored in a repository with the Namespace's name used for the Tag, for example, a
Tag living in the `development` namespace will be cached (mirrored) in
`internal.registry/development/` repository.

#### Importing images from private registries

Tagger supports importing images from private registries, for that to work one needs to define a
secret with the registry credentials on the same Namespace where the Tag lives. This secret must
be of type `kubernetes.io/dockerconfigjson`. You can find more information about these secrets at
https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/

### Tag status

Follow below the properties found on a Tag `.status` property and their meaning:

| Name              | Description                                                                |
| ----------------- | -------------------------------------------------------------------------- |
| generation        | Generation currently in use                                                |
| references        | A list of all imported references (aka generations)                        |
| lastImportAttempt | Information about the last import attempt for the Tag, see below           |

The property `.status.references` is an array of imported generations, Tagger currently holds up
to five generations, every item on the array is composed of the following properties:

| Name           | Description                                                                   |
| -------------- | ----------------------------------------------------------------------------- |
| generation     | Indicate to which generation the reference belongs                            |
| from           | Keeps a reference from where the reference got imported                       |
| importedAt     | Date and time of the import                                                   |
| imageReference | Where this reference points to (by hash), may point to the internal registry  |

On a Tag status, one can also find information about the last import attempt for a Tag, it looks
like:

| Name    | Description                                                                          |
| ------- | ------------------------------------------------------------------------------------ |
| when    | Date and time of last import attempt                                                 |
| succeed | A boolean indicating if the last import was successful                               |
| reason  | In case of failure (succeed = false), what was the error                             |

### Configuring webhooks for docker.io and quay.io

One can also configure webhooks on Quay or Docker Hub to point to Tagger, thus allowing automatic
imports to happen every time a new version of the image is pushed to the registry.

Upon deployment Tagger will create two services, one for each of docker.io and quay.io, you can
then expose these services externally, as needed, through a load balancer and properly configure
the webhooks in the chosen registry to fully activate the feature.

For quay.io you just need to configure a notification, for further info refer to
https://docs.quay.io/guides/notifications.html for further information.

Bear in mind that a Tag that wants to leverage webhooks must point its `from` property to the full
registry path as Tagger does not take into account unqualified registry searches. For example, a
Tag that wants to use docker.io webhooks should have its `from` property set to
`docker.io/repository/image:tag` instead of solely `repository/image:tag`.

Every time Tagger receives a notification (webhook) it will create a new `generation` for the Tag
thus triggering first a new Tag import and later Deployment automatic rollouts. With this feature
properly configured, every time an image is pushed to the registry all Deployments leveraging it
will be automatically updated.


### Deploying

The deployment of this operator may seem a little bit cumbersome, as I move forward this process
will get simpler. For now, you gonna need to follow the procedure below.

```
$ # you may customize certs and keys in use. please remember to feed
$ # manifests/02_secret.yaml and manifests/04_webhook.yaml with your new keys
$ kubectl create namespace tagger
$ kubectl create -f ./manifests/00_crd.yaml
$ kubectl create -f ./manifests/01_rbac.yaml
$ kubectl create -f ./manifests/02_secret.yaml
$ kubectl create -f ./manifests/03_deploy.yaml
$ kubectl create -f ./manifests/04_webhook.yaml
```

### Disclaimer

The private key present on this project does not represent a problem, they are not being used
anywhere yet and to keep keys in here makes *things* easier (especially at this stage of
development). When you decide to deploy Tagger in your environment you are advised to generate
your own keys.
