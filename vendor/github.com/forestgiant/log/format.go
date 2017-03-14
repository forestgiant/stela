package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/go-logfmt/logfmt"
)

// Formatter is a simple interface for formatting messages written to an io.Writer
type Formatter interface {
	Format(writer io.Writer, keyvals ...interface{}) error
}

// JSONFormatter implements the LogFormatter interface for JSON outpiut
type JSONFormatter struct{}

// Format will write the provided key-value pairs to the specified io.Writer in JSON format
func (f JSONFormatter) Format(writer io.Writer, keyvals ...interface{}) error {
	n := len(keyvals)
	m := make(map[string]interface{})
	for i := 0; i < n; i += 2 {
		if i+1 >= n {
			break
		}

		k, ok := keyvals[i].(string)
		if !ok {
			return errors.New("Could not Marshal key-value pairs to JSON")
		}

		vg, ok := keyvals[i+1].(ValueGenerator)
		if ok {
			m[k] = vg()
		} else {
			m[k] = keyvals[i+1]
		}
	}

	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return err
	}

	outputString := fmt.Sprintf("%s", buf.Bytes())
	_, err := writer.Write([]byte(outputString))
	return err
}

// LogfmtFormatter implements the LogFormatter interface for Logfmt outpiut
type LogfmtFormatter struct{}

// Format will write the provided key-value pairs to the specified io.Writer in Logfmt format
func (f LogfmtFormatter) Format(writer io.Writer, keyvals ...interface{}) error {
	var e = logfmt.NewEncoder(writer)
	n := len(keyvals)
	for i := 0; i < n; i += 2 {
		if i+1 >= n {
			break
		}

		var value interface{}
		if vg, ok := keyvals[i+1].(ValueGenerator); ok {
			value = vg()
		} else {
			value = keyvals[i+1]
		}

		if errorEncoding := e.EncodeKeyval(keyvals[i], value); errorEncoding != nil {
			return errorEncoding
		}
	}
	return e.EndRecord()
}
