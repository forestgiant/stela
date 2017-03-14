# log
This package allows developers to easily create structured, leveled logs.

## Installation
This package was written in the Go programming language.  Once you have Go installed, you can install this package by issuing the following command.
```
go get github.com/forestgiant/log
```

Once the command above has finished, you should be able to import the package for use in your own code as follows.
```
import "github.com/forestgiant/log"
```

## Log Levels
This package provides convenience methods which log at the following levels.  When these methods are used, the corresponding levels are automatically associated with the `level` key in the log output.
- Alert - alert
- Critical - critical
- Debug - debug
- Emergency - emergency
- Error - error
- Info - info
- Notice - notice
- Warning - warning

## Log Formats
This package provides `Formatter` implementations capable of structuring logs in the following formats.  If you would prefer to use an alternate format, you can implement the Formatter interface and provide your custom `Formatter` to the `Logger`.
- JSON - default
- logfmt

## Log Output
By default, this package will log to `os.Stdout`.  If you would prefer, output can be written to an alternate `io.Writer`.

## Context
Each `Logger` has a series of key-value pairs that make up it's context.  If values exist in the `Logger`'s context, these values will be present in the log output.

## Value Generators
Unlike most values that are added to a `Logger`'s context, a `ValueGenerator` is a function that will be evaluated at the time the log entry is formatted.  This is useful in scenarios where you may want to provide some runtime context to your logs, such as line numbers or timestamps.

## Documentation and Examples
If you need more information, please check out our [GoDoc page](https://godoc.org/github.com/forestgiant/log).  You will find a number of [examples](https://godoc.org/github.com/forestgiant/log#pkg-examples) on that page that should help understand how this package can best be used.  If you notice anything out of place or undocumented, be sure to let us know!

## Inspiration
This package was largely based on [Go kit's log package](https://github.com/go-kit/kit/tree/master/log).  Through reading the source, as well as a number of conversations they pointed out in their GitHub issues, I feel I was able to get a much better grasp on the topic of structured logging in general.  I'd like to extend my thanks to that team for having made such a great resource!
