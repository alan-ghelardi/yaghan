package config

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.nuinfra.net/commons/pkg/config"
	"golang.nuinfra.net/commons/pkg/logger"
	"golang.nuinfra.net/commons/pkg/secret"
)

func TestNewFromFile(t *testing.T) {
	tests := []struct {
		name           string
		filename       string
		expectedConfig *Bundle
	}{
		{
			name:     "full configuration",
			filename: "full-config.yaml",
			expectedConfig: &Bundle{
				Base: config.Base{
					Logger: &logger.Config{
						LogLevel:  zap.NewAtomicLevelAt(zapcore.DebugLevel),
						ZapConfig: json.RawMessage([]byte(`{"development":true}`)),
					},
					GRPCPort:      3000,
					RestPort:      8080,
					OpenAPIV2Path: "/etc/open-api.json",
					Auth: &config.Auth{
						AllowedAudience:   "foo",
						ScopeKey:          "scp",
						RemoveScopePrefix: "default/",
						SigningKeys: &config.SigningKeys{
							LocalKeyPath: "/etc/jwk.json",
						},
					},
					TLS: &config.TLS{
						SelfSignedCert: true,
					},
					KeepAlive: &config.KeepAlive{
						MinTime:  time.Second,
						Interval: 5 * time.Second,
						Timeout:  time.Minute,
					},
					MaxConcurrentStreams: 10,
				},
				WatchStream: &WatchStream{
					EventDeliveryTimeout: 15 * time.Second,
					HeartbeatInterval:    defaultHeartbeatInterval,
					Redis: &Redis{
						Address:                "localhost:6379",
						Database:               1,
						StreamName:             "my-stream",
						StreamKey:              "my-key",
						MaxStreamLength:        100,
						KeyTTL:                 15 * time.Minute,
						Timeout:                15 * time.Second,
						StreamReadinessTimeout: 15 * time.Second,
						TLS:                    &RedisTLS{},
					},
				},
				Database: &Database{
					AWS: &AWS{
						SandboxesTableName: "sandboxes",
						NodesTableName:     "nodes",
					},
				},
			},
		},
		{
			name:     "minimal configuration",
			filename: "minimal-config.yaml",
			expectedConfig: &Bundle{
				Base: config.Base{
					Logger: &logger.Config{
						LogLevel: zap.NewAtomicLevelAt(zapcore.InfoLevel),
					},
					GRPCPort: config.DefaultGRPCPort,
					Auth:     &config.Auth{ScopeKey: config.DefaultScopeKey},
					KeepAlive: &config.KeepAlive{
						MinTime:  config.DefaultKeepAliveMinTime,
						Interval: config.DefaultKeepAliveInterval,
						Timeout:  config.DefaultKeepAliveTimeout,
					},
					MaxConcurrentStreams: config.DefaultMaxConcurrentStreams,
				},
				Database: &Database{
					AWS: &AWS{
						SandboxesTableName: "sandboxes",
						NodesTableName:     "nodes",
					},
				},
			},
		},
		{
			name:     "default values for Redis stream",
			filename: "redis-stream.yaml",
			expectedConfig: &Bundle{
				Base: config.Base{
					Logger: &logger.Config{
						LogLevel: zap.NewAtomicLevelAt(zapcore.InfoLevel),
					},
					GRPCPort: config.DefaultGRPCPort,
					Auth:     &config.Auth{ScopeKey: config.DefaultScopeKey},
					KeepAlive: &config.KeepAlive{
						MinTime:  config.DefaultKeepAliveMinTime,
						Interval: config.DefaultKeepAliveInterval,
						Timeout:  config.DefaultKeepAliveTimeout,
					},
					MaxConcurrentStreams: config.DefaultMaxConcurrentStreams,
				},
				WatchStream: &WatchStream{
					EventDeliveryTimeout: defaultEventDeliveryTimeout,
					HeartbeatInterval:    defaultHeartbeatInterval,
					Redis: &Redis{
						Address:                "localhost:6379",
						Database:               0,
						StreamName:             defaultRedisStreamName,
						StreamKey:              defaultRedisStreamKey,
						MaxStreamLength:        defaultRedisMaxStreamLength,
						KeyTTL:                 defaultRedisTTL,
						Timeout:                defaultRedisTimeout,
						StreamReadinessTimeout: defaultRedisStreamReadinessTimeout,
					},
				},
				Database: &Database{
					AWS: &AWS{
						SandboxesTableName: "sandboxes",
						NodesTableName:     "nodes",
					},
				},
			},
		},
		{
			name:     "Redis with auth token",
			filename: "redis-auth-token.yaml",
			expectedConfig: &Bundle{
				Base: config.Base{
					Logger: &logger.Config{
						LogLevel: zap.NewAtomicLevelAt(zapcore.InfoLevel),
					},
					GRPCPort: config.DefaultGRPCPort,
					Auth:     &config.Auth{ScopeKey: config.DefaultScopeKey},
					KeepAlive: &config.KeepAlive{
						MinTime:  config.DefaultKeepAliveMinTime,
						Interval: config.DefaultKeepAliveInterval,
						Timeout:  config.DefaultKeepAliveTimeout,
					},
					MaxConcurrentStreams: config.DefaultMaxConcurrentStreams,
				},
				WatchStream: &WatchStream{
					EventDeliveryTimeout: defaultEventDeliveryTimeout,
					HeartbeatInterval:    defaultHeartbeatInterval,
					Redis: &Redis{
						Address:                "localhost:6379",
						Database:               0,
						StreamName:             defaultRedisStreamName,
						StreamKey:              defaultRedisStreamKey,
						MaxStreamLength:        defaultRedisMaxStreamLength,
						KeyTTL:                 defaultRedisTTL,
						Timeout:                defaultRedisTimeout,
						StreamReadinessTimeout: defaultRedisStreamReadinessTimeout,
						AuthToken:              secret.InlineValue("cGFzc3dvcmQ="),
					},
				},
				Database: &Database{
					AWS: &AWS{
						SandboxesTableName: "sandboxes",
						NodesTableName:     "nodes",
					},
				},
			},
		},
		{
			name:     "Redis with TLS",
			filename: "redis-tls.yaml",
			expectedConfig: &Bundle{
				Base: config.Base{
					Logger: &logger.Config{
						LogLevel: zap.NewAtomicLevelAt(zapcore.InfoLevel),
					},
					GRPCPort: config.DefaultGRPCPort,
					Auth:     &config.Auth{ScopeKey: config.DefaultScopeKey},
					KeepAlive: &config.KeepAlive{
						MinTime:  config.DefaultKeepAliveMinTime,
						Interval: config.DefaultKeepAliveInterval,
						Timeout:  config.DefaultKeepAliveTimeout,
					},
					MaxConcurrentStreams: config.DefaultMaxConcurrentStreams,
				},
				WatchStream: &WatchStream{
					EventDeliveryTimeout: defaultEventDeliveryTimeout,
					HeartbeatInterval:    defaultHeartbeatInterval,
					Redis: &Redis{
						Address:                "localhost:6379",
						Database:               0,
						StreamName:             defaultRedisStreamName,
						StreamKey:              defaultRedisStreamKey,
						MaxStreamLength:        defaultRedisMaxStreamLength,
						KeyTTL:                 defaultRedisTTL,
						Timeout:                defaultRedisTimeout,
						StreamReadinessTimeout: defaultRedisStreamReadinessTimeout,
						TLS:                    &RedisTLS{},
					},
				},
				Database: &Database{
					AWS: &AWS{
						SandboxesTableName: "sandboxes",
						NodesTableName:     "nodes",
					},
				},
			},
		},
		{
			name:     "Redis with auth token and TLS",
			filename: "redis-auth-token-tls.yaml",
			expectedConfig: &Bundle{
				Base: config.Base{
					Logger: &logger.Config{
						LogLevel: zap.NewAtomicLevelAt(zapcore.InfoLevel),
					},
					GRPCPort: config.DefaultGRPCPort,
					Auth:     &config.Auth{ScopeKey: config.DefaultScopeKey},
					KeepAlive: &config.KeepAlive{
						MinTime:  config.DefaultKeepAliveMinTime,
						Interval: config.DefaultKeepAliveInterval,
						Timeout:  config.DefaultKeepAliveTimeout,
					},
					MaxConcurrentStreams: config.DefaultMaxConcurrentStreams,
				},
				WatchStream: &WatchStream{
					EventDeliveryTimeout: defaultEventDeliveryTimeout,
					HeartbeatInterval:    defaultHeartbeatInterval,
					Redis: &Redis{
						Address:                "localhost:6379",
						Database:               0,
						StreamName:             defaultRedisStreamName,
						StreamKey:              defaultRedisStreamKey,
						MaxStreamLength:        defaultRedisMaxStreamLength,
						KeyTTL:                 defaultRedisTTL,
						Timeout:                defaultRedisTimeout,
						StreamReadinessTimeout: defaultRedisStreamReadinessTimeout,
						AuthToken:              secret.InlineValue("cGFzc3dvcmQ="),
						TLS:                    &RedisTLS{InsecureSkipVerify: true},
					},
				},
				Database: &Database{
					AWS: &AWS{
						SandboxesTableName: "sandboxes",
						NodesTableName:     "nodes",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualConfig, err := NewFromFile(filepath.Join("testdata", test.filename))
			require.NoError(t, err)
			assert.Equal(t, test.expectedConfig, actualConfig)
		})
	}
}

func TestLoadCertificate(t *testing.T) {
	tls := &config.TLS{SelfSignedCert: true}
	_, err := tls.LoadCertificate()
	if err != nil {
		t.Fatal(err)
	}
}
