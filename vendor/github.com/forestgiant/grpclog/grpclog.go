package grpclog

import (
	"fmt"
)

// Logger that receives the defined Info and Error methods
type Logger interface {
	Info(string, ...interface{}) error
	Error(string, ...interface{}) error
}

// Structured implements the github.com/grpc/grpc-go/grpclog Logger
// interface by wrapping a github.com/forestgiant/log Logger
type Structured struct {
	Logger Logger
}

// Fatal will result in an Error level log output
func (l *Structured) Fatal(args ...interface{}) {
	l.Logger.Error(fmt.Sprint(args))
}

// Fatalf will result in an Error level log output
func (l *Structured) Fatalf(format string, args ...interface{}) {
	l.Logger.Error(fmt.Sprintf(format, args))
}

// Fatalln will result in an Error level log output
func (l *Structured) Fatalln(args ...interface{}) {
	l.Logger.Error(fmt.Sprint(args))
}

// Print will result in an Info level log output
func (l *Structured) Print(args ...interface{}) {
	l.Logger.Info(fmt.Sprint(args))
}

// Printf will result in an Info level log output
func (l *Structured) Printf(format string, args ...interface{}) {
	l.Logger.Info(fmt.Sprintf(format, args...))
}

// Println will result in an Info level log output
func (l *Structured) Println(args ...interface{}) {
	l.Logger.Info(fmt.Sprint(args))
}

// Suppressed will result in suppressed grpc log output
type Suppressed struct{}

// Fatal will result in no output
func (l *Suppressed) Fatal(args ...interface{}) {}

// Fatalf will result in no output
func (l *Suppressed) Fatalf(format string, args ...interface{}) {}

// Fatalln will result in no output
func (l *Suppressed) Fatalln(args ...interface{}) {}

// Print will result in no output
func (l *Suppressed) Print(args ...interface{}) {}

// Printf will result in no output
func (l *Suppressed) Printf(format string, args ...interface{}) {}

// Println will result in no output
func (l *Suppressed) Println(args ...interface{}) {}
