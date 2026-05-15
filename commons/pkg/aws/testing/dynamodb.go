package testing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	awsconfig "github.com/alan-ghelardi/yaghan/commons/pkg/aws/config"
	"github.com/alan-ghelardi/yaghan/commons/pkg/aws/dynamodb"
	"github.com/alan-ghelardi/yaghan/commons/pkg/utilities"
	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// dynamoDBContainerRequest is the testcontainers spec for the
// amazon/dynamodb-local image. The image and command match
// api-server/dev/docker-compose.yml so dev and tests run against the
// same emulator.
func dynamoDBContainerRequest() testcontainers.GenericContainerRequest {
	return testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "amazon/dynamodb-local:latest",
			ExposedPorts: []string{"8000/tcp"},
			Cmd:          []string{"-jar", "DynamoDBLocal.jar", "-inMemory"},
			WaitingFor:   wait.ForListeningPort("8000/tcp"),
		},
		Started: true,
	}
}

// terminateContainer stops a testcontainers-managed container with a
// fresh, bounded context. The caller's context is typically already
// cancelled (t.Context() in cleanup, or background-cancel in TestMain),
// so using it for Terminate would always fail with "context canceled".
func terminateContainer(container testcontainers.Container) error {
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return container.Terminate(stopCtx)
}

// StartDynamoDB starts an amazon/dynamodb-local container for the
// duration of a single test and returns its endpoint URL (with http://
// scheme). The container is terminated via t.Cleanup; terminate
// failures are logged as warnings rather than failing the test, since
// the container may already be reaped (Ryuk).
//
// For per-suite sharing — substantially faster — see
// [StartSharedDynamoDB] and the TestMain pattern.
func StartDynamoDB(t *testing.T) string {
	t.Helper()

	endpoint, cleanup, err := startContainer(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Logf("warning: unable to terminate DynamoDB container: %v", err)
		}
	})
	return endpoint
}

// StartSharedDynamoDB starts an amazon/dynamodb-local container suitable
// for sharing across all tests in a package via TestMain. It returns
// the http://host:port endpoint, a cleanup func to call before
// os.Exit in TestMain, and any error.
//
// Per-test isolation under a shared container is the responsibility of
// [CreateTable], which appends a random suffix to the table name.
//
// Typical use:
//
//	var sharedEndpoint string
//
//	func TestMain(m *testing.M) {
//	    endpoint, cleanup, err := awstesting.StartSharedDynamoDB(context.Background())
//	    if err != nil { /* ... */ }
//	    sharedEndpoint = endpoint
//	    code := m.Run()
//	    _ = cleanup()
//	    os.Exit(code)
//	}
func StartSharedDynamoDB(ctx context.Context) (endpoint string, cleanup func() error, err error) {
	return startContainer(ctx)
}

// startContainer is the shared implementation behind StartDynamoDB and
// StartSharedDynamoDB. It returns an http://host:port endpoint and a
// cleanup func that terminates the container with a fresh context.
func startContainer(ctx context.Context) (string, func() error, error) {
	container, err := testcontainers.GenericContainer(ctx, dynamoDBContainerRequest())
	if err != nil {
		return "", nil, err
	}
	cleanup := func() error { return terminateContainer(container) }

	raw, err := container.Endpoint(ctx, "")
	if err != nil {
		_ = cleanup()
		return "", nil, err
	}
	return "http://" + raw, cleanup, nil
}

// CreateTable creates a DynamoDB table from the JSON schema at
// schemaFile against the supplied endpoint, and returns the
// (suffix-mangled) name actually used. The schema's TableName field is
// appended with a short random suffix so multiple tests sharing a
// single container (typical TestMain pattern) get isolated tables; per
// test isolation under a per-test container is also preserved.
//
// The table is deleted via t.Cleanup. Cleanup failures are logged but
// do not fail the test.
func CreateTable(t *testing.T, endpointURL string, schemaFile string) (tableName string) {
	t.Helper()

	_, base, _, _ := runtime.Caller(1)
	realSchemaPath, err := filepath.Abs(filepath.Join(base, schemaFile))
	if err != nil {
		t.Fatalf("cannot resolve DynamoDB schema file: %v", err)
	}

	data, err := os.ReadFile(realSchemaPath)
	if err != nil {
		t.Fatal(err)
	}

	createTableInput := &dynamodbservice.CreateTableInput{}
	if err := json.Unmarshal(data, createTableInput); err != nil {
		t.Fatalf("error parsing schema at %s: %v", realSchemaPath, err)
	}

	// Append a short random suffix so concurrent or sequential calls
	// against a shared container don't collide. 8 alphanumeric chars
	// give ~3e12 unique values; collision probability inside a single
	// test run is effectively zero.
	uniqueName := *createTableInput.TableName + "-" + utilities.RandomString(8)
	createTableInput.TableName = &uniqueName

	ctx := t.Context()
	cfg := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, cfg)
	dynamodbClient := dynamodb.New(ctx, dynamodb.Config{Endpoint: endpointURL})

	_, err = dynamodbClient.CreateTable(ctx, createTableInput)
	if err != nil {
		var resourceInUse *types.ResourceInUseException
		if errors.As(err, &resourceInUse) {
			// Defensive: with the random suffix this branch is
			// effectively unreachable, but harmless.
			t.Logf("Table %s already exists, proceeding...", uniqueName)
		} else {
			t.Fatal(err)
		}
	}

	t.Cleanup(func() {
		// As above: t.Context() is cancelled by cleanup time. Use a
		// fresh bounded context so DeleteTable actually runs.
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := dynamodbClient.DeleteTable(stopCtx, &dynamodbservice.DeleteTableInput{TableName: &uniqueName})
		if err != nil {
			t.Logf("warning: error deleting table %s: %v", uniqueName, err)
		}
	})

	return uniqueName
}
