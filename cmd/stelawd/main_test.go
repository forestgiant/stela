package main

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/api"
	"golang.org/x/net/context"
)

func createTestConfigFile(t *testing.T, content string) *os.File {
	c := []byte(content)
	tmpFile, err := ioutil.TempFile("", "testConfig.list")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tmpFile.Write(c); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	return tmpFile
}

func Test_openConfig(t *testing.T) {
	succesFile := createTestConfigFile(t, `# Service 1
minio.service.fg, play.minio.io:8000, 1000`)
	defer os.Remove(succesFile.Name())

	var tests = []struct {
		filePath   string
		shouldFail bool
	}{
		{"", true},
		{succesFile.Name(), false},
	}

	for _, test := range tests {
		_, err := openConfig(test.filePath)
		if (err != nil) != test.shouldFail {
			t.Fatal(err)
		}
	}
}

func Test_createWatchers(t *testing.T) {

	stelaClient, err := api.NewClient(context.TODO(), stela.DefaultStelaAddress, "")
	if err != nil {
		t.Fatal("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
	}
	defer stelaClient.Close()

	// definde test config
	successConfig := `# Service 1
minio.service.fg, play.minio.io:8000, 1000, success value`

	failConfigService := `# Service 2
, play.minio.io:8000, 1000, value test`

	failConfigAddress := `# Service 3
minio.service.fg, , 1000, value test`

	failConfigInterval := `# Service 4
minio.service.fg, play.minio.io:8000, value test`

	failConfigAtoi := `# Service 5
minio.service.fg, play.minio.io:NaN, 9000, value test`

	failConfigValue := `# Service 1
minio.service.fg, play.minio.io:8000, 1000, `

	var tests = []struct {
		stelaClient *api.Client
		config      string
		expected    []*watcher
		shouldFail  bool
	}{
		{stelaClient, failConfigService, nil, true},
		{stelaClient, failConfigAddress, nil, true},
		{stelaClient, failConfigInterval, nil, true},
		{stelaClient, failConfigAtoi, nil, true},
		{stelaClient, failConfigValue, nil, true},
		{nil, successConfig, nil, true},
		{stelaClient, successConfig,
			[]*watcher{
				&watcher{
					service: &stela.Service{
						Name:    "minio.service.fg",
						Address: "play.minio.io",
						Port:    8000,
						Value:   "success value",
					},
					interval:    time.Duration(1000 * time.Millisecond),
					stelaClient: stelaClient,
				},
			}, false},
	}

	for i, test := range tests {
		config := strings.NewReader(test.config)
		w, err := createWatchers(test.stelaClient, config)
		if (err != nil) != test.shouldFail {
			t.Fatalf("Test %d failed: %v", i, err)
		}

		if !reflect.DeepEqual(w, test.expected) {
			t.Fatalf("Watchers not equal. Got: %v, Wanted: %v", w, test.expected)
		}
	}
}
