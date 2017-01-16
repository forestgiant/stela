# stelawd
Is a watch-dog daemon to define a set of services that will be monitored. 

## Config File
The config file defaults to watch.list. This is a simple csv config file that uses `#` as comments.
The format is `# service name, host:port, interval (ms), value` with newlines indicating a new service to watch.

You can pass in any config file conforming to this format using `-watchlist` flag
