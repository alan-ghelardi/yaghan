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

	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	"golang.nuinfra.net/commons/pkg/aws/dynamodb"
)

// StartDynamoDB starts an amazon/dynamodb-local container for the duration
// of the test and returns its endpoint URL (with http:// scheme), ready to
// feed into [dynamodb.Config.Endpoint]. The container is terminated via
// t.Cleanup; terminate failures are logged as warnings rather than failing
// the test, since the container may already be reaped (Ryuk).
//
// The image and command match api-server/dev/docker-compose.yml so dev and
// tests run against the same emulator.
func StartDynamoDB(t *testing.T) string {
	t.Helper()

	ctx := t.Context()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "amazon/dynamodb-local:latest",
			ExposedPorts: []string{"8000/tcp"},
			Cmd:          []string{"-jar", "DynamoDBLocal.jar", "-inMemory"},
			WaitingFor:   wait.ForListeningPort("8000/tcp"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		// t.Context() is already cancelled by the time t.Cleanup runs, so
		// using it here would always fail Terminate with "context
		// canceled". Use a fresh, time-bounded context instead.
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := container.Terminate(stopCtx); err != nil {
			t.Logf("warning: unable to terminate DynamoDB container: %v", err)
		}
	})

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	return "http://" + endpoint
}

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

	ctx := t.Context()

	config := awsconfig.New(ctx)
	ctx = awsconfig.With(ctx, config)

	dynamodbClient := dynamodb.New(ctx, dynamodb.Config{Endpoint: endpointURL})

	_, err = dynamodbClient.CreateTable(ctx, createTableInput)
	if err != nil {
		var resourceInUse *types.ResourceInUseException
		if errors.As(err, &resourceInUse) {
			// Table already exists, ignore the error
			t.Logf("Table %s already exists, proceeding...", *createTableInput.TableName)
		} else {
			t.Fatal(err)
		}
	}

	t.Cleanup(func() {
		_, err := dynamodbClient.DeleteTable(ctx, &dynamodbservice.DeleteTableInput{TableName: createTableInput.TableName})
		if err != nil {
			t.Logf("error deleting table %s: %v", *createTableInput.TableName, err)
		}
	})

	return *createTableInput.TableName
}
