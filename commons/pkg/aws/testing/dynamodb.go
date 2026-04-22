package testing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	dynamodbservice "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	awsconfig "golang.nuinfra.net/commons/pkg/aws/config"
	"golang.nuinfra.net/commons/pkg/aws/dynamodb"
)

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
