package infra

import (
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestLoadCreateTableInput_NodesSchema(t *testing.T) {
	path := filepath.Join("..", "..", "..", "api-server", "dynamodb-tables", "nodes.json")
	input, err := loadCreateTableInput(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := *input.TableName; got != "nodes" {
		t.Errorf("TableName: got %q, want %q", got, "nodes")
	}
	if input.BillingMode != types.BillingModePayPerRequest {
		t.Errorf("BillingMode: got %q, want %q", input.BillingMode, types.BillingModePayPerRequest)
	}
	if got := len(input.AttributeDefinitions); got != 4 {
		t.Errorf("AttributeDefinitions: got %d, want 4", got)
	}
	if got := len(input.KeySchema); got != 1 {
		t.Errorf("KeySchema: got %d, want 1", got)
	}
	if len(input.KeySchema) == 1 && input.KeySchema[0].KeyType != types.KeyTypeHash {
		t.Errorf("KeySchema[0].KeyType: got %q, want %q", input.KeySchema[0].KeyType, types.KeyTypeHash)
	}
	if got := len(input.GlobalSecondaryIndexes); got != 2 {
		t.Errorf("GlobalSecondaryIndexes: got %d, want 2", got)
	}
}
