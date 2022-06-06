package native

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"context"
	l "log"

	"github.com/pact-foundation/pact-go/v2/log"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

func init() {
	Init()
}

func TestMockServer_CreateAndCleanupMockServer(t *testing.T) {
	m := MockServer{}
	port, _ := m.CreateMockServer(pactComplex, "0.0.0.0:0", false)
	defer m.CleanupMockServer(port)

	if port <= 0 {
		t.Fatal("want port > 0, got", port)
	}
}

func TestMockServer_MismatchesSuccess(t *testing.T) {
	m := MockServer{}
	port, _ := m.CreateMockServer(pactSimple, "0.0.0.0:0", false)
	defer m.CleanupMockServer(port)

	res, err := http.Get(fmt.Sprintf("http://localhost:%d/foobar", port))
	if err != nil {
		t.Fatalf("Error sending request: %v", err)
	}

	if res.StatusCode != 200 {
		t.Fatalf("want '200', got '%d'", res.StatusCode)
	}

	mismatches := m.MockServerMismatchedRequests(port)
	if len(mismatches) != 0 {
		t.Fatalf("want 0 mismatches, got '%d'", len(mismatches))
	}
}

func TestMockServer_MismatchesFail(t *testing.T) {
	m := MockServer{}
	port, _ := m.CreateMockServer(pactSimple, "0.0.0.0:0", false)
	defer m.CleanupMockServer(port)

	mismatches := m.MockServerMismatchedRequests(port)
	if len(mismatches) != 1 {
		t.Fatalf("want 1 mismatch, got '%d'", len(mismatches))
	}
}

func TestMockServer_VerifySuccess(t *testing.T) {
	tmpPactFolder, err := ioutil.TempDir("", "pact-go")
	assert.NoError(t, err)

	m := MockServer{}
	port, _ := m.CreateMockServer(pactSimple, "0.0.0.0:0", false)
	defer m.CleanupMockServer(port)

	_, err = http.Get(fmt.Sprintf("http://localhost:%d/foobar", port))
	if err != nil {
		t.Fatalf("Error sending request: %v", err)
	}

	success, mismatches := m.Verify(port, tmpPactFolder)
	if !success {
		t.Fatalf("want 'true' but got '%v'", success)
	}

	if len(mismatches) != 0 {
		t.Fatalf("want 0 mismatches, got '%d'", len(mismatches))
	}
}

func TestMockServer_VerifyFail(t *testing.T) {
	tmpPactFolder, err := ioutil.TempDir("", "pact-go")
	assert.NoError(t, err)
	m := MockServer{}
	port, _ := m.CreateMockServer(pactSimple, "0.0.0.0:0", false)

	success, mismatches := m.Verify(port, tmpPactFolder)
	if success {
		t.Fatalf("want 'false' but got '%v'", success)
	}

	if len(mismatches) != 1 {
		t.Fatalf("want 1 mismatch, got '%d'", len(mismatches))
	}
}

func TestMockServer_WritePactfile(t *testing.T) {
	tmpPactFolder, err := ioutil.TempDir("", "pact-go")
	assert.NoError(t, err)

	m := MockServer{}
	port, _ := m.CreateMockServer(pactSimple, "0.0.0.0:0", false)
	defer m.CleanupMockServer(port)

	_, err = http.Get(fmt.Sprintf("http://localhost:%d/foobar", port))
	if err != nil {
		t.Fatalf("Error sending request: %v", err)
	}
	err = m.WritePactFile(port, tmpPactFolder)

	if err != nil {
		t.Fatal("error: ", err)
	}
}

func TestMockServer_GetTLSConfig(t *testing.T) {
	config := GetTLSConfig()

	t.Log("tls config", config)
}

func TestVersion(t *testing.T) {
	t.Log("version: ", Version())
}

func TestHandleBasedHTTPTests(t *testing.T) {
	tmpPactFolder, err := ioutil.TempDir("", "pact-go")
	assert.NoError(t, err)

	m := NewHTTPMockServer("test-http-consumer", "test-http-provider")

	i := m.NewInteraction("some interaction")

	i.UponReceiving("some interaction").
		Given("some state").
		WithRequest("GET", "/products").
		WithJSONResponseBody(`{
	  	"name": {
      	"pact:matcher:type": "type",
      	"value": "some name"
    	},
	  	"age": 23,
	  	"alive": true
		}`).
		WithStatus(200)

	// // Start the mock service
	// const host = "127.0.0.1"
	port, err := m.Start("0.0.0.0:0", false)
	assert.NoError(t, err)
	defer m.CleanupMockServer(port)

	_, err = http.Get(fmt.Sprintf("http://0.0.0.0:%d/products", port))
	assert.NoError(t, err)

	mismatches := m.MockServerMismatchedRequests(port)
	if len(mismatches) != 0 {
		t.Fatalf("want 0 mismatches, got '%d'", len(mismatches))
	}

	err = m.WritePactFile(port, tmpPactFolder)
	assert.NoError(t, err)
}

func TestPluginInteraction(t *testing.T) {
	tmpPactFolder, err := ioutil.TempDir("", "pact-go")
	assert.NoError(t, err)
	log.SetLogLevel("trace")

	m := NewHTTPMockServer("test-plugin-consumer", "test-plugin-provider")

	// Protobuf plugin test
	m.UsingPlugin("protobuf", "0.0.3")
	m.WithSpecificationVersion(SPECIFICATION_VERSION_V4)

	i := m.NewInteraction("some plugin interaction")

	dir, _ := os.Getwd()
	path := fmt.Sprintf("%s/plugin.proto", dir)

	protobufInteraction := `{
			"pact:proto": "` + path + `",
			"pact:message-type": "InitPluginRequest",
			"pact:content-type": "application/protobuf",
			"implementation": "notEmpty('pact-go-driver')",
			"version": "matching(semver, '0.0.0')"
		}`

	i.UponReceiving("some interaction").
		Given("plugin state").
		WithRequest("GET", "/protobuf").
		WithStatus(200).
		WithPluginInteractionContents(INTERACTION_PART_RESPONSE, "application/protobuf", protobufInteraction)

	port, err := m.Start("0.0.0.0:0", false)
	assert.NoError(t, err)
	defer m.CleanupMockServer(port)

	res, err := http.Get(fmt.Sprintf("http://0.0.0.0:%d/protobuf", port))
	assert.NoError(t, err)

	bytes, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)

	initPluginRequest := &InitPluginRequest{}
	proto.Unmarshal(bytes, initPluginRequest)
	assert.NoError(t, err)

	assert.Equal(t, "pact-go-driver", initPluginRequest.Implementation)
	assert.Equal(t, "0.0.0", initPluginRequest.Version)

	mismatches := m.MockServerMismatchedRequests(port)
	if len(mismatches) != 0 {
		assert.Len(t, mismatches, 0)
		t.Log(mismatches)
	}

	err = m.WritePactFile(port, tmpPactFolder)
	assert.NoError(t, err)
}

var pactSimple = `{
  "consumer": {
    "name": "consumer"
  },
  "provider": {
    "name": "provider"
  },
  "interactions": [
    {
      "description": "Some name for the test",
      "request": {
        "method": "GET",
        "path": "/foobar"
      },
      "response": {
        "status": 200
      },
      "description": "Some name for the test",
      "provider_state": "Some state"
  }]
}`

var pactComplex = `{
  "consumer": {
    "name": "consumer"
  },
  "provider": {
    "name": "provider"
  },
  "interactions": [
    {
    "request": {
      "method": "GET",
      "path": "/foobar",
      "body": {
        "pass": 1234,
        "user": {
          "address": "some address",
          "name": "someusername",
          "phone": 12345678,
          "plaintext": "plaintext"
        }
      }
    },
    "response": {
      "status": 200
    },
    "description": "Some name for the test",
    "provider_state": "Some state",
    "matchingRules": {
      "$.body.pass": {
        "match": "regex",
        "regex": "\\d+"
      },
      "$.body.user.address": {
        "match": "regex",
        "regex": "\\s+"
      },
      "$.body.user.name": {
        "match": "regex",
        "regex": "\\s+"
      },
      "$.body.user.phone": {
        "match": "regex",
        "regex": "\\d+"
      }
    }
  }]
}`

func TestGrpcPluginInteraction(t *testing.T) {
	tmpPactFolder, err := ioutil.TempDir("", "pact-go")
	assert.NoError(t, err)
	log.InitLogging()
	log.SetLogLevel("TRACE")

	m := NewHTTPMockServer("test-grpc-consumer", "test-plugin-provider")

	// Protobuf plugin test
	m.UsingPlugin("protobuf", "0.1.5")
	// m.WithSpecificationVersion(SPECIFICATION_VERSION_V4)

	i := m.NewSyncMessageInteraction("grpc interaction")

	dir, _ := os.Getwd()
	path := fmt.Sprintf("%s/plugin.proto", dir)

	grpcInteraction := `{
			"pact:proto": "` + path + `",
			"pact:proto-service": "PactPlugin/InitPlugin",
			"pact:content-type": "application/protobuf",
			"request": {
				"implementation": "notEmpty('pact-go-driver')",
				"version": "matching(semver, '0.0.0')"	
			},
			"response": {
				"catalogue": [
					{
						"type": "INTERACTION",
						"key": "test"
					}
				]
			}
		}`

	i.
		Given("plugin state").
		// For gRPC interactions we prpvide the config once for both the request and response parts
		WithPluginInteractionContents(INTERACTION_PART_REQUEST, "application/protobuf", grpcInteraction)

	// Start the gRPC mock server
	port, err := m.StartTransport("grpc", "127.0.0.1", 0, make(map[string][]interface{}))
	assert.NoError(t, err)
	defer m.CleanupMockServer(port)

	// Now we can make a normal gRPC request
	initPluginRequest := &InitPluginRequest{
		Implementation: "pact-go-test",
		Version:        "1.0.0",
	}

	// Need to make a gRPC call here
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		l.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := NewPactPluginClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err := c.InitPlugin(ctx, initPluginRequest)
	if err != nil {
		l.Fatalf("could not initialise the plugin: %v", err)
	}
	l.Printf("InitPluginResponse: %v", r)

	mismatches := m.MockServerMismatchedRequests(port)
	if len(mismatches) != 0 {
		assert.Len(t, mismatches, 0)
		t.Log(mismatches)
	}

	err = m.WritePactFile(port, tmpPactFolder)
	assert.NoError(t, err)
}