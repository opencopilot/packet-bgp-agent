### Packet BGP Agent

Watches Packet metadata for changes to a `customdata` field called `BGP_ANNOUNCE`, adds the specified IP blocks to the loopback device, and uses `gobgp` to begin announcing those IPs.

#### Usage

Can be run as a docker container:

`docker run --net host --cap-add NET_ADMIN -e MD5_PASSWORD=... opencopilot/packet-bgp-agent`

Note that host networking and `--cap-add NET_ADMIN` are required to configure networking on the host.