## PortUtil
Quickly verify if a port is available or get a unique port (TCP/UDP).

## Installation
`go get -u github.com/forestgiant/portutil`

or vendor with [gv](https://github.com/forestgiant/gv/)
`gv get github.com/forestgiant/portutil`

## Docs
http://godoc.org/github.com/forestgiant/portutil

## Examples
### Verify, VerifyTCP, VerifyUDP:
```
port, err := portutil.Verify("tcp", 8080)
if err != nil {
	log.Fatal(err)
}
```
```
port, err := portutil.VerifyUDP(8080)
if err != nil {
	log.Fatal(err)
}
```
```
port, err := portutil.VerifyTCP(8080)
if err != nil {
	log.Fatal(err)
}
```

Verify Port from a HostPort string
```
serviceHost, err := portutil.VerifyHostPort("tcp", "127.0.0.1:8080")
if err != nil {
	log.Fatal(err)
}
```

### GetUnique, GetUniqueTCP, GetUniqueUDP:
```
port, err := portutil.GetUnique("tcp")
if err != nil {
	log.Fatal(err)
}
```
```
port, err := portutil.GetUniqueTCP
if err != nil {
	log.Fatal(err)
}
```
```
port, err := portutil.GetUniqueUDP
if err != nil {
	log.Fatal(err)
}
```