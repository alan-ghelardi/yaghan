package auth

import (
	"testing"

	"golang.nuinfra.net/commons/pkg/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMethodScopes(t *testing.T) {
	tests := []struct {
		name         string
		scope        string
		config       *config.Auth
		expectedCode codes.Code
	}{
		{
			name:         "request authorized",
			scope:        "foo",
			expectedCode: codes.OK,
		},
		{
			name:         "at least one scope is valid",
			scope:        "baz bla bar",
			expectedCode: codes.OK,
		},
		{
			name:         "various scopes are valid",
			scope:        "foo bar",
			expectedCode: codes.OK,
		},
		{
			name:         "no valid scopes",
			scope:        "baz bla",
			expectedCode: codes.PermissionDenied,
		},
		{
			name:         "normalize scopes before validating the request",
			scope:        "default/foo default/bar",
			config:       &config.Auth{RemoveScopePrefix: "default/"},
			expectedCode: codes.OK,
		},
		{
			name:         "do not remove the scope prefix before validating the request",
			scope:        "default/foo default/bar",
			expectedCode: codes.PermissionDenied,
		},
		{
			name:         "scope prefix does not match the provided scope",
			scope:        "default/foo default/bar",
			config:       &config.Auth{RemoveScopePrefix: "other/"},
			expectedCode: codes.PermissionDenied,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.config == nil {
				test.config = &config.Auth{}
			}

			ms := NewMethodScopes("foo bar", &config.Auth{})
			err := ms.Check(NewMethodScopes(test.scope, test.config))
			if code := status.Code(err); test.expectedCode != code {
				t.Errorf("Check method returned an unexpected status code: expected %v, but got %v instead", test.expectedCode, code)
			}
		})
	}

}
