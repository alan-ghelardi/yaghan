package testing

import (
	"testing"

	"github.com/sivchari/kumo"
)

// StartEmulator starts an in-process AWS emulator server. The returned URL
// should be used for local endpoint resolution.
func StartEmulator(t *testing.T) (endpointURL string) {
	t.Helper()

	server := kumo.NewServer()
	t.Cleanup(server.Close)
	return server.URL
}
