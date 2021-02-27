#
# Builder
#

FROM registry.fedoraproject.org/fedora:latest AS builder

WORKDIR /src
COPY . .

RUN dnf install -y \
    btrfs-progs-devel \
    device-mapper-devel \
    golang \
    gpgme-devel \
    make

RUN make

#
# Application
#

FROM registry.fedoraproject.org/fedora:latest

COPY --from=builder /src/_output/bin/tagger /usr/local/bin/tagger

# 8080 pod mutating webhook handler.
# 8081 quay webhooks handler.
# 8082 docker webhooks handler.
# 8083 tags export/import handler.
# 8090 metrics endpoint.
EXPOSE 8080 8081 8082 8083 8090

ENTRYPOINT [ "/usr/local/bin/tagger" ]
