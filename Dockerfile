FROM docker.io/library/golang:1.15.3-buster AS builder
RUN apt-get update -y
RUN apt-get install -y libgpgme-dev
WORKDIR /go/src/tagger
COPY . .
RUN make build


FROM registry.centos.org/centos:8
WORKDIR /
# 8080 pod mutating webhook handler.
# 8081 quay webhooks handler.
# 8082 docker webhooks handler.
# 8083 tags export/import handler.
# 8090 metrics endpoint.
EXPOSE 8080 8081 8082 8083 8090
COPY --from=builder /go/src/tagger/_output/bin/tagger /usr/local/bin/
CMD "/usr/local/bin/tagger"
