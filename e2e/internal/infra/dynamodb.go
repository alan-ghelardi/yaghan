package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// EnsureTables creates each table declared by a JSON schema under
// schemaDir, idempotently. Schemas are the AWS-CLI input format that
// api-server/dynamodb-tables/*.json uses — the same files dev/start.sh
// feeds to `aws dynamodb create-table --cli-input-json`.
func EnsureTables(ctx context.Context, client *dynamodb.Client, schemaDir string) error {
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		return fmt.Errorf("read schema dir %s: %w", schemaDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(schemaDir, entry.Name())
		input, err := loadCreateTableInput(path)
		if err != nil {
			return err
		}
		if err := ensureTable(ctx, client, input); err != nil {
			return fmt.Errorf("ensure table %s: %w", *input.TableName, err)
		}
	}
	return nil
}

// ResetTable deletes and re-creates the table described by the given
// schema file. Used in BeforeSuite to start each run from a clean
// slate even when DynamoDB Local has carried over from a previous run.
func ResetTable(ctx context.Context, client *dynamodb.Client, schemaFile string) error {
	input, err := loadCreateTableInput(schemaFile)
	if err != nil {
		return err
	}
	name := *input.TableName

	_, err = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: input.TableName})
	if err != nil {
		var notFound *types.ResourceNotFoundException
		if !errors.As(err, &notFound) {
			return fmt.Errorf("delete table %s: %w", name, err)
		}
	} else if err := waitTableGone(ctx, client, name); err != nil {
		return err
	}

	if _, err := client.CreateTable(ctx, input); err != nil {
		return fmt.Errorf("create table %s: %w", name, err)
	}
	return waitTableActive(ctx, client, name)
}

func ensureTable(ctx context.Context, client *dynamodb.Client, input *dynamodb.CreateTableInput) error {
	_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: input.TableName})
	if err == nil {
		return nil
	}
	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describe table: %w", err)
	}
	if _, err := client.CreateTable(ctx, input); err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	return waitTableActive(ctx, client, *input.TableName)
}

// loadCreateTableInput unmarshals an AWS-CLI input-format JSON file
// into a CreateTableInput. The CLI input format and the Go SDK input
// struct have matching field names, so encoding/json handles it
// directly.
func loadCreateTableInput(path string) (*dynamodb.CreateTableInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", path, err)
	}
	var input dynamodb.CreateTableInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parse schema %s: %w", path, err)
	}
	if input.TableName == nil || *input.TableName == "" {
		return nil, fmt.Errorf("schema %s: missing TableName", path)
	}
	return &input, nil
}

func waitTableActive(ctx context.Context, client *dynamodb.Client, name string) error {
	w := dynamodb.NewTableExistsWaiter(client)
	return w.Wait(ctx, &dynamodb.DescribeTableInput{TableName: &name}, 30*time.Second)
}

func waitTableGone(ctx context.Context, client *dynamodb.Client, name string) error {
	w := dynamodb.NewTableNotExistsWaiter(client)
	return w.Wait(ctx, &dynamodb.DescribeTableInput{TableName: &name}, 30*time.Second)
}
