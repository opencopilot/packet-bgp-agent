### Packet BGP Agent

Watches Packet metadata for changes to a `customdata` field called `BGP_ANNOUNCE`, adds the specified IP blocks to the loopback device, and uses `gobgp` to begin announcing those IPs.

#### Usage

Can be run as a docker container:

`docker run --net host --cap-add NET_ADMIN -e MD5_PASSWORD=... opencopilot/packet-bgp-agent`

Note that host networking and `--cap-add NET_ADMIN` are required to configure networking on the host.

| ENV Var | Description | Default |
|---|---|---|
|`MD5_PASSWORD`| MD5 password to use| (empty string)|
|`ASN`|ASN to announce| `65000`|


#### Setting Custom Data

You can set `customdata` (an arbitrary json blob) on a Packet device like so:

`curl -H 'X-Auth-Token: XXX' -v -H "Content-Type: application/json" -X PUT -d '{"customdata": {"BGP_ANNOUNCE":["147.75.65.xxx/31", "147.75.73.xxx/32"]}}' https://api.packet.net/devices/DEVICE_ID`

#### Dependencies

This code uses the [netlink](https://github.com/vishvananda/netlink) library and [gobgp](https://github.com/osrg/gobgp)
