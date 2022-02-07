#
# Builder
#

FROM docker.io/fedora:34 AS builder
RUN dnf install -y \
    btrfs-progs-devel \
    device-mapper-devel \
    gpgme-devel \
    go \
    make
WORKDIR /src
ARG version
ENV VERSION=${version:-v0.0.0}
COPY . .
RUN make tagger
RUN make kubectl-image

#
# Application
#
FROM docker.io/fedora:34
RUN dnf install -y device-mapper-libs
COPY --from=builder /src/output/bin/tagger /usr/local/bin/tagger
COPY --from=builder /src/output/bin/kubectl-image /usr/local/bin/kubectl-image
# 8080 mutating webhook handlers.
# 8083 images export/import handler.
# 8090 metrics endpoint.
EXPOSE 8080 8083 8090
