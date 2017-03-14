package discovery

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"

	"gitlab.fg/go/stela"

	"github.com/forestgiant/sliceutil"
	"github.com/hashicorp/raft"
	"github.com/miekg/dns"
)

var (
	d      DiscoveryService
	srvMap map[string][]*stela.Service
)

func TestMain(m *testing.M) {
	// Create test DB file in temp folder
	dbDir, err := dirToTestDB()
	if err != nil {
		fmt.Println("Testing:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dbDir)

	// Create Discovery Service used for all test
	config := raft.DefaultConfig()
	config.StartAsLeader = true
	d, err = NewDiscoveryService("peer_stela.json", "raft_stela.db", dbDir, ":10000", true, config)
	if err != nil {
		fmt.Println("TestMain:", err)
		os.Exit(1)
	}

	// Create Test Services
	srvMap = make(map[string][]*stela.Service)
	addTestService("test.service.fg", "server.local", "10.0.0.99", 8010)
	addTestService("test.service.fg", "macbook.local", "10.0.0.50", 8010)
	addTestService("test.service.fg", "linux.local", "192.168.1.99", 8010)
	addTestService("storage.service.fg", "macbook.local", "10.0.0.50", 8000)
	addTestService("storage.service.fg", "linux.local", "192.168.1.99", 8000)
	addTestService("config.service.fg", "server.local", "10.0.0.99", 8099)
	addTestService("config.service.fg", "linux.local", "192.168.1.99", 8091)

	// Run all test
	t := m.Run()

	os.Exit(t)
}

func dirToTestDB() (string, error) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", err
	}

	return dir, nil
}

func addTestService(name string, target string, address string, port int) {
	testService := new(stela.Service)
	testService.Name = name
	testService.Target = target
	testService.Address = address
	testService.Port = port

	testServices := srvMap[testService.Name]
	testServices = append(testServices, testService)

	srvMap[testService.Name] = testServices
}

func TestNewDiscoveryService(t *testing.T) {
	// This should error
	_, err := NewDiscoveryService("peer_stela.json", "raft_stela.db", "stela", ":10000", false, raft.DefaultConfig())
	if err == nil {
		t.Errorf("TestNewDiscoveryService: Should have errored.")
	}
}

// TestEqual test method (s *stela.Service) Compare
func TestEqual(t *testing.T) {
	fmt.Println("Testing Compare")

	tests := []struct {
		s1     *stela.Service
		s2     *stela.Service
		result bool
	}{
		{
			&stela.Service{
				Name:    "test.service.fg",
				Target:  "server.local",
				Address: "10.0.0.99",
				Port:    8010,
			},
			&stela.Service{
				Name:    "test.service.fg",
				Target:  "server.local",
				Address: "10.0.0.99",
				Port:    8010,
			},
			true,
		},
		{
			&stela.Service{Name: "test.service.fg"},
			&stela.Service{Name: "simple.service.fg"},
			false,
		},
		{
			&stela.Service{Target: "server.local"},
			&stela.Service{Target: "linux.local"},
			false,
		},
		{
			&stela.Service{Address: "10.0.0.99"},
			&stela.Service{Address: "10.0.0.1"},
			false,
		},
		{
			&stela.Service{Port: 8010},
			&stela.Service{Port: 8000},
			false,
		},
	}
	for _, test := range tests {
		if test.s1.Equal(test.s2) != test.result {
			t.Errorf("Compare error. expected: %t", test.result)
		}
	}
}

// TestNewSRV test method (*Service) NewSRV
func TestNewSRV(t *testing.T) {
	fmt.Println("Testing NewSRV")

	// Check all TestServices
	for _, testServices := range srvMap {
		for _, testService := range testServices {
			weight := uint16(0)
			priority := uint16(0)
			ttl := uint32(86400)
			target := dns.Fqdn(testService.Target)    // hostname of the machine providing the service, ending in a dot.
			serviceName := dns.Fqdn(testService.Name) // domain name for which this record is valid, ending in a dot.

			SRV := new(dns.SRV)

			SRV.Hdr = dns.RR_Header{
				Name:   serviceName,
				Rrtype: dns.TypeSRV,
				Class:  dns.ClassINET,
				Ttl:    ttl}
			SRV.Priority = priority
			SRV.Weight = weight
			SRV.Port = uint16(testService.Port)
			SRV.Target = target

			testSRV := testService.NewSRV()

			if testSRV.String() != SRV.String() {
				t.Errorf("NewSRV error: \n testService SRV: \n %v \n Expected: \n %v", testSRV, SRV)
			}
		}
	}
}

// TestNewA test method (*Service) NewA
func TestNewA(t *testing.T) {
	fmt.Println("Testing NewA")

	for _, testServices := range srvMap {
		for _, testService := range testServices {
			serviceName := dns.Fqdn(testService.Target) // domain name for which this record is valid, ending in a dot.

			A := new(dns.A)
			A.Hdr = dns.RR_Header{
				Name:   serviceName,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    86400}
			A.A = net.ParseIP(testService.Address)

			testA := testService.NewA(net.ParseIP(testService.Address))
			if testA.String() != A.String() {
				t.Errorf("NewA error: \n testService A: \n %v \n Expected: \n %v", testA, A)
			}
		}
	}
}

// func TestSetBoltDB(t *testing.T) {
// 	fmt.Println("Testing SetBoltDB")
//
// 	ds := d.(*discoveryService)
//
// 	// Close existing db
// 	// ds.CloseBoltDB()
//
// 	// Create new boltdb
// 	dbDir, err := dirToTestDB()
// 	if err != nil {
// 		t.Errorf("TestSetBoltDB: pathToTestDB error")
// 	}
//
// 	dbPath := filepath.Join(dbDir, dbName)
//
// 	boltdb, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
// 	if err != nil {
// 		log.Fatal(fmt.Errorf("BoltDB error: %s", err))
// 	}
//
// 	// Set new boltdb
// 	ds.SetBoltDB(boltdb)
//
// 	// Test if it was set correctly
// 	if ds.boltdb != boltdb {
// 		t.Errorf("TestSetBoltDB: boltdb didn't match. expected %v. received: %v", boltdb, ds.boltdb)
// 	}
// }

// func TestCloseBoltDB(t *testing.T) {
// 	fmt.Println("Testing CloseBoltDB")
//
// 	ds := d.(*discoveryService)
// 	previousDB := ds.boltdb
//
// 	// Create new boltdb
// 	dbDir, err := dirToTestDB()
// 	if err != nil {
// 		t.Errorf("TestCloseBoltDB: pathToTestDB error")
// 	}
//
// 	dbPath := filepath.Join(dbDir, dbName)
//
// 	boltdb, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
// 	if err != nil {
// 		t.Errorf("TestCloseBoltDB: BoltDB error: %s", err)
// 	}
//
// 	// Set new boltdb
// 	ds.SetBoltDB(boltdb)
// 	ds.CloseBoltDB()
//
// 	testBoltDB := new(bolt.DB)
// 	if fmt.Sprint(boltdb) != fmt.Sprint(testBoltDB) {
// 		t.Errorf("TestCloseBoltDB: bolt close failed. Expected: %v, Received: %v", testBoltDB, boltdb)
// 	}
//
// 	fmt.Println(boltdb)
// 	ds.boltdb = previousDB
// }

func TestRegister(t *testing.T) {
	fmt.Println("Testing Register")

	// Check all TestServices
	for _, testServices := range srvMap {
		for _, testService := range testServices {
			// Register service
			returnedService, err := d.Register(testService.Name, testService.Target, testService.Address, testService.Port)
			if err != nil {
				t.Error("Error registering service", err)
			}

			// Make sure the Service we sent is the one echoed back
			if !testService.Equal(returnedService) {
				t.Errorf("service returned from Register did not match supplied: %v", returnedService)
			}
		}
	}
}

func TestDiscover(t *testing.T) {
	fmt.Println("Testing Discover")

	// Check all TestServices
	for _, testServices := range srvMap {
		var returnedServices []*stela.Service
		var err error

		for _, testService := range testServices {
			// Register service
			returnedServices, err = d.Discover(testService.Name)
			if err != nil {
				t.Error("Error Discover(ing) service", err, returnedServices)
			}
		}

		var returnedServicesString []string
		var testServicesString []string

		/// Set all priorites to 0 and convert to string for slice comparison
		for _, service := range returnedServices {
			service.Priority = 0
			returnedServicesString = append(returnedServicesString, fmt.Sprint(service))
		}

		for _, service := range testServices {
			testServicesString = append(testServicesString, fmt.Sprint(service))
		}

		// Make sure the Service we sent is the one echoed back
		if !sliceutil.Compare(returnedServicesString, testServicesString) {
			t.Errorf("service returned from Discover did not match supplied. \n expected:%v, \n received: %v", testServicesString, returnedServicesString)
		}
	}
}

func TestDiscoverOne(t *testing.T) {
	fmt.Println("Testing DiscoverOne")

	// Check all TestServices
	for _, testServices := range srvMap {
		var returnedService *stela.Service
		var err error

		for _, testService := range testServices {
			// Register service
			returnedService, err = d.DiscoverOne(testService.Name)
			if err != nil {
				t.Error("Error DiscoverOne service", err, returnedService)
			}

			if returnedService.Priority != 0 {
				t.Error("Discovered priority not equal 0", err, returnedService)
			}
		}
	}
}

func TestGenerateServiceKey(t *testing.T) {
	fmt.Println("Testing generateServiceKey")

	// Check all TestServices
	for _, testServices := range srvMap {
		for _, testService := range testServices {
			// Create SRV record key for our test service
			key, err := generateServiceKey(testService.Name, dns.TypeSRV)
			if err != nil {
				t.Errorf("Unable to generate Service Key: %v", err)
			}

			// reverse strings for reverse domain lookup
			labels := dns.SplitDomainName(testService.Name)
			labelCount := len(labels)
			var tempLabel string
			for i := 0; i < labelCount; i++ {
				tempLabel = labels[i]
				labels[i] = labels[labelCount-1]
				labels[labelCount-1] = tempLabel
			}

			testKey := strings.Join(labels, ".")

			// Test key
			if key != testKey+"_33" {
				t.Errorf("Service key does not match. Expected %v, Key is: %v", testKey+"_33", key)
			}
		}
	}

	// We want to error so we pass a invalid Domain Name
	key, err := generateServiceKey("...", dns.TypeSRV)
	if err == nil {
		t.Errorf("Service key should error: %v", key)
	}
}

func TestGetAllSRVForBucket(t *testing.T) {
	fmt.Println("Testing getAllSRVForBucket")

	ds := d.(*discoveryService)
	for key, testServices := range srvMap {
		key, err := generateServiceKey(key, dns.TypeSRV)
		if err != nil {
			t.Errorf("TestGetAllSRVForBucket: Unable to generate Service Key: %v", err)
		}

		srvs, err := ds.getAllSRVForBucket(key)
		if err != nil {
			t.Errorf("TestGetAllSRVForBucket: Unable to getAllSRVForBucket: %v", err)
		}

		// Create a slice of strings for testServices and returned srvs to compare
		testSRVStrings := make([]string, len(testServices))
		SRVStrings := make([]string, len(srvs))
		for i, testService := range testServices {
			testSRVStrings[i] = testService.NewSRV().String()
		}

		for i, srv := range srvs {
			// First set priority and weight to 0 for comparison
			srv.Priority = 0
			srv.Weight = 0
			SRVStrings[i] = srv.String()
		}

		if !sliceutil.Compare(testSRVStrings, SRVStrings) {
			t.Errorf("TestGetAllSRVForBucket: SRVs returned did not match expected. \n received %v \n expected %v", SRVStrings, testSRVStrings)
		}
	}
}

func TestParseDNSQuery(t *testing.T) {
	fmt.Println("Testing ParseDNSQuery")

	for key, testServices := range srvMap {
		msgStrings := make([]string, len(testServices))
		testSRVStrings := make([]string, len(testServices))

		// Create DNS message
		msg := new(dns.Msg)

		// Test SRV
		msg.SetQuestion(dns.Fqdn(key), dns.TypeSRV)

		err := d.ParseDNSQuery(msg)
		if err != nil {
			t.Errorf("TestParseDNSQuery failed: %v", err)
		}

		// Convert answers to strings before comparison
		for i, rr := range msg.Answer {
			// First set priority and weight to 0 for comparison
			srv := rr.(*dns.SRV)
			srv.Priority = 0
			srv.Weight = 0
			msgStrings[i] = rr.String()
		}

		// Convert testServices to strings for comparison
		for i, testService := range testServices {
			testSRVStrings[i] = testService.NewSRV().String()
		}

		if !sliceutil.Compare(msgStrings, testSRVStrings) {
			t.Errorf("TestParseDNSQuery: MSGs returned did not match expected \n Expected: %v \n Received: %v", testSRVStrings, msgStrings)
		}
	}
}

func TestRotateSRVPriorities(t *testing.T) {
	fmt.Println("Testing RotateSRVPriority")

	// Let's try to Round Robin the first test service
	for key, testServices := range srvMap {
		testService := testServices[0]

		// First let's get the SRV that matches the test Service
		bucketKey, err := generateServiceKey(key, dns.TypeSRV)
		if err != nil {
			t.Error("TestRotateSRVPriority: Error with bucketKey", err)
		}

		var testSRV *dns.SRV
		var srvLength int
		getTestSRVPriority := func() int {
			srvs, err := d.(*discoveryService).getAllSRVForBucket(bucketKey)
			srvLength = len(srvs)
			if err != nil {
				t.Error("TestRotateSRVPriority: Error with getAllSRVForBucket", err)
			}

			for _, srv := range srvs {
				if srv.Target == dns.Fqdn(testService.Target) {
					testSRV = srv
					break
				}
			}

			return int(testSRV.Priority)
		}
		previousPriority := getTestSRVPriority()

		if testSRV == nil {
			t.Fatalf("TestRotateSRVPriority: testSRV is nil")
		}

		err = d.RotateSRVPriorities(testService.Name, false)
		if err != nil {
			t.Error("TestRotateSRVPriority: Error with RotateSRVPriority", err)
		}

		newPriority := getTestSRVPriority()

		// Modulate on srv length
		expectedPriority := (previousPriority + 1) % srvLength
		if expectedPriority != newPriority {
			t.Errorf("TestRotateSRVPriority: Error priority didn't increment. Expected: %d, Received: %d", expectedPriority, newPriority)
		}
	}
}
