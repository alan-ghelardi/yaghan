// Package transport is the host-side byte channel to the in-VM agent.
// It dials the Firecracker vsock UDS, performs the CONNECT handshake,
// and exposes a framed-protobuf Request/Response pipe.
//
// The transport demultiplexes guest responses by AgentResponse.Id so
// concurrent callers do not steal each other's frames. Each logical
// RPC opens a [Conversation] — a small handle bound to a unique id —
// and exchanges messages over it; the underlying read loop fans
// inbound frames out to the matching conversation.
package transport

import (
	"context"
	"io"

	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"
)

// Transport is the host-side channel to one guest agent.
// Implementations are safe for concurrent use; callers should obtain a
// per-RPC [Conversation] via [Transport.OpenConversation] rather than
// touching the wire directly.
type Transport interface {
	io.Closer

	// OpenConversation reserves a fresh request id and returns a
	// Conversation bound to that id. AgentResponses with a matching id
	// flow to Conversation.Recv; everything else is dropped. Open
	// conversations are closed when Transport.Close is invoked.
	OpenConversation() Conversation

	// Err reports the last I/O or framing error observed by the reader
	// goroutine. Nil when Close caused the shutdown or when the read
	// loop is still running.
	Err() error
}

// Conversation is a handle on one logical request/response exchange
// over a [Transport]. It owns a unique request id; Send stamps that id
// onto every outgoing AgentRequest, and Recv yields only AgentResponses
// the agent emits with the matching id.
type Conversation interface {
	io.Closer

	// ID is the request id assigned to this conversation.
	ID() uint64

	// Send writes one framed AgentRequest to the guest. The caller
	// should not set req.Id — implementations override it with the
	// conversation's id before writing.
	Send(ctx context.Context, req *dataplanev1alpha1.AgentRequest) error

	// Recv returns a channel that yields AgentResponses with a matching
	// id. The same channel is returned on every call. It is closed when
	// Close is called or when the underlying transport's read loop
	// exits — inspect [Transport.Err] in the latter case.
	Recv() <-chan *dataplanev1alpha1.AgentResponse
}
