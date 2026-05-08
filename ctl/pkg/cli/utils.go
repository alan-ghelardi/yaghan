package cli

import (
	"fmt"
	"slices"
	"strings"

	"buf.build/go/protoyaml"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	flagOutputFormat = "output-format"

	OutputFormatJSON  = "json"
	OutputFormatYAML  = "yaml"
	OutputFormatTable = "table"
)

// AddOneOfFlag defines a flag that only accepts a predefined list of values.
//
// The flag is registered as a string flag and appends the list of allowed values
// to the usage description.
//
// Example:
//
//	AddOneOfFlag(cmd, "format", "f", "json", "Specify the output format", []string{"json", "yaml", "table"})
//
// This results in a flag description like:
//
//	--format string   Specify the output format. One of: (json, yaml, table).
func AddOneOfFlag(cmd *cobra.Command, name, shorthand, value, usage string, allowedValues []string) *string {
	return cmd.Flags().StringP(name, shorthand, value, fmt.Sprintf("%s. One of: (%s).", usage, strings.Join(allowedValues, ", ")))
}

// AddOutputFormatFlag defines the --output-format (-o) flag.
func AddOutputFormatFlag(cmd *cobra.Command, defaultValue string, outputFormats ...string) {
	AddOneOfFlag(cmd, flagOutputFormat, "o", defaultValue, "Output format", outputFormats)
}

// GetOneOfFlag retrieves the value of a flag and ensures it is one of the allowed values.
//
// If the flag value is not in the list of valid options, an error is returned.
//
// Example usage:
//
//	format, err := GetOneOfFlag(cmd, "format", []string{"json", "yaml", "table"})
//	if err != nil {
//	    return err // Handle invalid flag value
//	}
func GetOneOfFlag(cmd *cobra.Command, name string, allowedValues []string) (string, error) {
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", err
	}

	// Validate against allowed values
	if slices.Contains(allowedValues, value) {
		return value, nil
	}

	return "", fmt.Errorf("invalid value %q for --%s. Allowed values: %s", value, name, strings.Join(allowedValues, ", "))
}

// GetOutputFormat returns the value passed to the --output-format flag.
func GetOutputFormat(cmd *cobra.Command, validFormats ...string) (string, error) {
	return GetOneOfFlag(cmd, flagOutputFormat, validFormats)
}

// Marshal marshals the given `proto.Message` in the specified output format.
func Marshal(message proto.Message, outputFormat string) (output []byte, err error) {
	switch outputFormat {
	case OutputFormatJSON:
		// Serialize to JSON using protojson
		options := protojson.MarshalOptions{
			Indent:        "  ", // Pretty-print JSON
			UseProtoNames: true,
		}
		output, err = options.Marshal(message)

	case OutputFormatYAML:
		// Serialize to YAML using protoyaml
		output, err = protoyaml.Marshal(message)

	default:
		err = fmt.Errorf("unsupported output format %q", outputFormat)
	}

	return
}
