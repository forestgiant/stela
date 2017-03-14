package log

import (
	"fmt"
	"path"
	"runtime"
	"time"
)

// A ValueGenerator is a function that generates a dynamic value to be evaluated at the time of a log event
type ValueGenerator func() interface{}

// DefaultTimestamp is a ValueGenerator that returns the current time in RFC3339 format
var DefaultTimestamp ValueGenerator = func() interface{} {
	return time.Now().Format(time.RFC3339)
}

// Caller is a ValueGenerator that returns the file and line number where the log event originated
func Caller(depth int) ValueGenerator {
	return func() interface{} {
		_, fp, line, _ := runtime.Caller(depth)
		return fmt.Sprintf("%s:%d", path.Base(fp), line)
	}
}

// DefaultCaller is a Callert with a depth that corresponds to the first call outside of this package
var DefaultCaller = Caller(5)
