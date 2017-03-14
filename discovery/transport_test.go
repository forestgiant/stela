package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"gitlab.fg/go/stela"
)

var serveMuxRegister = []struct {
	pattern string
	h       http.Handler
}{
	{"/dir/", serve(200)},
	{"/search", serve(201)},
	{"codesearch.google.com/search", serve(202)},
	{"codesearch.google.com/", serve(203)},
	{"example.com/", http.HandlerFunc(checkQueryStringHandler)},
}

var serveMuxTests = []struct {
	method  string
	host    string
	path    string
	code    int
	pattern string
}{
	{"GET", "google.com", "/", 404, ""},
	{"GET", "google.com", "/dir", 301, "/dir/"},
	{"GET", "google.com", "/dir/", 200, "/dir/"},
	{"GET", "google.com", "/dir/file", 200, "/dir/"},
	{"GET", "google.com", "/search", 201, "/search"},
	{"GET", "google.com", "/search/", 404, ""},
	{"GET", "google.com", "/search/foo", 404, ""},
	{"GET", "codesearch.google.com", "/search", 202, "codesearch.google.com/search"},
	{"GET", "codesearch.google.com", "/search/", 203, "codesearch.google.com/"},
	{"GET", "codesearch.google.com", "/search/foo", 203, "codesearch.google.com/"},
	{"GET", "codesearch.google.com", "/", 203, "codesearch.google.com/"},
	{"GET", "images.google.com", "/search", 201, "/search"},
	{"GET", "images.google.com", "/search/", 404, ""},
	{"GET", "images.google.com", "/search/foo", 404, ""},
	{"GET", "google.com", "/../search", 301, "/search"},
	{"GET", "google.com", "/dir/..", 301, ""},
	{"GET", "google.com", "/dir/..", 301, ""},
	{"GET", "google.com", "/dir/./file", 301, "/dir/"},

	// The /foo -> /foo/ redirect applies to CONNECT requests
	// but the path canonicalization does not.
	{"CONNECT", "google.com", "/dir", 301, "/dir/"},
	{"CONNECT", "google.com", "/../search", 404, ""},
	{"CONNECT", "google.com", "/dir/..", 200, "/dir/"},
	{"CONNECT", "google.com", "/dir/..", 200, "/dir/"},
	{"CONNECT", "google.com", "/dir/./file", 200, "/dir/"},
}

// checkQueryStringHandler checks if r.URL.RawQuery has the same value
// as the URL excluding the scheme and the query string and sends 200
// response code if it is, 500 otherwise.
func checkQueryStringHandler(w http.ResponseWriter, r *http.Request) {
	u := *r.URL
	u.Scheme = "http"
	u.Host = r.Host
	u.RawQuery = ""
	if "http://"+r.URL.RawQuery == u.String() {
		w.WriteHeader(200)
	} else {
		w.WriteHeader(500)
	}
}

// serve returns a handler that sends a response with the given code.
func serve(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}
}

// https://golang.org/src/net/http/serve_test.go
func TestCustomMux(t *testing.T) {
	mux := newCustomMux()
	for _, e := range serveMuxRegister {
		mux.Handle(e.pattern, e.h)
	}

	for _, tt := range serveMuxTests {
		r := &http.Request{
			Method: tt.method,
			Host:   tt.host,
			URL: &url.URL{
				Path: tt.path,
			},
		}
		_, pattern := mux.Handler(r)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, r)
		if pattern != tt.pattern || rr.Code != tt.code {
			t.Errorf("%s %s %s = %d, %q, want %d, %q", tt.method, tt.host, tt.path, rr.Code, pattern, tt.code, tt.pattern)
		}
	}
}

func TestHandleRegisterService(t *testing.T) {
	fmt.Println("TestHandleRegisterService")

	// Post Service to handler
	s := new(stela.Service)
	s.Name = "test.service.fg"
	s.Target = "server.local"
	s.Address = "10.0.0.99"
	s.Port = 8010

	registerHandle := handleRegisterService(d)
	jsonStr, _ := json.Marshal(s)
	req, _ := http.NewRequest("POST", "", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	registerHandle.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Register handler didn't return %v", http.StatusOK)
	}
}

func TestHandleDeregisterService(t *testing.T) {
	fmt.Println("TestHandleDeregisterService")

	// Post Service to handler
	s := new(stela.Service)
	s.Name = "test.service.fg"
	s.Target = "server.local"
	s.Address = "10.0.0.99"
	s.Port = 8010

	registerHandle := handleDeregisterService(d)
	jsonStr, _ := json.Marshal(s)
	req, _ := http.NewRequest("POST", "", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	registerHandle.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Deregister handler didn't return %v", http.StatusOK)
	}
}
