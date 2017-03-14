package log

import (
	"errors"
	"io"
	"os"
	"sync"
)

//ErrIgnoredLogLevel is the error returned when a logger ignores the log at the current level configuration
var ErrIgnoredLogLevel = errors.New("Log level is ignored by the logger")

// Logger implements the Logger interface as well as some helper methods for leveled logging.
type Logger struct {
	Writer    io.Writer
	Formatter Formatter
	context   []interface{}
	logMutex  *sync.Mutex

	FilterLevel int
}

// Log the provided key value pairs
func (l *Logger) log(keyvals ...interface{}) error {
	if l.logMutex == nil {
		l.logMutex = new(sync.Mutex)
	}

	l.logMutex.Lock()
	defer l.logMutex.Unlock()

	var writer io.Writer
	if l.Writer == nil {
		writer = os.Stdout
	} else {
		writer = l.Writer
	}

	var formatter Formatter
	if l.Formatter == nil {
		formatter = JSONFormatter{}
	} else {
		formatter = l.Formatter
	}

	var err = formatter.Format(writer, append(l.context, keyvals...)...)
	return err
}

// With returns a copy of the logger with the provided key-value pairs to the backing context
func (l Logger) With(keyvals ...interface{}) Logger {
	return Logger{
		Writer:    l.Writer,
		Formatter: l.Formatter,
		context:   append(l.context, keyvals...),
	}
}

// Emergency logs the provided key value pairs, adding the emergency log level
func (l *Logger) Emergency(message string, keyvals ...interface{}) error {
	return l.log(append([]interface{}{"level", "emergency", "msg", message}, keyvals...)...)
}

// Alert logs the provided key value pairs, adding the alert log level
func (l *Logger) Alert(message string, keyvals ...interface{}) error {
	if l.FilterLevel > 6 {
		return ErrIgnoredLogLevel
	}

	return l.log(append([]interface{}{"level", "alert", "msg", message}, keyvals...)...)
}

// Critical logs the provided key value pairs, adding the critical log level
func (l *Logger) Critical(message string, keyvals ...interface{}) error {
	if l.FilterLevel > 5 {
		return ErrIgnoredLogLevel
	}

	return l.log(append([]interface{}{"level", "critical", "msg", message}, keyvals...)...)
}

// Error logs the provided key value pairs, adding the error log level
func (l *Logger) Error(message string, keyvals ...interface{}) error {
	if l.FilterLevel > 4 {
		return ErrIgnoredLogLevel
	}

	return l.log(append([]interface{}{"level", "error", "msg", message}, keyvals...)...)
}

// Warning logs the provided key value pairs, adding the warning log level
func (l *Logger) Warning(message string, keyvals ...interface{}) error {
	if l.FilterLevel > 3 {
		return ErrIgnoredLogLevel
	}

	return l.log(append([]interface{}{"level", "warning", "msg", message}, keyvals...)...)
}

// Notice logs the provided key value pairs, adding the notice log level
func (l *Logger) Notice(message string, keyvals ...interface{}) error {
	if l.FilterLevel > 2 {
		return ErrIgnoredLogLevel
	}

	return l.log(append([]interface{}{"level", "notice", "msg", message}, keyvals...)...)
}

// Info logs the provided key value pairs, adding the info log level
func (l *Logger) Info(message string, keyvals ...interface{}) error {
	if l.FilterLevel > 1 {
		return ErrIgnoredLogLevel
	}

	return l.log(append([]interface{}{"level", "info", "msg", message}, keyvals...)...)
}

// Debug logs the provided key value pairs, adding the debug log level
func (l *Logger) Debug(message string, keyvals ...interface{}) error {
	if l.FilterLevel > 0 {
		return ErrIgnoredLogLevel
	}

	return l.log(append([]interface{}{"level", "debug", "msg", message}, keyvals...)...)
}
