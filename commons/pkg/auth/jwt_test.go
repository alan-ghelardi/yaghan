package auth

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/metadata"

	"net/http"
	"net/http/httptest"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authv1 "golang.nuinfra.net/apis/gen/nuinfra/auth/v1"
	authtesting "golang.nuinfra.net/commons/pkg/auth/testing"
	"golang.nuinfra.net/commons/pkg/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	createFooMethod   = "/dev.example.v1alpha1.Service/CreateFoo"
	getBarMethod      = "/dev.example.v1alpha1.Service/GetBar"
	healthCheckMethod = "/dev.example.v1alpha1.Service/HealthCheck"
	createFooScope    = "foo:create"
	getBarScope       = "bar:get"
	testClientScope   = "bar:get foo:get"
	scopeKey          = "scope"
)

func TestAuthorizer(t *testing.T) {
	tests := []struct {
		name          string
		methodName    string
		addMetadata   bool
		addOauthToken bool
		claims        authtesting.Claims
		clock         jwt.Clock
		config        *config.Auth
		expectedCode  codes.Code
		expectedActor *authv1.Actor
	}{
		{
			name:         "authorize public methods",
			methodName:   healthCheckMethod,
			addMetadata:  false,
			expectedCode: codes.OK,
		},
		{
			name:         "missing metadata in a non-public method call",
			methodName:   createFooMethod,
			expectedCode: codes.InvalidArgument,
		},
		{
			name:         "missing oauth token",
			methodName:   createFooMethod,
			addMetadata:  true,
			expectedCode: codes.Unauthenticated,
		},
		{
			name:          "missing scope claim",
			methodName:    createFooMethod,
			addMetadata:   true,
			addOauthToken: true,
			expectedCode:  codes.Unauthenticated,
		},
		{
			name:          "insufficient scope",
			methodName:    createFooMethod,
			addMetadata:   true,
			addOauthToken: true,
			claims: authtesting.Claims{
				scopeKey: testClientScope,
			},
			expectedCode:  codes.PermissionDenied,
			expectedActor: &authv1.Actor{},
		},
		{
			name:          "request authorized",
			methodName:    getBarMethod,
			addMetadata:   true,
			addOauthToken: true,
			claims: authtesting.Claims{
				scopeKey: testClientScope,
			},
			expectedCode:  codes.OK,
			expectedActor: &authv1.Actor{},
		},
		{
			name:          "request made by an automation tool",
			methodName:    getBarMethod,
			addMetadata:   true,
			addOauthToken: true,
			claims: authtesting.Claims{
				scopeKey: testClientScope,
				"sub":    "42",
			},
			expectedCode:  codes.OK,
			expectedActor: &authv1.Actor{ActorId: "42"},
		},
		{
			name:          "request made by human",
			methodName:    getBarMethod,
			addMetadata:   true,
			addOauthToken: true,
			claims: authtesting.Claims{
				scopeKey:      testClientScope,
				"sub":         "42",
				usernameClaim: "my-app_john_doe@example.com",
			},
			expectedCode: codes.OK,
			expectedActor: &authv1.Actor{
				ActorId:  "42",
				AppName:  new("my-app"),
				Username: new("john_doe@example.com"),
			},
		},
		{
			name:          "token has expired",
			methodName:    getBarMethod,
			addMetadata:   true,
			addOauthToken: true,
			claims: authtesting.Claims{
				scopeKey: testClientScope,
			},
			clock: jwt.ClockFunc(func() time.Time {
				// The fake token expires after five minutes
				return time.Now().Add(time.Hour)
			}),
			expectedCode: codes.Unauthenticated,
		},
		{
			name:          "unexpected audience",
			methodName:    getBarMethod,
			addMetadata:   true,
			addOauthToken: true,
			claims: authtesting.Claims{
				"aud":    "foo",
				scopeKey: testClientScope,
			},
			config: &config.Auth{
				AllowedAudience: "bar",
				ScopeKey:        scopeKey,
			},
			expectedCode: codes.Unauthenticated,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.config == nil {
				test.config = &config.Auth{ScopeKey: scopeKey}
			}

			token, keySet := authtesting.SignedJWTTokenAndKeySet(t, test.claims)

			authorizer := newJWTAuthorizer(test.config)
			if test.clock != nil {
				authorizer.clock = test.clock
			}

			if err := authorizer.setJWTOptions(keySet); err != nil {
				t.Fatal(err)
			}

			authorizer.saveScopes(healthCheckMethod, NewMethodScopes("", &config.Auth{}))
			authorizer.saveScopes(createFooMethod, NewMethodScopes(createFooScope, &config.Auth{}))
			authorizer.saveScopes(getBarMethod, NewMethodScopes(getBarScope, &config.Auth{}))

			ctx := ctxzap.ToContext(t.Context(), zaptest.NewLogger(t))
			if test.addMetadata {
				var md metadata.MD
				if test.addOauthToken {
					md = metadata.Pairs("Authorization", fmt.Sprintf("Bearer %s", token))
				}
				ctx = metadata.NewIncomingContext(ctx, md)
			}

			actor, err := authorizer.Authorize(ctx, test.methodName)
			if err != nil {
				st, _ := status.FromError(err)
				t.Logf("Authorizer returned the following error: %v - %v", err, st.Details())
			}

			require.Equal(t, test.expectedCode, status.Code(err))
			assert.Equal(t, test.expectedActor, actor)
		})
	}
}

func TestLoadKeys(t *testing.T) {
	_, keySet := authtesting.SignedJWTTokenAndKeySet(t, authtesting.Claims{})
	localKeyPath := authtesting.WriteKey(t, keySet)

	ctx := ctxzap.ToContext(t.Context(), zaptest.NewLogger(t))

	t.Run("load local key", func(t *testing.T) {
		authorizer := newJWTAuthorizer(&config.Auth{
			SigningKeys: &config.SigningKeys{
				LocalKeyPath: localKeyPath,
			},
		})
		if err := authorizer.loadKeys(ctx); err != nil {
			t.Fatal(err)
		}

		if authorizer.keySet == nil {
			t.Error("No JWK key is set")
		}
	})

	t.Run("load remote key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			err := json.NewEncoder(w).Encode(keySet)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}))

		t.Cleanup(server.Close)

		authorizer := newJWTAuthorizer(&config.Auth{
			SigningKeys: &config.SigningKeys{
				RemoteKeyURL: server.URL,
			},
		})
		if err := authorizer.loadKeys(ctx); err != nil {
			t.Fatal(err)
		}

		if authorizer.keySet == nil {
			t.Error("No JWK key is set")
		}
	})
}
