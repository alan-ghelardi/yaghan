package logger

import (
	jsonpatch "github.com/evanphx/json-patch"

	"encoding/json"

	_ "embed"

	"go.uber.org/zap"
)

//go:embed zap-config.json
var defaultLoggerConfig []byte

// New creates a new zap.Logger using the provided configuration.
// If config is nil, it creates a logger with default settings.
func New(loggerName string, config *Config) (*zap.Logger, error) {
	// Use default config if none provided
	if config == nil {
		config = &Config{
			LogLevel: zap.NewAtomicLevelAt(zap.InfoLevel),
		}
	}

	baseConfig := defaultLoggerConfig

	if userProvidedConfig := config.ZapConfig; len(userProvidedConfig) != 0 {
		patch, err := jsonpatch.CreateMergePatch(baseConfig, userProvidedConfig)
		if err != nil {
			return nil, err
		}

		baseConfig, err = jsonpatch.MergePatch(defaultLoggerConfig, patch)
		if err != nil {
			return nil, err
		}
	}

	zapConfig := &zap.Config{}
	if err := json.Unmarshal(baseConfig, zapConfig); err != nil {
		return nil, err
	}

	zapConfig.Level = config.LogLevel

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, err
	}

	return logger.Named(loggerName), nil
}
