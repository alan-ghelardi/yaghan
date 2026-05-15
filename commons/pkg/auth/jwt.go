package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	authv1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/auth/v1"
	"github.com/alan-ghelardi/yaghan/commons/pkg/config"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	usernameClaim = "username"
)

var (
	errMissingMetadata = status.Errorf(codes.InvalidArgument, "missing metadata")
	errMissingToken    = status.Errorf(codes.Unauthenticated, "missing oauth token")
	errInvalidToken    = status.Errorf(codes.Unauthenticated, "invalid token")
)

// jwtAuthorizer provides authorization for gRPC methods using JWT tokens.
type jwtAuthorizer struct {
	ScopeRegistry

	// Server authentication settings.
	config *config.Auth

	// JWK public keys et used to validate access tokens.
	keySet jwk.Set

	// Options for parsing and verifying JWT tokens.
	options []jwt.ParseOption

	// clock allows us to control the time in unit tests.
	clock jwt.Clock

	mut sync.RWMutex
}

func newJWTAuthorizer(authConfig *config.Auth) *jwtAuthorizer {
	return &jwtAuthorizer{
		config: authConfig,
		clock: jwt.ClockFunc(func() time.Time {
			return time.Now()
		}),
	}
}

// loadKeys loads the signing keys used for validating incoming access tokens.
func (j *jwtAuthorizer) loadKeys(ctx context.Context) error {
	var (
		keySet jwk.Set
		err    error
	)

	if filePath := j.config.SigningKeys.LocalKeyPath; len(filePath) != 0 {
		keySet, err = j.loadLocalKeys(filePath)
	} else if url := j.config.SigningKeys.RemoteKeyURL; len(url) != 0 {
		keySet, err = j.loadRemoteKeys(ctx)
	} else {
		return errors.New("no signing keys are configured")
	}

	if err != nil {
		return err
	}

	if err := j.setJWTOptions(keySet); err != nil {
		return err
	}

	return nil
}

func (j *jwtAuthorizer) setJWTOptions(keySet jwk.Set) error {
	if j.keySet != nil {
		// Fast lane: verify if the keys currently in use are the same as the
		// provided by the key set.
		foundNewKeys := false
		for i := range keySet.Len() {
			key, _ := keySet.Key(i)
			if j.keySet.Index(key) >= 0 {
				foundNewKeys = true
				break
			}
		}

		if !foundNewKeys {
			return nil
		}
	}

	j.mut.Lock()
	defer j.mut.Unlock()

	j.keySet = keySet
	j.options = []jwt.ParseOption{
		jwt.WithClock(j.clock),
		jwt.WithAcceptableSkew(time.Minute),
		jwt.WithRequiredClaim(j.config.ScopeKey),
	}

	// Configure keys
	for i := range j.keySet.Len() {
		key, _ := keySet.Key(i)
		alg, ok := key.Algorithm()
		if !ok {
			id, _ := key.KeyID()
			return fmt.Errorf("missing algorithm in key %s", id)
		}

		j.options = append(j.options, jwt.WithKey(alg, key))
	}

	if aud := j.config.AllowedAudience; len(aud) != 0 {
		j.options = append(j.options, jwt.WithAudience(aud))
	}

	return nil
}

func (j *jwtAuthorizer) loadLocalKeys(filePath string) (jwk.Set, error) {
	keyData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read signing key from %s: %w", filePath, err)
	}

	keySet, err := jwk.Parse(keyData)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid JWK set: %w", filePath, err)
	}

	return keySet, nil
}

func (j *jwtAuthorizer) loadRemoteKeys(ctx context.Context) (jwk.Set, error) {
	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	if err := cache.Register(ctx, j.config.SigningKeys.RemoteKeyURL); err != nil {
		return nil, fmt.Errorf("failed to register URL %s in the cache: %w", j.config.SigningKeys.RemoteKeyURL, err)
	}

	keySet, err := cache.Refresh(ctx, j.config.SigningKeys.RemoteKeyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load JWK keys from %s: %w", j.config.SigningKeys.RemoteKeyURL, err)
	}

	go func() {
		logger := ctxzap.Extract(ctx)
	loop:
		for {
			time.Sleep(10 * time.Second)

			select {
			case <-ctx.Done():
				break loop

			default:
				keySet, err := cache.Lookup(ctx, j.config.SigningKeys.RemoteKeyURL)
				if err != nil {
					logger.Error("Unable to look up JWK keys", zap.Error(err))
				}

				if err := j.setJWTOptions(keySet); err != nil {
					logger.Error("Unable to set JWT options", zap.Error(err))
				}
			}
		}
	}()

	return keySet, nil
}

// Authorize implements Authorizer.
func (j *jwtAuthorizer) Authorize(ctx context.Context, fullMethodName string) (*authv1.Actor, error) {
	requiredScopes, err := j.GetScopes(fullMethodName)
	if err != nil {
		return nil, err
	}
	if requiredScopes.IsEmpty() {
		return nil, nil
	}

	meta, found := metadata.FromIncomingContext(ctx)
	if !found {
		return nil, errMissingMetadata
	}
	authorization, found := meta["authorization"]
	if !found || len(authorization) == 0 {
		return nil, errMissingToken
	}

	token := strings.TrimPrefix(authorization[0], "Bearer ")
	if len(token) == 0 {
		return nil, errInvalidToken
	}

	j.mut.RLock()
	opts := j.options
	j.mut.RUnlock()

	verifiedToken, err := jwt.Parse([]byte(token), opts...)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	actor := parseActor(verifiedToken)

	var scope string
	if err := verifiedToken.Get(j.config.ScopeKey, &scope); err != nil {
		return actor, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := requiredScopes.Check(NewMethodScopes(scope, j.config)); err != nil {
		return actor, err
	}
	return actor, nil
}

func parseActor(token jwt.Token) *authv1.Actor {
	actor := &authv1.Actor{}

	sub, _ := token.Subject()
	actor.ActorId = sub

	var username string
	err := token.Get(usernameClaim, &username)
	if err == nil {
		elems := strings.SplitN(username, "_", 2)
		if len(elems) == 2 {
			actor.AppName = proto.String(elems[0])
			actor.Username = proto.String(elems[1])
		} else if len(username) != 0 {
			actor.Username = &username
		}
	}

	return actor
}
