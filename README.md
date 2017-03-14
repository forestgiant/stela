# Stela
A microservice used for discovery of services with gRPC. Stela uses IPv6 multicast to allow for scalable, distributed discovery on premise.  The project is named after the [stone monuments](https://en.wikipedia.org/wiki/Stela) of the same name.

### Features
* **Distributed** run one or many Stela instances on any number of computers. 
* **Subscriptions** subscribe to service names to receive a stream of any services registered or deregistered with that name.
* **Simple** Stela is meant to be started quickly with little configuration. It only focuses on service registration.

## Running stela nodes
By default communicates over gRPC `:3100` and IPv6 multicast `:31053`. 
Start your first instance:

`stela -cert /path/to/server.crt -key /path/to/server.key -ca /path/to/ca.crt`

If the SSL files you wish to use are stored as `server.crt`, `server.key`, and `ca.crt` in the directory where the command is issued, you can omit the `cert`, `key`, and `ca` parameters.

`stela`

Start two other instances on the same computer.

$ `stela -port 31001 -multicast 31054`

$ `stela -port 31002 -multicast 31055`

Or start another instances on a different computer.

$ `stela`

Services registered with any of the instances will be discoverable by all.

## Examples
Refer to `api/example_test.go`