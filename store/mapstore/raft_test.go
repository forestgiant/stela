package mapstore

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestOpen(t *testing.T) {
	// Create test DB file in temp folder
	raftDir, err := tempDir()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(raftDir)

	m := MapStore{
		RaftDir:  raftDir,
		RaftAddr: "127.0.0.1",
	}
	m.Open(true)
}

func tempDir() (string, error) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", err
	}

	return dir, nil
}
