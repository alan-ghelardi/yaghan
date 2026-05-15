package secret

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"

	smclient "github.com/alan-ghelardi/yaghan/commons/pkg/aws/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/go-viper/mapstructure/v2"
)

const (
	inlineKey           = "inline"
	secretsManagerKey   = "secretsmanager"
	secretsManagerIDKey = "secret-id"
)

// Secret represents a confidential piece of data that can be loaded
// from various sources, such as the local file system, Amazon S3, and others.
type Secret interface {
	fmt.Stringer

	Load(ctx context.Context) (string, error)
}

// DecodeHook is a mapstructure decode hook that parses Secret values from configuration files.
func DecodeHook(_, target reflect.Type, data any) (any, error) {
	if target.Implements(reflect.TypeFor[Secret]()) {
		return Parse(data)
	}
	return data, nil
}

// Ensure DecodeHook satisfies the DecodeHookFunc interface.
var _ mapstructure.DecodeHookFunc = DecodeHook

// Parse creates a Secret from the supplied input.
func Parse(input any) (Secret, error) {
	switch input := input.(type) {
	case string:
		return &File{Path: input}, nil
	case map[string]any:
		return parseMap(input)
	}
	return nil, fmt.Errorf("unrecognized secret decoded as %T", input)
}

func parseMap(m map[string]any) (Secret, error) {
	if value, found := m[inlineKey]; found {
		return parseInline(value)
	}
	if value, found := m[secretsManagerKey]; found {
		return parseSecretsManager(value)
	}
	return nil, errors.New("the secret must be a dictionary in the format: {inline: <string>} or {secretsmanager: {secret-id: <string>}}")
}

func parseInline(value any) (Secret, error) {
	s, ok := value.(string)
	if !ok {
		return nil, errors.New("the inline secret must be a string in the format: {inline: <string>}")
	}
	return InlineValue(s), nil
}

func parseSecretsManager(value any) (Secret, error) {
	m, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("the secretsmanager secret must be a dictionary in the format: {secretsmanager: {secret-id: <string>}}")
	}
	id, ok := m[secretsManagerIDKey].(string)
	if !ok || len(id) == 0 {
		return nil, errors.New("the secretsmanager secret requires a non-empty secret-id")
	}
	return &SecretsManagerValue{SecretID: id}, nil
}

// File is a secret read from the File system.
type File struct {
	Path string
}

var _ Secret = (*File)(nil)

// Load implements Secret.
func (f *File) Load(_ context.Context) (string, error) {
	file, err := os.Open(f.Path)
	if err != nil {
		return "", err
	}

	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// String implements Secret.
func (f *File) String() string {
	return f.Path
}

// InlineValue is an inline secret encoded in the base64 format.
type InlineValue string

var _ Secret = (InlineValue)("")

// Load implements Secret.
func (i InlineValue) Load(_ context.Context) (string, error) {
	data, err := base64.StdEncoding.DecodeString(string(i))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// String implements Secret.
func (i InlineValue) String() string {
	return "{inline: <redacted>}"
}

// SecretsManagerValue is a secret retrieved from AWS Secrets Manager.
type SecretsManagerValue struct {
	SecretID string
}

var _ Secret = (*SecretsManagerValue)(nil)

// Load implements Secret.
func (s *SecretsManagerValue) Load(ctx context.Context) (string, error) {
	client := smclient.Get(ctx)
	output, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(s.SecretID),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %q from Secrets Manager: %w", s.SecretID, err)
	}
	if output.SecretString != nil {
		return *output.SecretString, nil
	}
	return string(output.SecretBinary), nil
}

// String implements Secret.
func (s *SecretsManagerValue) String() string {
	return fmt.Sprintf("{secretsmanager: %s}", s.SecretID)
}
