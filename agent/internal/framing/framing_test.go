package framing

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	dataplanev1alpha1 "golang.nuinfra.net/apis/gen/nuinfra/data_plane/v1alpha1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestRoundtripRequest(t *testing.T) {
	cases := []struct {
		name string
		msg  *dataplanev1alpha1.AgentRequest
	}{
		{
			name: "exec request",
			msg: &dataplanev1alpha1.AgentRequest{
				Id: 1,
				Payload: &dataplanev1alpha1.AgentRequest_ExecRequest{
					ExecRequest: &dataplanev1alpha1.ExecRequest{
						SandboxId: "sb-1",
						Payload: &dataplanev1alpha1.ExecRequest_ExecProcess{
							ExecProcess: &dataplanev1alpha1.ExecProcess{
								Command: "/bin/echo",
								Args:    []string{"hi"},
							},
						},
					},
				},
			},
		},
		{
			name: "cancel",
			msg: &dataplanev1alpha1.AgentRequest{
				Id:      2,
				Payload: &dataplanev1alpha1.AgentRequest_Cancel{Cancel: &dataplanev1alpha1.CancelRequest{}},
			},
		},
		{
			name: "stdin with eof",
			msg: &dataplanev1alpha1.AgentRequest{
				Id: 3,
				Payload: &dataplanev1alpha1.AgentRequest_Stdin{
					Stdin: &dataplanev1alpha1.StdinChunk{
						Data: []byte("payload"),
						Eof:  true,
					},
				},
			},
		},
		{
			name: "empty",
			msg:  &dataplanev1alpha1.AgentRequest{Id: 4},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			require.NoError(t, Write(&buf, tc.msg))

			var got dataplanev1alpha1.AgentRequest
			require.NoError(t, Read(&buf, &got))
			assert.True(t, proto.Equal(tc.msg, &got), "expected %v, got %v", tc.msg, &got)
			assert.Zero(t, buf.Len(), "reader should consume the whole frame")
		})
	}
}

func TestRoundtripResponse(t *testing.T) {
	// StreamChunk is the only Response variant with non-trivial
	// payload — cover it plus a large data case to exercise the
	// length prefix beyond a single varint byte. After the proto
	// refactor, StreamChunk now travels nested inside ExecResponse.
	msg := &dataplanev1alpha1.AgentResponse{
		Id: 9,
		Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
			ExecResponse: &dataplanev1alpha1.ExecResponse{
				Payload: &dataplanev1alpha1.ExecResponse_StreamChunk{
					StreamChunk: &dataplanev1alpha1.StreamChunk{
						Pid:    12345,
						Data:   bytes.Repeat([]byte("x"), 1024),
						Stream: dataplanev1alpha1.StreamChunk_STREAM_TYPE_STDOUT,
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, Write(&buf, msg))

	var got dataplanev1alpha1.AgentResponse
	require.NoError(t, Read(&buf, &got))
	assert.True(t, proto.Equal(msg, &got))
	assert.Zero(t, buf.Len())
}

func TestWriteRejectsOversizePayload(t *testing.T) {
	huge := &dataplanev1alpha1.AgentResponse{
		Id: 1,
		Payload: &dataplanev1alpha1.AgentResponse_ExecResponse{
			ExecResponse: &dataplanev1alpha1.ExecResponse{
				Payload: &dataplanev1alpha1.ExecResponse_StreamChunk{
					StreamChunk: &dataplanev1alpha1.StreamChunk{
						Data: bytes.Repeat([]byte("A"), MaxMessageSize+1),
					},
				},
			},
		},
	}
	err := Write(io.Discard, huge)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMessageTooLarge)
}

func TestReadRejectsOversizeLengthPrefix(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, binary.Write(&buf, binary.BigEndian, uint32(MaxMessageSize+1)))
	// Deliberately provide no body — Read must bail on the size check
	// before attempting to allocate or read the body.

	var got dataplanev1alpha1.AgentRequest
	err := Read(&buf, &got)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMessageTooLarge)
}

func TestReadShortLengthPrefix(t *testing.T) {
	// A single byte is not enough to decode a 4-byte length.
	err := Read(bytes.NewReader([]byte{0x01}), &dataplanev1alpha1.AgentRequest{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, io.EOF, "a partial length prefix is corruption, not a clean EOF")
}

func TestReadEmptyStreamIsEOF(t *testing.T) {
	err := Read(bytes.NewReader(nil), &dataplanev1alpha1.AgentRequest{})
	assert.ErrorIs(t, err, io.EOF)
}

func TestReadShortBody(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, binary.Write(&buf, binary.BigEndian, uint32(16)))
	buf.WriteString("only 8 b") // 8 bytes, short of the 16 declared

	err := Read(&buf, &dataplanev1alpha1.AgentRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read body")
}

func TestWriteErrorPropagates(t *testing.T) {
	errBoom := errors.New("boom")
	err := Write(failingWriter{err: errBoom}, &dataplanev1alpha1.AgentRequest{Id: 1})
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}

// failingWriter returns its configured error on every Write.
type failingWriter struct{ err error }

func (f failingWriter) Write(_ []byte) (int, error) { return 0, f.err }

// Sanity check: the padding we use in TestReadShortBody is indeed short
// of the declared length. Protects against the test drifting silently.
func TestSelfCheck(t *testing.T) {
	const declared = 16
	assert.Less(t, len("only 8 b"), declared)
	assert.True(t, strings.HasPrefix("only 8 b", "only"))
}
