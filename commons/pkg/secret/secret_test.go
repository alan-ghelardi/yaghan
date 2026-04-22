package secret

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/google/go-cmp/cmp"
	"go.uber.org/mock/gomock"
	smclient "golang.nuinfra.net/commons/pkg/aws/secretsmanager"
	smmocks "golang.nuinfra.net/commons/pkg/aws/secretsmanager/mocks"
)

func TestSecrets(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{
			name: "local file",
			in:   "testdata/fake-secret",
			want: "abcd1234",
		},
		{
			name: "inline secret",
			in: map[string]any{
				inlineKey: base64.StdEncoding.EncodeToString([]byte("abcd1234")),
			},
			want: "abcd1234",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			secret, err := Parse(test.in)
			if err != nil {
				t.Fatal(err)
			}

			data, err := secret.Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, data); diff != "" {
				t.Errorf("Mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseSecretsManager(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		want    Secret
		wantErr string
	}{
		{
			name: "valid secretsmanager secret",
			in: map[string]any{
				secretsManagerKey: map[string]any{
					secretsManagerIDKey: "my-secret",
				},
			},
			want: &SecretsManagerValue{SecretID: "my-secret"},
		},
		{
			name: "missing secret-id",
			in: map[string]any{
				secretsManagerKey: map[string]any{},
			},
			wantErr: "the secretsmanager secret requires a non-empty secret-id",
		},
		{
			name: "secretsmanager value is not a map",
			in: map[string]any{
				secretsManagerKey: "not-a-map",
			},
			wantErr: "the secretsmanager secret must be a dictionary",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			secret, err := Parse(test.in)
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", test.wantErr)
				}
				if !contains(err.Error(), test.wantErr) {
					t.Fatalf("expected error containing %q, got %q", test.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, secret); diff != "" {
				t.Errorf("Mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSecretsManagerValueLoad(t *testing.T) {
	tests := []struct {
		name      string
		secretID  string
		output    *secretsmanager.GetSecretValueOutput
		clientErr error
		want      string
		wantErr   bool
	}{
		{
			name:     "returns secret string",
			secretID: "my-secret",
			output: &secretsmanager.GetSecretValueOutput{
				SecretString: aws.String("super-secret-value"),
			},
			want: "super-secret-value",
		},
		{
			name:     "returns secret binary",
			secretID: "my-binary-secret",
			output: &secretsmanager.GetSecretValueOutput{
				SecretBinary: []byte("binary-secret-value"),
			},
			want: "binary-secret-value",
		},
		{
			name:      "client error",
			secretID:  "missing-secret",
			clientErr: errors.New("secret not found"),
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mock := smmocks.NewMockClient(ctrl)

			mock.EXPECT().
				GetSecretValue(gomock.Any(), &secretsmanager.GetSecretValueInput{
					SecretId: aws.String(test.secretID),
				}).
				Return(test.output, test.clientErr)

			ctx := smclient.With(context.Background(), mock)
			s := &SecretsManagerValue{SecretID: test.secretID}

			data, err := s.Load(ctx)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, data); diff != "" {
				t.Errorf("Mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSecretsManagerValueString(t *testing.T) {
	s := &SecretsManagerValue{SecretID: "my-secret"}
	want := "{secretsmanager: my-secret}"
	if got := s.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
