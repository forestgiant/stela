package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/forestgiant/stela"
	"github.com/forestgiant/stela/api"
	"golang.org/x/net/context"
)

func Test_valid(t *testing.T) {
	stelaClient, err := api.NewClient(context.TODO(), stela.DefaultStelaAddress, "")
	if err != nil {
		t.Fatal("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
	}
	defer stelaClient.Close()

	var tests = []struct {
		w        *watcher
		expected bool
	}{
		{&watcher{}, false},
		{&watcher{interval: time.Duration(1 * time.Millisecond)}, false},
		{&watcher{
			interval: time.Duration(1 * time.Millisecond),
			service:  &stela.Service{},
		}, false},
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Name: "test.service.fg", Port: 9000},
			stelaClient: stelaClient,
		}, false},
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Address: "127.0.0.1", Port: 9000},
			stelaClient: stelaClient,
		}, false},
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Name: "test.service.fg", Address: "127.0.0.1", Port: 9000},
			stelaClient: stelaClient,
		}, true},
	}
	for _, test := range tests {
		valid := test.w.valid()
		if valid != test.expected {
			t.Fatalf("Wanted: %t, Got: %t", test.expected, valid)
		}
	}
}

func Test_updateRegistry(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	stelaClient, err := api.NewClient(ctx, stela.DefaultStelaAddress, "")
	if err != nil {
		t.Fatal("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
	}
	defer stelaClient.Close()

	var tests = []struct {
		w              *watcher
		shouldRegister bool
	}{
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Name: "test_updateRegistry1.service.fg", Address: "127.0.0.1", Port: stela.DefaultStelaPort},
			stelaClient: stelaClient,
		}, true},
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Name: "test_updateRegistry2.service.fg", Address: "noaddress.fg", Port: 80},
			stelaClient: stelaClient,
		}, false},
	}

	for _, test := range tests {
		got := test.w.updateRegistry()
		if got != test.shouldRegister {
			t.Fatalf("updateRegistry failed for watcher: %s. Got: %t, Wanted: %t", *test.w, got, test.shouldRegister)
		}
	}
}

func Test_watch(t *testing.T) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()
	stelaClient, err := api.NewClient(ctx, stela.DefaultStelaAddress, "")
	if err != nil {
		t.Fatal("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
	}
	defer stelaClient.Close()

	var tests = []struct {
		w          *watcher
		shouldFail bool
	}{
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Address: "127.0.0.1", Port: 20},
			stelaClient: stelaClient,
		}, true},
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Address: "127.0.0.1", Port: 100},
			stelaClient: stelaClient,
		}, true},
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Name: "Test_watch.service.fg", Address: "127.0.0.1", Port: 5000},
			stelaClient: stelaClient,
		}, false},
		{&watcher{
			interval:    time.Duration(1 * time.Millisecond),
			service:     &stela.Service{Name: "Test_watch_stela.service.fg", Address: "127.0.0.1", Port: 9000},
			stelaClient: stelaClient,
		}, false},
	}

	for _, test := range tests {
		if (err != test.w.watch()) != test.shouldFail {
			t.Fatal("watch should have failed")
		}

		// give each watcher time to tick once then stop
		time.Sleep(test.w.interval * 2)

		test.w.stop()
	}
}

func TestString(t *testing.T) {
	var tests = []struct {
		w *watcher
		s string
	}{
		{&watcher{
			interval: time.Duration(1000 * time.Millisecond),
			service:  &stela.Service{Address: "127.0.0.1", Port: 9000},
		}, `Name: , Address: 127.0.0.1, Port: 9000, Interval: 1s`},
		{&watcher{
			interval: time.Duration(2000 * time.Millisecond),
			service:  &stela.Service{Name: "test.service.fg", Address: "noaddress", Port: 80},
		}, `Name: test.service.fg, Address: noaddress, Port: 80, Interval: 2s`},
		{&watcher{
			interval: time.Duration(3000 * time.Millisecond),
			service:  &stela.Service{Name: "test.service.fg", Address: "127.0.0.1", Port: 9000},
		}, `Name: test.service.fg, Address: 127.0.0.1, Port: 9000, Interval: 3s`},
	}

	for _, test := range tests {
		s := fmt.Sprint(test.w)
		if s != test.s {
			t.Fatalf("Stringer failed. Got: %s, Wanted: %s", s, test.s)
		}
	}
}
