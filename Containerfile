FROM golang:latest AS builder
WORKDIR /src
ARG version
ENV VERSION=${version:-v0.0.0}
COPY . .
RUN make tagger

FROM docker.io/fedora:42
COPY --from=builder /src/output/bin/tagger /usr/local/bin/tagger
EXPOSE 8080 8083 8090
