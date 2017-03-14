package semver

import (
	"flag"
	"fmt"
	"testing"
)

func TestNewVersion(t *testing.T) {

	tests := []struct {
		version string
		pass    bool
	}{
		{"0.0.4", true},
		{"1.0.4", true},
		{"4.4.4", true},
		{"7.0.7", true},
		{"alpha1.0.4", false},
		{"v4.4.4", false},
		{"7.0.beta2", false},
	}

	for _, test := range tests {
		versionStr := test.version

		v, err := NewVersion(versionStr)

		// Don't test if the test isn't suppose to pass
		if test.pass {
			if err != nil {
				t.Fatalf("error for version %s", err)
			}

			// Make sure the supplied version string and the return one match
			if versionStr != fmt.Sprint(v) {
				t.Fatalf("Version string is: %s, should be: %s", fmt.Sprint(v), versionStr)
			}
		} else {
			if err == nil {
				t.Fatalf("version should error %s", err)
			}
		}
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		s1   string
		s2   string
		pass bool
	}{
		{"0.0.4", "0.0.4", true},
		{"1.0.4", "1.0.4", true},
		{"4.4.4", "4.4.4", true},
		{"7.0.7", "7.0.7", true},
		{"3.0.4", "3.0.3", false},
		{"3.0.4", "3.1.4", false},
		{"3.0.4", "4.0.4", false},
	}

	for _, test := range tests {
		if Equal(test.s1, test.s2) != test.pass {
			t.Fatalf("s1: %s, s1: %s, pass: %t", test.s1, test.s2, test.pass)
		}
	}
}

func TestSetVersion(t *testing.T) {
	var (
		testUsage = "Test Usage"
		testPtr   = flag.Bool("test", false, testUsage)
	)

	// Set up short hand flags
	flag.BoolVar(testPtr, "t", false, testUsage+" (shorthand)")

	SetVersion("0.0.4")

}
