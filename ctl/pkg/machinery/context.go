package machinery

import (
	"context"
	"fmt"
	"io"
	"os"

	controlplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Context provides a convenient indirection to inject components into commands.
type Context struct {
	// ClientSet provides access to gRPC clients of control and data plane services.
	ClientSet *ClientSet

	// IOStreams contains I/O streams (stdin, stdout and stderr).
	IOStreams *IOStreams

	// Fatal is a function to handle uncaught errors.
	Fatal func(err error)
}

type ClientSet struct {
	ClusterService controlplanev1alpha1.ClusterServiceClient
	SandboxService controlplanev1alpha1.SandboxServiceClient
	DaemonService  dataplanev1alpha1.DaemonServiceClient
}

// IOStreams contains input and output streams.
type IOStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// NewContext creates a new Context object.
func NewContext(_ context.Context) *Context {
	clientSet := newClientSet()
	Streams := &IOStreams{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	return &Context{
		ClientSet: clientSet,
		IOStreams: Streams,
		Fatal:     defaultErrorHandler,
	}
}

// newClientSet ...
func newClientSet() *ClientSet {
	grpcOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	apiServerConn, err := grpc.NewClient("localhost:9090", grpcOpts...)
	if err != nil {
		defaultErrorHandler(fmt.Errorf("client set: %w", err))
	}
	daemonConn, err := grpc.NewClient("localhost:9091", grpcOpts...)
	if err != nil {
		defaultErrorHandler(fmt.Errorf("client set: %w", err))
	}
	return &ClientSet{
		ClusterService: controlplanev1alpha1.NewClusterServiceClient(apiServerConn),
		SandboxService: controlplanev1alpha1.NewSandboxServiceClient(apiServerConn),
		DaemonService:  dataplanev1alpha1.NewDaemonServiceClient(daemonConn),
	}
}

// defaultErrorHandler prints the error to stderr and exits.
func defaultErrorHandler(err error) {
	if err == nil {
		return
	}
	if status.Code(err) == codes.Unauthenticated {
		fmt.Fprintln(os.Stderr, "Authentication required. Run `sindri auth login`, then try again.")
	} else {
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
	}
	os.Exit(1)
}
