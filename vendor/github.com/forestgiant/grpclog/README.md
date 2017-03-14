# grpclog

This package provides structures that implement the [grpc Logger interface](http://google.golang.org/grpc/grpclog) with the purpose of presenting log messages in a structured format.

The `Structured` type wraps a [ForestGiant Logger](http://github.com/forestgiant/log), allowing grpc log messages to be presented in a structured format.

The `Suppressed` type will simply suppress the output it receives.
