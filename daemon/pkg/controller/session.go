package controller

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// sessionStore reads and persists the daemon's session id. The id is
// the int64 the api-server assigns in the first acknowledgement of an
// EstablishSession stream and accepts back on reconnect to resume from
// the last delivered event.
//
// The store is a single file at the configured path. The id is written
// in plain decimal form for easy operator inspection. Writes are
// atomic (write-tmp-then-rename) so a crash mid-write cannot leave the
// file in a torn state.
type sessionStore struct {
	path string
}

func newSessionStore(path string) *sessionStore {
	return &sessionStore{path: path}
}

// Load returns the persisted session id. A missing file yields (0, nil)
// — the daemon then connects with session_id = 0 to receive a fresh
// id from the server.
func (s *sessionStore) Load() (int64, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read session id file %s: %w", s.path, err)
	}
	id, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse session id from %s: %w", s.path, err)
	}
	return id, nil
}

// Save persists id atomically to the configured path, creating any
// missing parent directories.
func (s *sessionStore) Save(id int64) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("ensure session id directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*")
	if err != nil {
		return fmt.Errorf("create session id tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// Best-effort cleanup if rename never happens.
		_ = os.Remove(tmpPath)
	}()
	if _, err := fmt.Fprintf(tmp, "%d\n", id); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write session id: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close session id tempfile: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename session id file: %w", err)
	}
	return nil
}
