package machinery

import (
	"context"
	"errors"
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

// ExitCodeError carries an exit code that should be propagated as the
// CLI's own exit status. `sandbox exec` returns this so the host shell
// observes the same code the guest command did.
type ExitCodeError struct {
	// Code is the exit status the CLI terminates with.
	Code int
	// Err is an optional underlying error for chaining via errors.Is /
	// errors.As. May be nil for a plain status-code propagation.
	Err error
}

func (e *ExitCodeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("command exited with status %d: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("command exited with status %d", e.Code)
}

func (e *ExitCodeError) Unwrap() error { return e.Err }

// errorExitCode maps an error to the exit code the CLI should
// terminate with. Honors *ExitCodeError; falls back to 1 for any other
// non-nil err, and 0 for nil. Pure function so tests don't need to
// invoke os.Exit.
func errorExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	return 1
}

// defaultErrorHandler prints the error to stderr and exits.
func defaultErrorHandler(err error) {
	if err == nil {
		return
	}
	switch {
	case status.Code(err) == codes.Unauthenticated:
		fmt.Fprintln(os.Stderr, "Authentication required. Run `sindri auth login`, then try again.")
	case isExitCodeError(err):
		// The guest already wrote its own stdout/stderr; the CLI just
		// propagates the status code. Quiet exit.
	default:
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
	}
	os.Exit(errorExitCode(err))
}

// isExitCodeError reports whether err wraps an *ExitCodeError.
func isExitCodeError(err error) bool {
	var ec *ExitCodeError
	return errors.As(err, &ec)
}
