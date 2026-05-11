package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

const (
	snapshotsDir  = "snapshots"
	memSnapFile   = "mem"
	stateSnapFile = "state"
	partialSuffix = ".partial"

	// DirMode matches firecrackerVM.CreateSnapshot's host-side
	// mkdir so a Load doesn't change permissions from what the jailer
	// expects on the same path.
	DirMode = 0o755
)

// ErrSnapshotNotFound is returned when a snapshot cannot be found in durable storage.
var ErrSnapshotNotFound = errors.New("snapshot not found")

// LocalReference identifies snapshot files stored on the local filesystem.
type LocalReference struct {
	// SnapshotID is the snapshot identifier.
	SnapshotID string

	// MemFilePath is the local filesystem path to the snapshot memory file.
	MemFilePath string

	// StateFilePath is the local filesystem path to the snapshot state file.
	StateFilePath string
}

// ContentsReader bundles the readable sides of a snapshot's artifacts. It
// is passed to [DurableStore.Put] so implementations can stream each
// artifact straight to remote storage without buffering.
type ContentsReader struct {
	// Memory is the snapshot memory file source.
	Memory io.Reader

	// State is the snapshot state file source.
	State io.Reader
}

// ContentsWriter bundles the writable sides of a snapshot's artifacts.
// [DurableStore.Get] writes each artifact directly into the supplied
// io.WriterAt, which lets the implementation download parts concurrently
// without buffering the full payload in memory.
type ContentsWriter struct {
	// Memory receives the snapshot memory file bytes.
	Memory io.WriterAt

	// State receives the snapshot state file bytes.
	State io.WriterAt
}

// DurableStore represents durable remote storage for snapshot files,
// such as Amazon S3 or Google Cloud Storage.
type DurableStore interface {
	// Put persists the snapshot contents into durable storage under the given snapshot ID.
	Put(ctx context.Context, snapshotID string, contents *ContentsReader) error

	// Get retrieves the snapshot contents for the given snapshot ID, writing
	// each artifact into the corresponding field of contents. It returns
	// [ErrSnapshotNotFound] when no snapshot with snapshotID exists.
	Get(ctx context.Context, snapshotID string, contents *ContentsWriter) error
}

// Store provides access to snapshot files stored locally and in durable remote storage.
type Store struct {
	durableStore DurableStore
}

// NewStore constructs a new Store object.
func NewStore(durableStore DurableStore) *Store {
	if durableStore == nil {
		panic("NewStore: nil durableStore")
	}
	return &Store{durableStore: durableStore}
}

// MakeLocalReference returns a LocalReference for a freshly-generated
// snapshot id rooted at chroot.
func MakeLocalReference(chroot string) *LocalReference {
	return localReferenceFor(chroot, uuid.NewString())
}

// localReferenceFor builds a LocalReference for an existing snapshot id
// under chroot. Shared by MakeLocalReference and Store.Load.
func localReferenceFor(chroot, snapshotID string) *LocalReference {
	return &LocalReference{
		SnapshotID:    snapshotID,
		MemFilePath:   filepath.Join(chroot, MemFilePath(snapshotID)),
		StateFilePath: filepath.Join(chroot, StateFilePath(snapshotID)),
	}
}

// Put reads the snapshot files referenced by localRef and persists them into
// the durable storage.
func (s *Store) Put(ctx context.Context, localRef *LocalReference) error {
	memFile, err := os.Open(localRef.MemFilePath)
	if err != nil {
		return fmt.Errorf("open mem file: %w", err)
	}
	defer memFile.Close()

	stateFile, err := os.Open(localRef.StateFilePath)
	if err != nil {
		return fmt.Errorf("open state file: %w", err)
	}
	defer stateFile.Close()

	if err := s.durableStore.Put(ctx, localRef.SnapshotID, &ContentsReader{
		Memory: memFile,
		State:  stateFile,
	}); err != nil {
		return fmt.Errorf("put snapshot %s: %w", localRef.SnapshotID, err)
	}
	return nil
}

// Load retrieves the snapshot from durable storage and copies its contents
// into local storage under chroot.
//
// It returns a LocalReference describing the local file paths for the snapshot.
// If the snapshot is already present locally, Load is a no-op.
// It returns ErrSnapshotNotFound when the snapshot does not exist in durable storage.
func (s *Store) Load(ctx context.Context, chroot, snapshotID string) (*LocalReference, error) {
	ref := localReferenceFor(chroot, snapshotID)

	if exists(ref.MemFilePath) && exists(ref.StateFilePath) {
		return ref, nil
	}

	if err := os.MkdirAll(filepath.Dir(ref.MemFilePath), DirMode); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}

	memPartial := ref.MemFilePath + partialSuffix
	statePartial := ref.StateFilePath + partialSuffix

	memFile, err := os.OpenFile(memPartial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create partial mem file: %w", err)
	}
	stateFile, err := os.OpenFile(statePartial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = memFile.Close()
		_ = os.Remove(memPartial)
		return nil, fmt.Errorf("create partial state file: %w", err)
	}

	getErr := s.durableStore.Get(ctx, snapshotID, &ContentsWriter{
		Memory: memFile,
		State:  stateFile,
	})

	// Close before rename so all writes are flushed; record any close
	// error but prefer the Get error if both fired.
	memCloseErr := memFile.Close()
	stateCloseErr := stateFile.Close()

	if getErr != nil {
		_ = os.Remove(memPartial)
		_ = os.Remove(statePartial)
		return nil, getErr
	}
	if err := errors.Join(memCloseErr, stateCloseErr); err != nil {
		_ = os.Remove(memPartial)
		_ = os.Remove(statePartial)
		return nil, fmt.Errorf("close partial files: %w", err)
	}

	if err := os.Rename(memPartial, ref.MemFilePath); err != nil {
		_ = os.Remove(memPartial)
		_ = os.Remove(statePartial)
		return nil, fmt.Errorf("rename mem file: %w", err)
	}
	if err := os.Rename(statePartial, ref.StateFilePath); err != nil {
		_ = os.Remove(statePartial)
		return nil, fmt.Errorf("rename state file: %w", err)
	}
	return ref, nil
}

// MemFilePath returns the absolute path to the snapshot memory file
// inside the jailer's chroot environment.
func MemFilePath(snapshotID string) string {
	return filepath.Join("/", snapshotsDir, snapshotID, memSnapFile)
}

// StateFilePath returns the absolute path to the snapshot state file
// inside the jailer's chroot environment.
func StateFilePath(snapshotID string) string {
	return filepath.Join("/", snapshotsDir, snapshotID, stateSnapFile)
}

// exists reports whether a regular file is present at path.
func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
