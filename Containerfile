#
# Builder
#

FROM docker.io/fedora:latest AS builder
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
RUN make depctrl

#
# Application
#
FROM docker.io/fedora:latest
RUN dnf install -y device-mapper-libs
COPY --from=builder /src/output/bin/tagger /usr/local/bin/tagger
COPY --from=builder /src/output/bin/depctrl /usr/local/bin/depctrl
# 8080 pod mutating webhook handler.
# 8081 quay webhooks handler.
# 8082 docker webhooks handler.
# 8083 tags export/import handler.
# 8090 metrics endpoint.
EXPOSE 8080 8081 8082 8083 8090
