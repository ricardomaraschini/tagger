FROM golang:1.15.3-buster AS builder
RUN apt-get update -y
RUN apt-get install -y libgpgme-dev
WORKDIR /go/src/tagger
COPY . .
RUN make build


FROM centos:8
WORKDIR /
EXPOSE 8080 8081
COPY --from=builder /go/src/tagger/_output/bin/tagger /usr/local/bin/
CMD "/usr/local/bin/tagger"
