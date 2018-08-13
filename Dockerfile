FROM golang:alpine

WORKDIR /go/src/github.com/opencopilot/packet-bgp-agent
COPY . .

RUN apk update; apk add curl; apk add git;
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
# RUN dep ensure -vendor-only -v

RUN go build -o cmd/packet-bgp-agent

ENTRYPOINT [ "cmd/packet-bgp-agent" ]