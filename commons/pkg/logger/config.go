package logger

import (
	"encoding/json"
	"reflect"

	"github.com/spf13/viper"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (

	// Default log level when it is not set explicitly.
	defaultLogLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)
)

// Config holds settings for configuring the logger.
// It is intended to be used in more specialized structs
// that provide server or client-specific configurations.
type Config struct {
	// LogLevel overrides the log level configured for the logger.
	LogLevel zap.AtomicLevel `mapstructure:"log-level"`

	// ZapConfig is the Zap logger configuration.
	ZapConfig json.RawMessage `mapstructure:"zap-config"`
}

// SetDefaults set the Viper default configurations for the logger.
func SetDefaults(v *viper.Viper) {
	v.SetDefault("logger.log-level", defaultLogLevel)
}

func AtomicLevelHook(source, target reflect.Type, data any) (any, error) {
	if source.Kind() == reflect.String && target == reflect.TypeOf(zap.AtomicLevel{}) {
		level := data.(string)
		return zap.ParseAtomicLevel(level)
	}
	return data, nil
}

func JSONRawMessageHook(source, target reflect.Type, data any) (any, error) {
	if source.Kind() == reflect.Map && target == reflect.TypeOf(json.RawMessage{}) {
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(jsonBytes), nil
	}
	return data, nil
}
