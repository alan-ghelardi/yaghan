package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"
	"time"

	"github.com/alan-ghelardi/yaghan/commons/pkg/logger"
	"github.com/alan-ghelardi/yaghan/commons/pkg/utilities"
	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

const (
	DefaultGRPCPort             = 8394
	DefaultScopeKey             = "scope"
	DefaultKeepAliveMinTime     = 15 * time.Second
	DefaultKeepAliveInterval    = 30 * time.Second
	DefaultKeepAliveTimeout     = 20 * time.Second
	DefaultMaxConcurrentStreams = math.MaxUint32
)

// Base contains common configuration fields intended to be embedded in
// more specialized structs that configure a gRPC server.
type Base struct {
	// Logger configures the Zap logger used by the server.
	Logger *logger.Config

	// Port the GRPC server listens on. Defaults to 8394 if not specified.
	GRPCPort uint `mapstructure:"grpc-port"`

	// Port the REST gateway listens on. Set to 0 to disable the REST
	// gateway.
	RestPort uint `mapstructure:"rest-port"`

	// Path to the OpenAPI v2 document to be served by the REST gateway. It
	// has no effect if the REST gateway is disabled.
	OpenAPIV2Path string `mapstructure:"open-api-v2-path"`

	// Auth configures server-side authentication for incoming requests.
	// If unset, the server does not perform any authentication and allows all requests.
	Auth *Auth

	// TLS configures server-side transport security.
	// If unset, the server accepts insecure connections.
	TLS *TLS

	// KeepAlive holds settings to configure the `keepalive` in HTTP2 connections.
	KeepAlive *KeepAlive `mapstructure:"keep-alive"`

	// Maximum number of gRPC concurrent streams.
	MaxConcurrentStreams uint32 `mapstructure:"max-concurrent-streams"`
}

// Auth configures the server-side authentication.
type Auth struct {
	// If set, restricts OAuth verification to this audience only.
	AllowedAudience string `mapstructure:"allowed-audience"`

	// The key in the JWT token claims that contains client scopes to verify.
	// Defaults to `scope` if unspecified.
	ScopeKey string `mapstructure:"scope-key"`

	// Removes the specified prefix from scopes extracted from JWT tokens before
	// validating incoming requests. This is useful for OAuth providers like
	// Amazon Cognito, which prefix scopes with the resource server's identifier.
	RemoveScopePrefix string `mapstructure:"remove-scope-prefix"`

	// SigningKeys configures the public keys used for validating oauth tokens.
	SigningKeys *SigningKeys `mapstructure:"signing-keys"`
}

// SigningKeys configures the sources of public keys used for validating OAuth tokens.
type SigningKeys struct {
	// LocalKeyPath specifies the path to a local file containing the signing key.
	LocalKeyPath string `mapstructure:"local-key-path"`

	// RemoteKeyURL specifies the URL of an endpoint to retrieve remote signing keys.
	RemoteKeyURL string `mapstructure:"remote-key-url"`
}

// TLS configures transport security in the server.
type TLS struct {

	// SelfSignedCert configures the server to generate and return a
	// self-signed certificate in TLS connections.
	SelfSignedCert bool `mapstructure:"self-signed-cert"`
}

// LoadCertificate returns a certificate to be used in secure
// connections. Currently, it returns a self-signed certificate for
// testing purposes. In the future, this function may be extended to
// load valid certificates from other sources.
func (t *TLS) LoadCertificate() (*tls.Certificate, error) {
	if t.SelfSignedCert {
		return genSelfSignedCert()
	}
	return nil, errors.New("no certificate is configured")
}

func genSelfSignedCert() (*tls.Certificate, error) {
	// Generate a private key
	pri, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create a template for the certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Nubank"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(100 * 365 * 24 * time.Hour), // Valid for 100 years
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &pri.PublicKey, pri)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	ecPrivateKey, err := x509.MarshalECPrivateKey(pri)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privateKey := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecPrivateKey})
	certificate, err := tls.X509KeyPair(cert, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate and private key: %w", err)
	}
	return &certificate, nil
}

// KeepAlive holds settings to configure the `keepalive` in HTTP2 connections.
type KeepAlive struct {

	// Minimum amount of time a client should wait before sending a
	// keepalive ping.
	MinTime time.Duration `mapstructure:"min-time"`

	Interval time.Duration `mapstructure:"interval"`

	Timeout time.Duration `mapstructure:"timeout"`
}

// NewFromFile reads configurations from the specified file.
func NewFromFile[T any](filename string, v *viper.Viper, decoders ...mapstructure.DecodeHookFunc) (obj T, err error) {
	v.SetConfigFile(filename)

	// Set defaults
	logger.SetDefaults(v)
	v.SetDefault("grpc-port", DefaultGRPCPort)
	v.SetDefault("auth.scope-key", DefaultScopeKey)
	v.SetDefault("keep-alive.min-time", DefaultKeepAliveMinTime)
	v.SetDefault("keep-alive.interval", DefaultKeepAliveInterval)
	v.SetDefault("keep-alive.timeout", DefaultKeepAliveTimeout)
	v.SetDefault("max-concurrent-streams", DefaultMaxConcurrentStreams)

	// Read config
	if err := v.ReadInConfig(); err != nil {
		return obj, fmt.Errorf("error reading config file: %w", err)
	}

	// Decode the configuration into the target object
	config := utilities.NewObject[T]()
	decoderFuncs := make([]mapstructure.DecodeHookFunc, 0, 3+len(decoders))
	decoderFuncs = append(decoderFuncs,
		logger.AtomicLevelHook,
		logger.JSONRawMessageHook,
		mapstructure.StringToTimeDurationHookFunc(),
	)
	decoderFuncs = append(decoderFuncs, decoders...)
	if err := v.UnmarshalExact(config, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(decoderFuncs...))); err != nil {
		return obj, fmt.Errorf("error parsing config file: %w", err)
	}

	// Validate config
	validate := validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := field.Tag.Get("mapstructure")
		if len(name) == 0 {
			return strings.ToLower(field.Name)
		}
		return name
	})
	if err := validate.Struct(config); err != nil {
		return obj, fmt.Errorf("invalid server configuration: %w", err)
	}

	return config, nil
}
