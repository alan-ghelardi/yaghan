package auth

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	authv1 "golang.nuinfra.net/apis/gen/nuinfra/auth/v1"
	"golang.nuinfra.net/commons/pkg/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Authorizer provides authorization for GRPC methods.
type Authorizer interface {
	// Authorize checks the incoming context for an access token in the
	// request metadata and performs authorization for the gRPC method if
	// applicable.  It returns an error if the authorization fails or the
	// actor who initiated the operation if the request is authorized.
	Authorize(ctx context.Context, fullMethodName string) (*authv1.Actor, error)
}

// passThrough is an Authorizer that let all request pass through with no
// checks.
type passThrough struct {
}

var _ Authorizer = (*passThrough)(nil)

// Authorize implements Authorizer.
func (p *passThrough) Authorize(context.Context, string) (*authv1.Actor, error) {
	return nil, nil
}

// NewAuthorizer creates a new Authorizer from the provided config.
func NewAuthorizer(ctx context.Context, config *config.Auth) (Authorizer, error) {
	logger := ctxzap.Extract(ctx)

	if config == nil || config.SigningKeys == nil {
		logger.Warn("authentication is not configured on the server - all incoming requests will be processed without authorization checks")
		return &passThrough{}, nil
	}

	authorizer := newJWTAuthorizer(config)
	if err := authorizer.loadKeys(ctx); err != nil {
		return nil, err
	}
	return authorizer, nil
}

// ScopeRegistry offers efficient access to scopes defined in gRPC methods.
// This type can be embedded by authorizers to gain the capability
// of inspecting the scopes required by specific methods.
type ScopeRegistry struct {

	// Map of full GRPC method names to the scopes required by them.
	scopesByMethod map[string]*MethodScopes

	mut sync.RWMutex
}

// GetScopes returns the scopes required by a given GRPC method.
func (s *ScopeRegistry) GetScopes(fullMethodName string) (*MethodScopes, error) {
	if len(fullMethodName) == 0 {
		return nil, errors.New("empty method name")
	}

	s.mut.RLock()
	if s.scopesByMethod == nil {
		s.scopesByMethod = make(map[string]*MethodScopes)
	}
	scopes, found := s.scopesByMethod[fullMethodName]
	s.mut.RUnlock()

	if found {
		return scopes, nil
	}

	// Treat the full method name to adhere to the format expected by
	// protoregistry
	methodName := fullMethodName[1:]
	methodName = strings.ReplaceAll(methodName, "/", ".")
	descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(methodName))
	if err != nil {
		s.saveScopes(fullMethodName, nil)
		return nil, nil
	}

	method := descriptor.(protoreflect.MethodDescriptor)
	ext := proto.GetExtension(method.Options(), authv1.E_RequiredScopes)
	if ext == nil {
		s.saveScopes(fullMethodName, nil)
		return nil, nil
	}

	scp, ok := ext.(string)
	if !ok {
		return nil, status.Errorf(codes.Internal, "invalid scopes declared in the method %s: expected string, but got %T", fullMethodName, ext)
	}

	scopes = NewMethodScopes(scp, &config.Auth{})
	s.saveScopes(fullMethodName, scopes)
	return scopes, nil
}

// saveScopes stores the provided scopes in memory to avoid a new lookup in the
// proto registry.
func (s *ScopeRegistry) saveScopes(fullMethodName string, scopes *MethodScopes) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.scopesByMethod == nil {
		s.scopesByMethod = make(map[string]*MethodScopes)
	}
	s.scopesByMethod[fullMethodName] = scopes
}

// MethodScopes represents a set of scopes required by gRPC methods.
type MethodScopes struct {
	scopeSet  map[string]bool
	scopeList []string
}

func NewMethodScopes(scopes string, config *config.Auth) *MethodScopes {
	scopeList := strings.Fields(scopes)
	if prefix := config.RemoveScopePrefix; len(prefix) != 0 {
		normalizedScopes := make([]string, 0, len(scopeList))
		for _, scope := range scopeList {
			normalizedScopes = append(normalizedScopes, strings.TrimPrefix(scope, prefix))
		}
		scopeList = normalizedScopes
	}

	ms := &MethodScopes{
		scopeSet:  make(map[string]bool, len(scopeList)),
		scopeList: scopeList,
	}
	for _, scope := range scopeList {
		ms.scopeSet[scope] = true
	}
	return ms
}

func (m *MethodScopes) IsEmpty() bool {
	return len(m.scopeSet) == 0
}

func (m *MethodScopes) Check(scopes *MethodScopes) error {
	insuficientScopes := &authv1.InsufficientScopes{
		RequiredScopes: m.scopeList,
		ClientScopes:   scopes.scopeList,
	}

	for scope := range scopes.scopeSet {
		if m.scopeSet[scope] {
			return nil
		}
	}

	status := status.New(codes.PermissionDenied, "insufficient scopes for the requested operation")
	detailedStatus, err := status.WithDetails(insuficientScopes)
	if err != nil {
		return status.Err()
	}
	return detailedStatus.Err()
}
