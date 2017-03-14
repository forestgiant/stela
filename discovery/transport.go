package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"gitlab.fg/go/stela"

	"github.com/forestgiant/raftutil"
	"github.com/hashicorp/raft"
	"github.com/miekg/dns"
)

const (
	raftTimeout = 10 * time.Second
)

// Create customMux to support CORS
type customMux struct {
	*http.ServeMux
}

// NewCustomMux return a customMux with an embedded http.ServeMux
func newCustomMux() *customMux {
	m := new(customMux)
	m.ServeMux = http.NewServeMux()

	return m
}

func (m *customMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if o := r.Header.Get("Origin"); o != "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}

	// // Don't serve if it only needs the options
	// if r.Method == "OPTIONS" {
	// 	return
	// }

	m.ServeMux.ServeHTTP(w, r)
}

func handleVerify(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := svc.WaitForLeader(); err != nil {
			http.Error(w, "Error verifying: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

func handleRoundRobin(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create the service from the JSON
		s := new(stela.Service)
		err = json.Unmarshal(body, s)
		if err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Call the business logic of the service to register
		err = svc.RotateSRVPriorities(s.Name, false)
		if err != nil {
			http.Error(w, "Error loadbalancing service: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

// Raft Join handler
func handleJoin(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First see who the raft leader is and if it's not us then proxy
		proxyResponse, err := proxyToRaftLeader(svc.(*discoveryService).raft, r)
		if err == nil {
			w.WriteHeader(proxyResponse.StatusCode)
			return
		}

		m := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "Join error decoding JSON: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if len(m) != 1 {
			http.Error(w, "Join error no services sent", http.StatusInternalServerError)
			return
		}

		remoteAddr, ok := m["addr"]
		if !ok {
			http.Error(w, "Join error addr key doesn't have a value", http.StatusInternalServerError)
			return
		}

		if err := svc.Join(remoteAddr); err != nil {
			http.Error(w, "Join failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// Raft remove handler
func handleRemove(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First see who the raft leader is and if it's not us then proxy
		proxyResponse, err := proxyToRaftLeader(svc.(*discoveryService).raft, r)
		if err == nil {
			w.WriteHeader(proxyResponse.StatusCode)
			return
		}

		m := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if len(m) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		remoteAddr, ok := m["addr"]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := svc.Remove(remoteAddr); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}

func handleRegisterService(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First see who the raft leader is and if it's not us then proxy
		proxyResponse, err := proxyToRaftLeader(svc.(*discoveryService).raft, r)
		if err == nil {
			if proxyResponse.StatusCode != http.StatusOK {
				http.Error(w, "Error proxying to leader", proxyResponse.StatusCode)
				return
			}
			body, err := ioutil.ReadAll(proxyResponse.Body)
			proxyResponse.Body.Close()
			if err != nil {
				http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create the service from the JSON
		s := &stela.Service{}
		if err := json.Unmarshal(body, s); err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create command to pass to raft
		c := &raftCommand{
			Operation: registerOperation,
			Service:   s,
		}
		cmd, err := json.Marshal(c)
		if err != nil {
			http.Error(w, "Error registering service: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Set the raft apply and it will call svc.Register when consensus is met
		ds := svc.(*discoveryService)
		raftFuture := ds.raft.Apply(cmd, raftTimeout)
		if err := raftFuture.(raft.Future); err.Error() != nil {
			http.Error(w, "Error registering service: "+err.Error().Error(), http.StatusInternalServerError)
			return
		}

		// TODO: Use logging middelware
		fsmResponse := raftFuture.Response().(*fsmResponse)
		if fsmResponse.error != nil {
			fmt.Println("Error registering services: " + fsmResponse.error.Error())
			http.Error(w, "Error registering services: "+fsmResponse.error.Error(), http.StatusInternalServerError)
			return
		}

		// respond with echo of service registered
		jsonResponse, err := json.Marshal(fsmResponse.result)
		if err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
	})
}

func handleDeregisterService(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First see who the raft leader is and if it's not us then proxy
		proxyResponse, err := proxyToRaftLeader(svc.(*discoveryService).raft, r)
		if err == nil {
			if proxyResponse.StatusCode != http.StatusOK {
				http.Error(w, "Error proxying to leader", proxyResponse.StatusCode)
				return
			}
			body, err := ioutil.ReadAll(proxyResponse.Body)
			proxyResponse.Body.Close()
			if err != nil {
				http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create the service from the JSON
		s := new(stela.Service)
		err = json.Unmarshal(body, s)
		if err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create command to pass to raft
		c := new(raftCommand)
		c.Operation = deregisterOperation
		c.Service = s
		cmd, err := json.Marshal(c)
		if err != nil {
			http.Error(w, "Error deregistering service: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Set the raft apply and it will call svc.Register when consensus is met
		ds := svc.(*discoveryService)
		raftFuture := ds.raft.Apply(cmd, raftTimeout)
		if err := raftFuture.Error(); err != nil {
			http.Error(w, "Error registering service: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO: Use logging middelware
		fsmResponse := raftFuture.Response().(*fsmResponse)
		if fsmResponse.error != nil {
			fmt.Println("Error deregistering service: " + fsmResponse.error.Error())
			http.Error(w, "Error deregistering services: "+fsmResponse.error.Error(), http.StatusInternalServerError)
			return
		}

		// respond with echo of service registered
		jsonResponse, err := json.Marshal(fsmResponse.result)
		if err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
	})
}

func handleDiscover(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First see who the raft leader is and if it's not us then proxy
		proxyResponse, err := proxyToRaftLeader(svc.(*discoveryService).raft, r)
		if err == nil {
			if proxyResponse.StatusCode != http.StatusOK {
				http.Error(w, "Error proxying to leader", proxyResponse.StatusCode)
				return
			}
			body, err := ioutil.ReadAll(proxyResponse.Body)
			proxyResponse.Body.Close()
			if err != nil {
				http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create the service from the JSON
		s := new(stela.Service)
		err = json.Unmarshal(body, s)
		if err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create command to pass to raft
		c := new(raftCommand)
		c.Operation = discoverOperation
		c.Service = s
		cmd, err := json.Marshal(c)
		if err != nil {
			http.Error(w, "Error discovering services: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Set the raft apply and it will call svc.Discover when consensus is met
		ds := svc.(*discoveryService)
		raftFuture := ds.raft.Apply(cmd, raftTimeout)
		if err := raftFuture.Error(); err != nil {
			http.Error(w, "Error discovering services: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO: Use logging middelware
		discoverResponse := raftFuture.Response().(*fsmDiscoverResponse)
		if discoverResponse.error != nil {
			fmt.Println("Error discovering services: " + discoverResponse.error.Error())
			http.Error(w, "Error discovering services: "+discoverResponse.error.Error(), http.StatusInternalServerError)
			return
		}

		// respond with echo of service registered
		jsonResponse, err := json.Marshal(discoverResponse.results)
		if err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
	})
}

func handleDiscoverOne(svc DiscoveryService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First see who the raft leader is and if it's not us then proxy
		proxyResponse, err := proxyToRaftLeader(svc.(*discoveryService).raft, r)
		if err == nil {
			if proxyResponse.StatusCode != http.StatusOK {
				http.Error(w, "Error proxying to leader", proxyResponse.StatusCode)
				return
			}
			body, err := ioutil.ReadAll(proxyResponse.Body)
			proxyResponse.Body.Close()
			if err != nil {
				http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error parsing request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create the service from the JSON
		s := &stela.Service{}
		if err = json.Unmarshal(body, s); err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Create command to pass to raft
		c := new(raftCommand)
		c.Operation = discoverOneOperation
		c.Service = s
		cmd, err := json.Marshal(c)
		if err != nil {
			http.Error(w, "Error discoverOne service: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Set the raft apply and it will call svc.DiscoverOne when consensus is met
		ds := svc.(*discoveryService)
		raftFuture := ds.raft.Apply(cmd, raftTimeout)
		if err := raftFuture.Error(); err != nil {
			http.Error(w, "Error discoverOne service: "+err.Error(), http.StatusInternalServerError)
			return
		}

		discoverResponse := raftFuture.Response().(*fsmResponse)
		if discoverResponse.error != nil {
			fmt.Println("Error discovering one service: " + discoverResponse.error.Error())
			http.Error(w, "Error discovering one service: "+discoverResponse.error.Error(), http.StatusInternalServerError)
			return
		}

		// respond with echo of service registered
		jsonResponse, err := json.Marshal(discoverResponse.result)
		if err != nil {
			http.Error(w, "Error unmarshalling json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonResponse)
	})
}

// Miekg DNS http transport
func handleDNSRequest(svc DiscoveryService) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)

		switch r.Opcode {
		case dns.OpcodeQuery:
			err := svc.ParseDNSQuery(m)
			if err != nil {
				fmt.Println(err)
			}

			// TODO: Use logging middelware
			// fmt.Println("DNS Lookup", r.Question)

		default:
			fmt.Println("dns request with opcode:", r.Opcode)
		}

		w.WriteMsg(m)
	}
}

func proxyToRaftLeader(r *raft.Raft, req *http.Request) (*http.Response, error) {
	// Test to see if we need to proxy to leader
	// Verify Leader and proxy if it isn't
	f := r.VerifyLeader()
	err := f.Error()

	if err == raft.ErrNotLeader {
		// Use raftutil to proxy to raft leader
		_, raftPort, err := net.SplitHostPort(r.Leader())
		if err != nil {
			return nil, err
		}

		// Now get the stela port to send to proxy
		stelaPort, err := stelaPortFromRaft(raftPort)
		if err != nil {
			return nil, err
		}

		// set URL Scheme & host
		req.URL.Scheme = "http"

		response, err := raftutil.ProxyToLeader(r, stelaPort, req, http.DefaultClient)
		if err != nil {
			return nil, err
		}

		return response, nil
	}

	return nil, errors.New("Couldn't proxy to raft leader")
}
