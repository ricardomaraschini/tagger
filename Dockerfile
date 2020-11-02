FROM golang:1.15.3-buster AS builder
RUN apt-get update -y
RUN apt-get install -y libgpgme-dev
WORKDIR /go/src/tagger
COPY . .
RUN go build -o /tagger ./cmd/tagger


FROM centos:8
WORKDIR /
COPY --from=builder /tagger /usr/local/bin
CMD "/usr/local/bin/tagger"
