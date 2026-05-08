package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "nil error",
			err:  nil,
			want: 0,
		},
		{
			name: "ExitCodeError forwards its Code",
			err:  &ExitCodeError{Code: 42},
			want: 42,
		},
		{
			name: "wrapped ExitCodeError still forwards via errors.As",
			err:  fmt.Errorf("wrapped: %w", &ExitCodeError{Code: 7}),
			want: 7,
		},
		{
			name: "plain error falls back to 1",
			err:  errors.New("oops"),
			want: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, errorExitCode(tc.err))
		})
	}
}

func TestExitCodeError_ErrorAndUnwrap(t *testing.T) {
	bare := &ExitCodeError{Code: 9}
	assert.Equal(t, "command exited with status 9", bare.Error())
	assert.Nil(t, bare.Unwrap())

	cause := errors.New("guest panic")
	wrapped := &ExitCodeError{Code: 11, Err: cause}
	assert.Contains(t, wrapped.Error(), "status 11")
	assert.Contains(t, wrapped.Error(), "guest panic")
	assert.Same(t, cause, wrapped.Unwrap())
}
