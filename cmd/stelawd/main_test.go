package main

import (
	"reflect"
	"testing"
	"time"

	"golang.org/x/net/context"

	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/api"
)

func Test_createWatchers(t *testing.T) {
	stelaClient, err := api.NewClient(context.TODO(), stela.DefaultStelaAddress, "")
	if err != nil {
		t.Fatal("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
	}
	defer stelaClient.Close()

	var tests = []struct {
		filePath string
		expected []*watcher
	}{
		{"testfiles/test.list", []*watcher{
			&watcher{
				service: &stela.Service{
					Name:    "minio.service.fg",
					Address: "play.minio.io",
					Port:    8000,
				},
				interval:    time.Duration(1000 * time.Millisecond),
				stelaClient: stelaClient,
			},
		}},
	}

	for _, test := range tests {
		w, err := createWatchers(stelaClient, test.filePath)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(w, test.expected) {
			t.Fatal("Watchers not equal")
		}
	}
}
