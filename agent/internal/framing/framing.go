// Package framing is a minimal length-prefixed protobuf codec used by
// the agent's vsock protocol. Each message is a 4-byte big-endian size
// followed by the marshalled proto bytes.
package framing

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

// MaxMessageSize bounds any single decoded message. It exists to keep a
// corrupted or hostile length prefix from triggering a multi-gigabyte
// allocation. 4 MiB is deliberately generous for the agent's workloads
// (stdout chunks are 4 KiB, control messages are tiny).
const MaxMessageSize = 4 * 1024 * 1024

// ErrMessageTooLarge is returned when a message would exceed
// [MaxMessageSize], either on encode or after reading the length prefix.
var ErrMessageTooLarge = errors.New("framing: message exceeds MaxMessageSize")

// Write marshals msg and writes it to w as a length-prefixed frame.
func Write(w io.Writer, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("framing: marshal: %w", err)
	}
	if len(data) > MaxMessageSize {
		return fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, len(data))
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return fmt.Errorf("framing: write length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("framing: write body: %w", err)
	}
	return nil
}

// Read reads one length-prefixed frame from r and unmarshals it into msg.
// It returns an error — including [io.EOF] without wrapping — when the
// length prefix or body cannot be fully read, or when the declared size
// exceeds [MaxMessageSize].
func Read(r io.Reader, msg proto.Message) error {
	var size uint32
	if err := binary.Read(r, binary.BigEndian, &size); err != nil {
		// Surface io.EOF unwrapped so callers can treat a clean stream
		// end the same way they'd treat it from any io.Reader.
		if errors.Is(err, io.EOF) {
			return err
		}
		return fmt.Errorf("framing: read length: %w", err)
	}
	if size > MaxMessageSize {
		return fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("framing: read body: %w", err)
	}
	if err := proto.Unmarshal(buf, msg); err != nil {
		return fmt.Errorf("framing: unmarshal: %w", err)
	}
	return nil
}
