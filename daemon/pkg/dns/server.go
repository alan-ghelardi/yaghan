package dns

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	miekg "github.com/miekg/dns"
)

// ServerOptions configures a [Server].
type ServerOptions struct {
	// Bind is the address the UDP and TCP listeners attach to,
	// "host:port" form. Default: ":53". Production deployments
	// should restrict this to the host-side veth pool address.
	Bind string

	// Handler is the request handler. Required.
	Handler miekg.Handler

	// ReadTimeout / WriteTimeout bound a single request lifecycle.
	// Defaults: 5s and 5s respectively, matching miekg defaults.
	ReadTimeout, WriteTimeout time.Duration
}

// Server hosts a UDP + TCP listener pair bound to the same address.
// It owns nothing else — handler logic lives in [Handler] and policy
// state in [PolicyStore], so the server itself is a thin lifecycle
// wrapper.
type Server struct {
	udp, tcp *miekg.Server

	mu      sync.Mutex
	started bool
	errCh   chan error
}

// NewServer validates opts and prepares the (not yet started)
// listeners.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Handler == nil {
		return nil, errors.New("Handler is required")
	}
	if opts.Bind == "" {
		opts.Bind = ":53"
	}
	if opts.ReadTimeout <= 0 {
		opts.ReadTimeout = 5 * time.Second
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = 5 * time.Second
	}
	return &Server{
		udp: &miekg.Server{
			Addr:         opts.Bind,
			Net:          "udp",
			Handler:      opts.Handler,
			ReadTimeout:  opts.ReadTimeout,
			WriteTimeout: opts.WriteTimeout,
		},
		tcp: &miekg.Server{
			Addr:         opts.Bind,
			Net:          "tcp",
			Handler:      opts.Handler,
			ReadTimeout:  opts.ReadTimeout,
			WriteTimeout: opts.WriteTimeout,
		},
		errCh: make(chan error, 2),
	}, nil
}

// Start brings both listeners up. The first listener to fail sends
// its error on errCh; the call itself returns nil and lets the caller
// surface the failure via [Server.Wait].
//
// Idempotent: a second Start is a no-op.
func (s *Server) Start(_ context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()

	go func() {
		if err := s.udp.ListenAndServe(); err != nil {
			s.errCh <- fmt.Errorf("udp listener: %w", err)
		}
	}()
	go func() {
		if err := s.tcp.ListenAndServe(); err != nil {
			s.errCh <- fmt.Errorf("tcp listener: %w", err)
		}
	}()
	return nil
}

// Shutdown stops both listeners. Blocks until they have released
// their sockets. Subsequent calls are no-ops.
func (s *Server) Shutdown(ctx context.Context) error {
	var first error
	if err := s.udp.ShutdownContext(ctx); err != nil && first == nil {
		first = fmt.Errorf("udp shutdown: %w", err)
	}
	if err := s.tcp.ShutdownContext(ctx); err != nil && first == nil {
		first = fmt.Errorf("tcp shutdown: %w", err)
	}
	return first
}

// Wait returns the first listener error or blocks until ctx is
// cancelled. Useful in a daemon main where Start kicks off
// background goroutines and the caller wants to surface a panic-grade
// listener failure rather than silently losing DNS.
func (s *Server) Wait(ctx context.Context) error {
	select {
	case err := <-s.errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
