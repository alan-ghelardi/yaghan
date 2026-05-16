package node

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// nodeIDStore reads and persists the daemon's node identifier in local
// runtime. The id is generated as a UUID on first run and written here
// so subsequent restarts re-register with the api-server under the
// same identity. In EC2 runtime the IMDS instance id is authoritative
// and this store is bypassed entirely.
//
// Writes are atomic (write-tmp-then-rename); a crash mid-write cannot
// leave the file in a torn state. Shape mirrors
// controller.sessionStore.
type nodeIDStore struct {
	path string
}

func newNodeIDStore(path string) *nodeIDStore {
	return &nodeIDStore{path: path}
}

// Load returns the persisted node id. A missing file yields ("", nil)
// so callers can treat that as "first run" and generate a fresh id.
func (s *nodeIDStore) Load() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read node id file %s: %w", s.path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// Save persists id atomically to the configured path, creating any
// missing parent directories.
func (s *nodeIDStore) Save(id string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("ensure node id directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".*")
	if err != nil {
		return fmt.Errorf("create node id tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := fmt.Fprintf(tmp, "%s\n", id); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write node id: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close node id tempfile: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename node id file: %w", err)
	}
	return nil
}
