package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestLog(t *testing.T) {
	var buffer = &bytes.Buffer{}
	var key = "key"
	var value = "value"
	var message = "test message"
	var contextKey = "context"
	var contextValue = "present"

	var validateJSONLogger = func(b []byte, message string, key string, value string) error {
		var object map[string]interface{}
		if err := json.Unmarshal(b, &object); err != nil {
			return err
		}

		if object[key] != value {
			return fmt.Errorf("Failed to properly log value for key %s. Expected %s but received %s", key, value, object[key])
		}

		if object[contextKey] != contextValue {
			return fmt.Errorf("Failed to properly log value for key %s. Expected %s but received %s", contextKey, contextValue, object[contextKey])
		}

		return nil
	}

	var validateLogfmtLogger = func(b []byte, message string, key string, value string) error {
		if string(b) != fmt.Sprintf("%s=%s msg=\"%s\" %s=%s\n", contextKey, contextValue, message, key, value) {
			return errors.New("ValueGenerator was not properly evaluated for LogfmtFormatter")
		}
		return nil
	}

	var tests = []struct {
		logger    Logger
		validator func(b []byte, message string, key string, value string) error
	}{
		{Logger{Writer: buffer}.With(contextKey, contextValue), validateJSONLogger},
		{Logger{Writer: buffer, Formatter: JSONFormatter{}}.With(contextKey, contextValue), validateJSONLogger},
		{Logger{Writer: buffer, Formatter: LogfmtFormatter{}}.With(contextKey, contextValue), validateLogfmtLogger},
	}

	for _, test := range tests {
		err := test.logger.log("msg", message, key, value)
		if err != nil {
			t.Error(err)
			return
		}

		if err = test.validator(buffer.Bytes(), message, key, value); err != nil {
			t.Error(err)
			return
		}

		buffer.Reset()
	}
}

func TestConcurrentLogUsage(t *testing.T) {
	var buffer = &bytes.Buffer{}
	var logger = Logger{Writer: buffer}

	var key = "key"
	var value = "value"
	var message = "test message"

	for i := 0; i < 100; i++ {
		go func() {
			err := logger.log("msg", message, key, value)
			if err != nil {
				t.Error(err)
				return
			}
		}()
	}
}

func TestLevels(t *testing.T) {
	var buffer = &bytes.Buffer{}
	var message = "Test message"
	var key = "key"
	var value = "value"

	var validateJSONLogger = func(b []byte, level string, message string, key string, value string) error {
		var object map[string]interface{}
		if err := json.Unmarshal(b, &object); err != nil {
			return err
		}

		if object[key] != value {
			return fmt.Errorf("Expected logged value %s for key %s, but received %s", value, key, object[key])
		}

		if object["level"] != level {
			return fmt.Errorf("Expected logged value %s for key %s, but received %s", level, "level", object["level"])
		}

		if object["msg"] != message {
			return fmt.Errorf("Expected logged value %s for key %s, but received %s", level, "msg", object["msg"])
		}

		return nil
	}

	var validateLogfmtLogger = func(b []byte, level string, message string, key string, value string) error {
		if string(b) != fmt.Sprintf("level=%s msg=\"%s\" %s=%s\n", level, message, key, value) {
			return errors.New("ValueGenerator was not properly evaluated for LogfmtFormatter")
		}
		return nil
	}

	for _, lv := range []struct {
		logger   Logger
		validate func(b []byte, level string, message string, key string, value string) error
	}{
		{Logger{Writer: buffer, Formatter: JSONFormatter{}}, validateJSONLogger},
		{Logger{Writer: buffer, Formatter: LogfmtFormatter{}}, validateLogfmtLogger},
	} {
		var tests = []struct {
			level string
			f     func(string, ...interface{}) error
		}{
			{"debug", lv.logger.Debug},
			{"info", lv.logger.Info},
			{"emergency", lv.logger.Emergency},
			{"alert", lv.logger.Alert},
			{"critical", lv.logger.Critical},
			{"error", lv.logger.Error},
			{"notice", lv.logger.Notice},
			{"warning", lv.logger.Warning},
		}

		for _, test := range tests {
			err := test.f(message, key, value)
			if err != nil {
				t.Error(err)
				return
			}

			if err = lv.validate(buffer.Bytes(), test.level, message, key, value); err != nil {
				t.Error(err)
				return
			}

			buffer.Reset()
		}
	}
}

type logFunc func(string, ...interface{}) error

func TestIgnoreLevels(t *testing.T) {
	var buffer = &bytes.Buffer{}
	var logger = Logger{Writer: buffer, FilterLevel: 0}

	for filter := 0; filter < 8; filter++ {
		logger.FilterLevel = filter

		var functions = []logFunc{
			logger.Emergency,
			logger.Alert,
			logger.Critical,
			logger.Error,
			logger.Warning,
			logger.Notice,
			logger.Info,
			logger.Debug,
		}

		for findex, f := range functions {
			err := f("test message", "key", "value")

			if findex == 0 && err == nil {
				continue
			}

			if err != ErrIgnoredLogLevel && logger.FilterLevel > 7-findex {
				t.Errorf("Failed to ignore log at level %d when filter level is set to %d", findex, logger.FilterLevel)
				continue
			}
		}
	}
}
