FROM golang:1.15.3-buster AS builder
RUN apt-get update -y
RUN apt-get install -y libgpgme-dev
WORKDIR /go/src/it
COPY . .
RUN go build -o /it ./cmd/it


FROM centos:8
WORKDIR /
COPY --from=builder /it /usr/local/bin
CMD "/usr/local/bin/it"
