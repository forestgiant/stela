package log

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultTimestamp(t *testing.T) {
	ts, ok := DefaultTimestamp().(string)
	if !ok {
		t.Error("DefaultTimestamp did not return a value of type string as expected.")
		return
	}

	_, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t.Error(err)
		return
	}
}

func TestCaller(t *testing.T) {
	var tests = []struct {
		vg               ValueGenerator
		expectedFileName string
	}{
		{Caller(0), "value.go"},
		{Caller(1), "value_test.go"},
		{Caller(2), "testing.go"},
	}

	for _, test := range tests {
		callerString, ok := test.vg().(string)
		if !ok {
			t.Error("Caller did not return a value of type string as expected.")
			return
		}

		splits := strings.Split(callerString, ":")
		if len(splits) < 2 {
			t.Error("Caller did not return a value of the expected format.")
			return
		}

		if splits[0] != test.expectedFileName {
			t.Errorf("Caller did not return the expected file name.  Found %s but expected %s.\n", splits[0], test.expectedFileName)
			return
		}
	}
}
