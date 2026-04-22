package controller

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	cpmocks "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1/mocks"
	"golang.nuinfra.net/daemon/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// fakeStream implements ClusterService_EstablishSessionClient. Tests
// inject server-to-client frames via push; the stream reader pops
// them through Recv. Client-to-server traffic is captured on sent
// for assertions.
type fakeStream struct {
	ctx    context.Context
	sent   chan *cpv1.EstablishSessionRequest
	recvCh chan recvFrame
}

type recvFrame struct {
	resp *cpv1.EstablishSessionResponse
	err  error
}

func newFakeStream(ctx context.Context) *fakeStream {
	return &fakeStream{
		ctx:    ctx,
		sent:   make(chan *cpv1.EstablishSessionRequest, 32),
		recvCh: make(chan recvFrame, 32),
	}
}

func (s *fakeStream) push(resp *cpv1.EstablishSessionResponse) {
	s.recvCh <- recvFrame{resp: resp}
}

// Send / Recv / ClientStream surface.
func (s *fakeStream) Send(req *cpv1.EstablishSessionRequest) error {
	s.sent <- req
	return nil
}

func (s *fakeStream) Recv() (*cpv1.EstablishSessionResponse, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case frame, ok := <-s.recvCh:
		if !ok {
			return nil, errors.New("stream closed")
		}
		return frame.resp, frame.err
	}
}

func (s *fakeStream) Header() (metadata.MD, error) { return nil, nil }
func (s *fakeStream) Trailer() metadata.MD         { return nil }
func (s *fakeStream) CloseSend() error             { return nil }
func (s *fakeStream) Context() context.Context     { return s.ctx }
func (s *fakeStream) SendMsg(any) error            { return nil }
func (s *fakeStream) RecvMsg(any) error            { return nil }

// fakeReconciler is a single-method stub for the Reconciler interface.
type fakeReconciler struct {
	mu  sync.Mutex
	fn  func(ctx context.Context, sandbox *cpv1.Sandbox) error
	got []*cpv1.Sandbox
}

func (f *fakeReconciler) Reconcile(ctx context.Context, sandbox *cpv1.Sandbox) error {
	f.mu.Lock()
	f.got = append(f.got, proto.CloneOf(sandbox))
	fn := f.fn
	f.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(ctx, sandbox)
}

func newFixtureBundle(t *testing.T) *config.Bundle {
	return &config.Bundle{
		AssetsDir:   "/assets",
		Firecracker: &config.Firecracker{},
		MicroVM:     &config.MicroVM{},
		APIServer:   &config.APIServer{Address: "127.0.0.1:0"},
		Controller: &config.Controller{
			SessionIDFile:        filepath.Join(t.TempDir(), "session.id"),
			MaxRetries:           3,
			ReconnectMaxInterval: 10 * time.Millisecond,
		},
	}
}

func sandboxFor(id string, version int64, phase cpv1.SandboxStatus_Phase) *cpv1.Sandbox {
	return &cpv1.Sandbox{
		Metadata: &cpv1.SandboxMeta{Id: id, Version: version},
		Status:   &cpv1.SandboxStatus{Phase: phase},
		Intent:   &cpv1.Intent{Phase: cpv1.SandboxStatus_PHASE_RUNNING},
	}
}

// receiveSent pulls one client-to-server frame off the fake stream
// with a generous timeout so test failures show the surrounding
// goroutine state rather than just hanging.
func receiveSent(t *testing.T, stream *fakeStream) *cpv1.EstablishSessionRequest {
	t.Helper()
	select {
	case req := <-stream.sent:
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("controller did not send anything within the timeout")
		return nil
	}
}

func TestController_ConnectSendsConnectionRequestAndPersistsSessionID(t *testing.T) {
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	client := cpmocks.NewMockClusterServiceClient(ctrl)
	rec := &fakeReconciler{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := newFakeStream(ctx)
	client.EXPECT().EstablishSession(gomock.Any()).Return(stream, nil)

	c := New(client, rec, bundle)

	// Pre-load the server's acknowledgement so connect() returns.
	stream.push(&cpv1.EstablishSessionResponse{
		Message: &cpv1.EstablishSessionResponse_Acknowledge{
			Acknowledge: &cpv1.ConnectionResponse{SessionId: 12345},
		},
	})

	got, err := c.connect(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)

	connectReq := receiveSent(t, stream)
	require.NotNil(t, connectReq.GetConnect(), "first frame must be a ConnectionRequest")
	assert.Zero(t, connectReq.GetConnect().GetSessionId(), "fresh session must send session_id=0")
	require.NotNil(t, connectReq.GetConnect().GetNode())

	// The new id must be persisted.
	store := newSessionStore(bundle.Controller.SessionIDFile)
	id, loadErr := store.Load()
	require.NoError(t, loadErr)
	assert.Equal(t, int64(12345), id)
}

func TestController_ConnectResumesWithPersistedSessionID(t *testing.T) {
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	client := cpmocks.NewMockClusterServiceClient(ctrl)

	// Pre-populate the session file.
	require.NoError(t, newSessionStore(bundle.Controller.SessionIDFile).Save(99))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := newFakeStream(ctx)
	client.EXPECT().EstablishSession(gomock.Any()).Return(stream, nil)

	c := New(client, &fakeReconciler{}, bundle)

	stream.push(&cpv1.EstablishSessionResponse{
		Message: &cpv1.EstablishSessionResponse_Acknowledge{
			Acknowledge: &cpv1.ConnectionResponse{SessionId: 99},
		},
	})

	_, err := c.connect(ctx)
	require.NoError(t, err)

	connectReq := receiveSent(t, stream)
	require.NotNil(t, connectReq.GetConnect())
	assert.Equal(t, int64(99), connectReq.GetConnect().GetSessionId(),
		"persisted session id must be sent on connect for resume")
}

func TestController_DispatchEnqueuesNewerEventsOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	client := cpmocks.NewMockClusterServiceClient(ctrl)
	c := New(client, &fakeReconciler{}, bundle)

	c.dispatch(&cpv1.Event{
		InvolvedObject: &cpv1.Event_Sandbox{Sandbox: sandboxFor("sb-1", 1, cpv1.SandboxStatus_PHASE_PENDING)},
	})
	c.dispatch(&cpv1.Event{
		// Older version — must NOT enqueue.
		InvolvedObject: &cpv1.Event_Sandbox{Sandbox: sandboxFor("sb-1", 0, cpv1.SandboxStatus_PHASE_PENDING)},
	})
	c.dispatch(&cpv1.Event{
		// Newer — replaces the indexer entry but the queue dedups, so
		// the queue length stays at 1 (key already pending).
		InvolvedObject: &cpv1.Event_Sandbox{Sandbox: sandboxFor("sb-1", 2, cpv1.SandboxStatus_PHASE_PENDING)},
	})

	assert.Equal(t, 1, c.queue.Len(), "queue should hold one deduplicated key")

	got := c.indexer.Get("sb-1")
	require.NotNil(t, got)
	assert.Equal(t, int64(2), got.GetMetadata().GetVersion(),
		"indexer must reflect the highest version observed")
}

func TestController_ProcessItemSendsUpdateOnDiff(t *testing.T) {
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	client := cpmocks.NewMockClusterServiceClient(ctrl)

	rec := &fakeReconciler{
		fn: func(_ context.Context, sandbox *cpv1.Sandbox) error {
			// Reconciler advances the status to mirror the intent.
			sandbox.Status = &cpv1.SandboxStatus{
				Phase:   cpv1.SandboxStatus_PHASE_RUNNING,
				Message: "Sandbox is running",
			}
			sandbox.Intent = nil
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := New(client, rec, bundle)
	c.indexer.Put(sandboxFor("sb-1", 1, cpv1.SandboxStatus_PHASE_PENDING))
	c.setStream(newFakeStream(ctx))

	require.NoError(t, c.processItem(ctx, "sb-1"))

	stream := c.stream.(*fakeStream)
	updateReq := receiveSent(t, stream)
	update := updateReq.GetUpdateSandbox()
	require.NotNil(t, update, "expected an UpdateSandboxRequest")
	assert.Equal(t, "sb-1", update.GetSandbox().GetMetadata().GetId())
	assert.Equal(t, cpv1.SandboxStatus_PHASE_RUNNING, update.GetSandbox().GetStatus().GetPhase())
	assert.Nil(t, update.GetSandbox().GetIntent(), "reconciler must zero intent on success")
}

func TestController_ProcessItemSkipsUpdateWhenNoDiff(t *testing.T) {
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	client := cpmocks.NewMockClusterServiceClient(ctrl)

	rec := &fakeReconciler{} // no-op reconciler — sandbox unchanged

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := New(client, rec, bundle)
	c.indexer.Put(sandboxFor("sb-1", 1, cpv1.SandboxStatus_PHASE_RUNNING))
	stream := newFakeStream(ctx)
	c.setStream(stream)

	require.NoError(t, c.processItem(ctx, "sb-1"))

	select {
	case unexpected := <-stream.sent:
		t.Fatalf("expected no UpdateSandbox when reconciler is a no-op, got %+v", unexpected)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestController_ProcessItemSurfacesReconcilerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	client := cpmocks.NewMockClusterServiceClient(ctrl)

	wantErr := errors.New("boom")
	rec := &fakeReconciler{
		fn: func(context.Context, *cpv1.Sandbox) error { return wantErr },
	}

	c := New(client, rec, bundle)
	c.indexer.Put(sandboxFor("sb-err", 1, cpv1.SandboxStatus_PHASE_PENDING))

	err := c.processItem(context.Background(), "sb-err")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestController_GiveUpSendsPhaseFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	bundle := newFixtureBundle(t)
	client := cpmocks.NewMockClusterServiceClient(ctrl)

	rec := &fakeReconciler{
		fn: func(context.Context, *cpv1.Sandbox) error { return errors.New("never works") },
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := New(client, rec, bundle)
	c.indexer.Put(sandboxFor("sb-fail", 1, cpv1.SandboxStatus_PHASE_PENDING))
	stream := newFakeStream(ctx)
	c.setStream(stream)

	// Drive processNext directly until it gives up. With MaxRetries=3,
	// processNext returns true after each attempt; on the give-up call
	// it pushes one PHASE_FAILED update and returns.
	c.queue.Add("sb-fail")
	for i := 0; i <= bundle.Controller.MaxRetries; i++ {
		if !c.processNext(ctx) {
			t.Fatalf("processNext returned false at iteration %d", i)
		}
	}

	updateReq := receiveSent(t, stream)
	update := updateReq.GetUpdateSandbox()
	require.NotNil(t, update)
	assert.Equal(t, cpv1.SandboxStatus_PHASE_FAILED, update.GetSandbox().GetStatus().GetPhase())
	assert.Contains(t, update.GetSandbox().GetStatus().GetMessage(), "never works")

	// Second send must NOT happen — we forgot the item.
	select {
	case unexpected := <-stream.sent:
		t.Fatalf("expected no further sends after give-up, got %+v", unexpected)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestController_DispatchIgnoresEventWithoutSandbox(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(cpmocks.NewMockClusterServiceClient(ctrl), &fakeReconciler{}, newFixtureBundle(t))

	c.dispatch(&cpv1.Event{}) // no Sandbox oneof

	assert.Zero(t, c.queue.Len())
}

func TestController_SendUpdateFailsWhenStreamMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	c := New(cpmocks.NewMockClusterServiceClient(ctrl), &fakeReconciler{}, newFixtureBundle(t))
	// stream is nil by default — sendUpdate must fail without dialing.
	err := c.sendUpdate(sandboxFor("sb", 1, cpv1.SandboxStatus_PHASE_RUNNING))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// Compile-time assertion that fakeStream satisfies the bidi-stream
// interface; if it doesn't the test build fails at this line rather
// than deep inside a test.
var _ grpc.BidiStreamingClient[cpv1.EstablishSessionRequest, cpv1.EstablishSessionResponse] = (*fakeStream)(nil)
