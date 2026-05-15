package config

import (
	"time"

	"github.com/alan-ghelardi/yaghan/commons/pkg/config"
	"github.com/alan-ghelardi/yaghan/commons/pkg/secret"
	"github.com/spf13/viper"
)

const (
	defaultEventDeliveryTimeout        = 3 * time.Minute
	defaultRedisStreamName             = "yaghan"
	defaultRedisStreamKey              = "yaghan"
	defaultRedisMaxStreamLength        = 1000
	defaultRedisTTL                    = 24 * time.Hour
	defaultRedisStreamReadinessTimeout = time.Minute
	defaultRedisTimeout                = 30 * time.Second
	defaultHeartbeatInterval           = 30 * time.Second
)

// Bundle defines configuration for the gRPC service and, optionally, the REST gateway.
type Bundle struct {
	config.Base `mapstructure:",squash"`

	// WatchStream sets up the server event stream, allowing daemons to subscribe and receive real-time sandbox change events.
	WatchStream *WatchStream `mapstructure:"watch-stream"`

	// Database contains configurations for the API server's database.
	Database *Database `mapstructure:"database" validate:"required"`
}

// WatchStream sets up the server event stream, allowing daemons to subscribe and receive real-time sandbox change events.
type WatchStream struct {
	// Timeout for delivering each event over the gRPC stream. Defaults to 3 minutes.
	EventDeliveryTimeout time.Duration `mapstructure:"event-delivery-timeout"`

	// HeartbeatInterval is the time between server-sent heartbeat events on idle
	// watch streams. Heartbeats are application-level DATA frames that keep
	// connections alive through load balancers (e.g. ALBs) which ignore HTTP/2
	// PING frames. Defaults to 30 seconds.
	HeartbeatInterval time.Duration `mapstructure:"heartbeat-interval"`

	// Configures Redis as the event stream's backend.
	Redis *Redis
}

// Redis contains configurations to create a connection with a Redis data stream.
type Redis struct {
	// The Redis server's address.
	Address string `validate:"omitempty,hostname_port"`

	// Database's number (0-15). Defaults to 0.
	Database uint `validate:"gte=0,lte=15"`

	// Redis's stream.
	StreamName string `mapstructure:"stream-name" validate:"required"`

	// Redis's stream key.
	StreamKey string `mapstructure:"stream-key" validate:"required"`

	// The approximate maximum number of entries allowed in the Redis stream. Defaults to 1000.
	MaxStreamLength uint64 `mapstructure:"max-stream-length" validate:"gte=10"`

	// How long keys are retained in Redis. Defaults to 24 hours.
	KeyTTL time.Duration `mapstructure:"key-ttl"`

	// Timeout for sending commands to the Redis server. Defaults to 30 seconds.
	Timeout time.Duration

	// Timeout for establishing a new connection with the Redis stream. Defaults to 1 minute.
	StreamReadinessTimeout time.Duration `mapstructure:"stream-readiness-timeout"`

	// AuthToken is an optional secret used to authenticate with the Redis server.
	// When set, its value is loaded and passed as the password in the Redis connection.
	// Supports all secret types: file paths, inline base64 values, and AWS Secrets Manager references.
	AuthToken secret.Secret `mapstructure:"auth-token"`

	// TLS enables TLS for the Redis connection. When set, the client connects over TLS
	// using the system certificate pool. When nil, the connection is plaintext.
	TLS *RedisTLS `mapstructure:"tls"`
}

// RedisTLS controls TLS settings for the Redis connection.
type RedisTLS struct {
	// InsecureSkipVerify disables certificate verification. Intended for testing only.
	InsecureSkipVerify bool `mapstructure:"insecure-skip-verify"`
}

// Database contains configurations for the API server's database.
type Database struct {
	AWS *AWS `mapstructure:"aws" validate:"required"`
}

// AWS specifies required configurations for the AWS services used by the database.
type AWS struct {
	// SandboxesTableName is the DynamoDB table in which sandboxes data is stored.
	SandboxesTableName string `mapstructure:"sandboxes-table-name" validate:"required"`

	// NodesTableName is the DynamoDB table in which nodes data is stored.
	NodesTableName string `mapstructure:"nodes-table-name" validate:"required"`

	// SnapshotsTableName is the DynamoDB table in which snapshots data is stored.
	SnapshotsTableName string `mapstructure:"snapshots-table-name" validate:"required"`

	// Endpoint is the local endpoint URL for AWS services, used for testing
	// purposes.
	EndpointURL string `mapstructure:"endpoint-url"`
}

// NewFromFile loads configuration from the given file into a Bundle.
func NewFromFile(filename string) (*Bundle, error) {
	v := viper.New()
	v.SetDefault("watch-stream.event-delivery-timeout", defaultEventDeliveryTimeout)
	v.SetDefault("watch-stream.redis.stream-name", defaultRedisStreamName)
	v.SetDefault("watch-stream.redis.stream-key", defaultRedisStreamKey)
	v.SetDefault("watch-stream.redis.max-stream-length", defaultRedisMaxStreamLength)
	v.SetDefault("watch-stream.redis.key-ttl", defaultRedisTTL)
	v.SetDefault("watch-stream.redis.timeout", defaultRedisTimeout)
	v.SetDefault("watch-stream.redis.stream-readiness-timeout", defaultRedisStreamReadinessTimeout)
	v.SetDefault("watch-stream.heartbeat-interval", defaultHeartbeatInterval)

	bundle, err := config.NewFromFile[*Bundle](filename, v, secret.DecodeHook)
	if err != nil {
		return nil, err
	}

	// Unset the WatchStream field if the Redis's address was not specified.
	// In practice, this means that no persistent event stream is enabled in the server.
	if len(bundle.WatchStream.Redis.Address) == 0 {
		bundle.WatchStream = nil
	}

	return bundle, nil
}
