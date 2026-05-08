package testing

import (
	"io"
	"regexp"
	"strings"
	"testing"

	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	dpmocks "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1/mocks"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"golang.nuinfra.net/ctl/pkg/cli"
	climocks "golang.nuinfra.net/ctl/pkg/cli/mocks"
)

// NewContext creates a cli.Context object for testing purposes.
//
// The Prompter field is wired to a gomock-generated mock so commands
// that drive interactive flows (e.g. paginated table output) can
// configure expectations via
// ctx.Prompter.(*climocks.MockPrompter).EXPECT(). Tests that
// don't touch the prompter are unaffected — gomock only fails on
// actual unexpected calls.
func NewContext(t *testing.T) *cli.Context {
	mockCtrl := gomock.NewController(t)

	return &cli.Context{
		ClientSet: &cli.ClientSet{
			ClusterService: cpmocks.NewMockClusterServiceClient(mockCtrl),
			SandboxService: cpmocks.NewMockSandboxServiceClient(mockCtrl),
			DaemonService:  dpmocks.NewMockDaemonServiceClient(mockCtrl),
		},
		Prompter: climocks.NewMockPrompter(mockCtrl),
		IOStreams: &cli.IOStreams{
			Stdin:  strings.NewReader(""),
			Stdout: new(strings.Builder),
			Stderr: new(strings.Builder),
		},
		Fatal: func(err error) {
			t.Fatal(err)
		},
	}
}

// AssertStdoutIsEqual asserts that the stdout's content is equal to the
// expected value.
func AssertStdoutIsEqual(ctx *cli.Context, t *testing.T, expected string) {
	t.Helper()
	assert.Equal(t, expected, Read(t, ctx.IOStreams.Stdout), "mismatch in the standard output")
}

// Read reads the writer's content.
func Read(t *testing.T, writer io.Writer) string {
	t.Helper()

	switch b := writer.(type) {
	case *strings.Builder:
		return b.String()

	default:
		t.Fatalf("the supplied io.Writer must be a *strings.Builder, not a %T", writer)
	}
	return ""
}

// SplitArgs splits a string representing command line arguments into a slice of tokens.
func SplitArgs(args string) []string {
	return regexp.MustCompile(`\s+`).Split(args, -1)
}
