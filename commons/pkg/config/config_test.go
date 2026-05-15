package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/alan-ghelardi/yaghan/commons/pkg/logger"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type testConfig struct {
	Base `mapstructure:",squash"`
}

func TestNewFromFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     *testConfig
	}{
		{
			name:     "full configuration",
			filename: "full-config.yaml",
			want: &testConfig{
				Base: Base{
					Logger: &logger.Config{
						LogLevel:  zap.NewAtomicLevelAt(zapcore.DebugLevel),
						ZapConfig: json.RawMessage([]byte(`{"development":true}`)),
					},
					GRPCPort:      3000,
					RestPort:      8080,
					OpenAPIV2Path: "/etc/open-api.json",
					Auth: &Auth{
						AllowedAudience:   "foo",
						ScopeKey:          "scp",
						RemoveScopePrefix: "default/",
						SigningKeys: &SigningKeys{
							LocalKeyPath: "/etc/jwk.json",
						},
					},
					TLS: &TLS{
						SelfSignedCert: true,
					},
					KeepAlive: &KeepAlive{
						MinTime:  time.Second,
						Interval: 5 * time.Second,
						Timeout:  time.Minute,
					},
					MaxConcurrentStreams: 10,
				},
			},
		},
		{
			name:     "use default values",
			filename: "empty-config.yaml",
			want: &testConfig{
				Base: Base{
					Logger: &logger.Config{
						LogLevel: zap.NewAtomicLevelAt(zapcore.InfoLevel),
					},
					GRPCPort: DefaultGRPCPort,
					Auth:     &Auth{ScopeKey: DefaultScopeKey},
					KeepAlive: &KeepAlive{
						MinTime:  DefaultKeepAliveMinTime,
						Interval: DefaultKeepAliveInterval,
						Timeout:  DefaultKeepAliveTimeout,
					},
					MaxConcurrentStreams: DefaultMaxConcurrentStreams,
				},
			},
		},
		{
			name:     "remote signing key",
			filename: "remote-key.yaml",
			want: &testConfig{
				Base: Base{
					Logger: &logger.Config{
						LogLevel: zap.NewAtomicLevelAt(zapcore.InfoLevel),
					},
					GRPCPort: DefaultGRPCPort,
					Auth: &Auth{
						ScopeKey: DefaultScopeKey,
						SigningKeys: &SigningKeys{
							RemoteKeyURL: "https://example.dev/oauth2/v1/keys",
						},
					},
					KeepAlive: &KeepAlive{
						MinTime:  DefaultKeepAliveMinTime,
						Interval: DefaultKeepAliveInterval,
						Timeout:  DefaultKeepAliveTimeout,
					},
					MaxConcurrentStreams: DefaultMaxConcurrentStreams,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := NewFromFile[*testConfig](filepath.Join("testdata", test.filename), viper.New())
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.want, config,
				cmpopts.IgnoreUnexported(zap.AtomicLevel{})); diff != "" {
				t.Errorf("Unexpected config (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadCertificate(t *testing.T) {
	tls := &TLS{SelfSignedCert: true}
	_, err := tls.LoadCertificate()
	if err != nil {
		t.Fatal(err)
	}
}
