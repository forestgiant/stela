package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestUnevenArguments(t *testing.T) {
	var tests = []struct {
		formatter Formatter
		expected  string
	}{
		{JSONFormatter{}, "{}\n"},
		{LogfmtFormatter{}, "\n"},
	}

	w := &bytes.Buffer{}
	for _, test := range tests {
		err := test.formatter.Format(w, "uneven")
		if err != nil {
			t.Error(err)
		}

		if w.String() != test.expected {
			t.Error("Failed to ignore key without a corresponding value")
		}

		w.Reset()
	}
}

func TestJSONNonStringKey(t *testing.T) {
	formatter := JSONFormatter{}
	w := &bytes.Buffer{}
	err := formatter.Format(w, 1, "value")
	if err == nil {
		t.Error("It is expected that non-string keys will throw an error.")
	}
}

func TestNilKey(t *testing.T) {
	var tests = []struct {
		formatter Formatter
		expected  string
	}{
		{JSONFormatter{}, "{}\n"},
		{LogfmtFormatter{}, "\n"},
	}

	w := &bytes.Buffer{}
	for _, test := range tests {
		err := test.formatter.Format(w, nil, "value")
		if err == nil {
			t.Error("It is expected that nil keys will throw an error.")
		}
		w.Reset()
	}
}

func TestValueGenerator(t *testing.T) {
	key := "key"
	value := "generator"
	var generator ValueGenerator = func() interface{} {
		return value
	}

	var validateJSON = func(b []byte) error {
		var object map[string]interface{}
		if err := json.Unmarshal(b, &object); err != nil {
			return err
		}

		if object[key] != value {
			return errors.New("ValueGenerator was not properly evaluated for JSONFormatter")
		}

		return nil
	}

	var validateLogfmt = func(b []byte) error {
		if string(b) != fmt.Sprintf("%s=%s\n", key, value) {
			return errors.New("ValueGenerator was not properly evaluated for LogfmtFormatter")
		}
		return nil
	}

	var tests = []struct {
		formatter Formatter
		expected  string
		validator func([]byte) error
	}{
		{JSONFormatter{}, "{}\n", validateJSON},
		{LogfmtFormatter{}, "\n", validateLogfmt},
	}

	w := &bytes.Buffer{}
	for _, test := range tests {
		err := test.formatter.Format(w, key, generator)
		if err != nil {
			t.Error(err)
		}

		if err = test.validator(w.Bytes()); err != nil {
			t.Error(err)
		}

		w.Reset()
	}
}
