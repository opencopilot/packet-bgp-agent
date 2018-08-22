FROM golang:alpine3.8

WORKDIR /go/src/github.com/opencopilot/packet-bgp-agent
COPY "cmd" "cmd"

RUN apk update;

# RUN go build -o cmd/packet-bgp-agent

ENTRYPOINT [ "cmd/packet-bgp-agent" ]