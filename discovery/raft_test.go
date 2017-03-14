package discovery

import (
	"encoding/json"
	"testing"

	"gitlab.fg/go/stela"

	"github.com/hashicorp/raft"
)

func TestApply(t *testing.T) {
	applyTest := func(s *stela.Service, operation raftOperation, shouldError bool) {
		// Setup log raft
		// Create command to pass to raft
		c := new(raftCommand)
		c.Operation = registerOperation
		c.Service = s

		cmd, err := json.Marshal(c)
		if err != nil {
			t.Error(err)
		}

		raftLog := new(raft.Log)
		raftLog.Data = cmd

		ds := d.(*discoveryService)

		// Set the raft apply and it will call svc.Register when consensus is met
		response := ds.Apply(raftLog)
		if response == nil {
			t.Error("TestApplyRegister: Response is nil")
		}

		// Check if the response is an error
		if _, ok := response.(error); ok {
			if !shouldError {
				t.Error(response)
			}
		} else {
			if shouldError {
				t.Error("TestApplyRegister: Should have errored")
			}
		}
	}

	// Test services
	for _, testServices := range srvMap {
		for _, testService := range testServices {
			applyTest(testService, registerOperation, false)
			applyTest(testService, deregisterOperation, false)
		}
	}

	// Test Nil service
	applyTest(nil, registerOperation, true)
}
