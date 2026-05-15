package client

import (
	"context"
	"net"
	"net/http"

	"github.com/alan-ghelardi/yaghan/firecracker-client/client"
	"github.com/alan-ghelardi/yaghan/firecracker-client/client/operations"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

// New creates a new firecracker API client
func New(socketPath string) *client.FirecrackerAPI {
	socketTransport := &http.Transport{
		DialContext: func(_ context.Context, _ string, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	config := client.DefaultTransportConfig()
	transport := httptransport.NewWithClient(config.Host, config.BasePath, config.Schemes, &http.Client{Transport: socketTransport})
	formats := strfmt.Default
	apiClient := new(client.FirecrackerAPI)
	apiClient.Transport = transport
	apiClient.Operations = operations.New(transport, formats)
	return apiClient
}
