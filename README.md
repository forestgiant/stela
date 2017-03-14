## Stela
Simple service discover using DNS SRV with an HTTP API. The project is named after the [stone monuments](https://en.wikipedia.org/wiki/Stela) of the same name.

## Running stela nodes
`stela` uses raft consensus.

Start your first instance
$ `stela -raftdir ~/stela/node0`

Start two other instances on the same computer.

$ `stela -port 9001 -dnsport 8054 -raftdir ~/stela/node1`

$ `stela -port 9002 -dnsport 8055 -raftdir ~/stela/node2`

Or start another instances on a different computer.

$ `stela -raftdir ~/stela/node0`

## Examples
Refer to `api/example_test.go`

## Health checking
`stela` deregisters services after 5 seconds. All api's should re-register every 2 seconds.

## JSON API
Creates UDP and TCP DNS server that responds to A and SRV records.

To register a service, post JSON to: `http://127.0.0.1:9000/registerservice` every 2 seconds.
{
	"name":     "test.service.fg",
	"target":   "jlu-macbook.local",
	"address":	"10.0.1.48"
    "port":     8010
}
```
You can test if it was registered with dig:
```
dig @127.0.0.1 -p 8053 test.service.fg SRV
```

or Look up the A record
```
dig @127.0.0.1 -p 8053 jlu-macbook.local
```

To deregister a service, post the same JSON to: `http://127.0.0.1:9000/deregisterservice`
```
{
	"name":     "test.service.fg",
	"target":   "jlu.macbook.local.",
	"address":	"10.0.1.48"
    "port":     8010
}
```

## Resources
* http://godoc.org/github.com/miekg/dns
* https://github.com/boltdb/bolt
* Example Project: https://github.com/miekg/exdns/blob/master/reflect/reflect.go
