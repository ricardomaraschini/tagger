![Tagger logo](./assets/tagger.png)

![lint](https://github.com/ricardomaraschini/Tagger/workflows/lint/badge.svg?branch=main)
![unit](https://github.com/ricardomaraschini/Tagger/workflows/unit/badge.svg?branch=main)
![build](https://github.com/ricardomaraschini/Tagger/workflows/build/badge.svg?branch=main)
![image](https://github.com/ricardomaraschini/Tagger/workflows/image/badge.svg?branch=main)
![release](https://github.com/ricardomaraschini/Tagger/workflows/release/badge.svg?branch=main)

### Motivation

Keeping track of all Container Images in use in a Kubernetes cluster is a complicated task.
Container Images may come from numerous different Image Registries. In some cases, controlling
how a stable version of a given Container Image looks escapes the user's authority. To add to
this, Container Runtimes rely on remote registries (from the cluster's point of view) when
obtaining Container Images, potentially making the process of pulling their blobs (manifests,
config, and layers) slower.

The notion of indexing Container Image versions by Tags is helpful. Still, it does not provide
users with the right confidence to always use the intended Container Image – today's "latest"
tag might not be tomorrow's "latest" tag. In addition to that, these Image Registries allow
access to Container Images by their Manifest content's hash (i.e., usually sha256), which gives
users the confidence at a cost in semantics.

When releasing a new version of an application to Push and to Deploy are split into two distinct
steps. Both the pusher and the puller need access to the same Image Registry, adding complexity.
Credentials are one example of the concerns. Other factors may pop up when running, for instance,
in an air-gapped environment, where the cluster may not reach external Image Registries.

Tagger aims to overcome these caveats by providing an image management abstraction layer. For
instance, by providing a direct mapping between a Container Image tag (e.g., "latest") and its
correspondent Manifest content's hash, users can then refer to the Container Image by its tag
– and be sure to use that specific version. More than that, when allied with an Internal Image
Registry, Tagger can also automatically mirror Container Images into the cluster.

While using Tagger, Deployments can refer to Container Images by an arbitrarily defined name,
such as "my-app", and Tagger will make sure that they use the right Container Image through its
internal "tag to Manifest content's hash" mapping.

For each new "release" of a given Container Image, Tagger creates a new Generation for it,
making it easy to roll back to previously pushed "releases" of the same Container Image in case
of problems.

When integrated with an Internal Registry, Tagger allows users to push or pull Images directly
without requiring an external Image Registry. It works as a layer between the user and the
Internal Registry. Every time a new "release" of Container Images is pushed, all Deployments are
updated automatically. Users don't need to know about the Internal Registry existence, if they
are logged-in to the Kubernetes cluster, they can obtain old or provide new "Generations" for a
Container Image.

In summary, Tagger mirrors remote Container Images into a Kubernetes cluster indexing them in
different Generations (allowing easy navigation through these multiple Generations), provides an
interface allowing users to pull and push images directly to the Kubernetes cluster and provides
full integration with Kubernetes Deployments (automatic triggers new rollouts on Container Image
changes).

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

### Mirroring images

Tagger allows administrators to Tag and mirror images locally within the cluster. You need to
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
apiVersion: tagger.dev/v1beta1
kind: Tag
metadata:
  name: myapp-devel
spec:
  from: quay.io/company/myapp:latest
  generation: 0
  mirror: false
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
| mirror     | Informs if a Tag should be mirrored to another registry, more on this below       |

#### Tag generation

Any Tag may contain multiple Generations, each Generation is identified by an integer and points
to a specific image hash. For example, once a Tag is created Tagger imports its hash and stores
it in Tag's status with generation `0`. One may, later on, need to reimport the Tag (assuming
someone pushed a new version of the image to the registry), in this case, a bump on
`spec.generation` property will do the job. By simply increasing the generation to `1` will
inform Tagger that a new import of the image needs to happen thus creating a new generation on
Tag's status (generation 1).

Tagger provides a `kubectl` plugin that allows upgrades from one generation into the next newest
one: `kubectl tag upgrade <tagname>`. Similar to `upgrade` one can also `downgrade` a Tag by
running `kubectl tag downgrade <tagname>`, thus making the Tag point to an older generation. All
`Deployments` using the upgraded or downgraded Tag will get automatically updated thus
triggering a new rollout of the pods, pointing to the new (upgraded) or old (downgraded) image
hash.

#### Mirroring images locally

If mirroring is set in a tag Tagger will mirror the image content into another registry provided
by the user. `Deployments` leveraging a mirrored Tag will be automatically updated to point to the
mirrored image. To mirror a Tag simply set its `spec.mirror` property to `true`.

To mirror images locally one needs to inform Tagger about the mirror registry location. There are
two ways of doing so, the first one is by following a Kubernetes enhancement proposed laid down
[here](https://bit.ly/3rxCRqH). This enhancement proposal still does not cover things such as
authentication thus should not be used in production. Tagger can also be informed of the mirror
registry location through a Secret called `mirror-registry-config`, this secret may contain the
following properties:


| Name     | Description                                                                         |
| ---------| ----------------------------------------------------------------------------------- |
| address  | The mirror registry URL                                                             |
| username | Username Tagger should use when accessing the mirror registry                       |
| password | The password to be used by Tagger                                                   |
| token    | The auth token to be used by Tagger (optional)                                      |
| insecure | Allows Tagger to access insecure registry if set to "true" (string)                 |

Follow below an example of a `mirror-registry-config` Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mirror-registry-config
  namespace: tagger
data:
  address: cmVnaXN0cnkuaW8=
  username: YWRtaW4=
  password: d2hhdCB3ZXJlIHlvdSB0aGlua2luZz8K
```

Mirrored Tags are stored in a repository with the Namespace's name used for the Tag, for example,
a Tag living in the `development` namespace will be mirrored in `internal.registry/development/`
repository.

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
| imageReference | Where this reference points to (by hash), may point to the mirror registry    |

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

You can deploy Tagger using Helm:

```
$ RELEASE=v2.0.6
$ BASEURL=https://github.com/ricardomaraschini/tagger/releases/download
$ helm install tagger $BASEURL/$RELEASE/helm-chart.tgz
```

To get a list of what can be customized during the deployment you can run the following commands

```
$ RELEASE=v2.0.6
$ BASEURL=https://github.com/ricardomaraschini/tagger/releases/download
$ helm show values $BASEURL/$RELEASE/helm-chart.tgz
```

`RELEASE` variable may be set to point to any of this repository's release. You can view a full
list of all releases in https://github.com/ricardomaraschini/tagger/releases. There is also a
development release that can be installed by running the following commands:

```
$ RELEASE=latest
$ BASEURL=https://github.com/ricardomaraschini/tagger/releases/download
$ helm show values $BASEURL/$RELEASE/helm-chart.tgz
```

You can inspect the objects being created during the installation by looking in `templates` dir
inside `assets/helm-chart` or by running the following commands:

```
$ RELEASE=v2.0.6
$ BASEURL=https://github.com/ricardomaraschini/tagger/releases/download
$ helm install --dry-run tagger $BASEURL/$RELEASE/helm-chart.tgz
```

By default Tagger won't be able to mirror until you provide it with a mirror registry config.
You can configure the mirror by editing the Secret `mirror-registry-config` in the operator
namespace. Follow an example of a valid `mirror-registry-config` secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mirror-registry-config
data:
  address: cmVnaXN0cnkuaW8=
  username: YWRtaW4=
  password: d2hhdCB3ZXJlIHlvdSB0aGlua2luZz8K
  token: YW4gb3B0aW9uYWwgdG9rZW4gZm9yIHRva2VuIGJhc2VkIGF1dGg=
  insecure: dHJ1ZQ==
```
