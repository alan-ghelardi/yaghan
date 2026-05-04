package exec

// This file exposes a few package-private symbols to the _test.go
// black-box test package without leaking them into the public API.
//
// Compiled only under `go test`, so the production binary still sees
// only the exported surface.

// SendResizeForTest is a test-only re-export of sendResize. The exec
// package's tests are in package exec_test (black-box) and need to
// drive the resize helper directly against the in-package
// fakeExecStream — without leaking the helper into the public API,
// which is the whole point of keeping it lowercase.
func SendResizeForTest(stream execStreamSender, sandboxID string, cols, rows uint32) error {
	return sendResize(stream, sandboxID, cols, rows)
}
