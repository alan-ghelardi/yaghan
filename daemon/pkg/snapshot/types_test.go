package snapshot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	memPayload   = "mem-bytes"
	statePayload = "state-bytes"
)

// fakeDurableStore is a small in-memory recorder used by Store tests.
// It captures the snapshotID/bytes received on Put and, on Get, writes
// pre-seeded payloads into the supplied ContentsWriter. Setting getErr
// short-circuits Get with that error.
type fakeDurableStore struct {
	t *testing.T

	// recorded by Put
	putCalls  int
	gotID     string
	gotMemory []byte
	gotState  []byte

	// served by Get
	memOut   []byte
	stateOut []byte
	getErr   error
	getCalls int
}

func (f *fakeDurableStore) Put(_ context.Context, snapshotID string, contents *ContentsReader) error {
	f.t.Helper()
	f.putCalls++
	f.gotID = snapshotID
	mem, err := io.ReadAll(contents.Memory)
	require.NoError(f.t, err)
	state, err := io.ReadAll(contents.State)
	require.NoError(f.t, err)
	f.gotMemory = mem
	f.gotState = state
	return nil
}

func (f *fakeDurableStore) Get(_ context.Context, _ string, contents *ContentsWriter) error {
	f.t.Helper()
	f.getCalls++
	if f.getErr != nil {
		return f.getErr
	}
	if _, err := contents.Memory.WriteAt(f.memOut, 0); err != nil {
		return err
	}
	if _, err := contents.State.WriteAt(f.stateOut, 0); err != nil {
		return err
	}
	return nil
}

// newStoreFixture wires a Store backed by a fake durable store and returns
// the store, the fake (so tests can drive both sides), and a fresh tempdir
// to use as the chroot argument to MakeLocalReference / Store.Load.
func newStoreFixture(t *testing.T) (*Store, *fakeDurableStore, string) {
	t.Helper()
	fake := &fakeDurableStore{t: t}
	return NewStore(fake), fake, t.TempDir()
}

// writeSnapshotFiles creates the mem/state files that Store.Put expects to
// find on disk, at the paths described by ref, with the canonical payloads.
func writeSnapshotFiles(t *testing.T, ref *LocalReference) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(ref.MemFilePath), 0o755))
	require.NoError(t, os.WriteFile(ref.MemFilePath, []byte(memPayload), 0o600))
	require.NoError(t, os.WriteFile(ref.StateFilePath, []byte(statePayload), 0o600))
}

func TestMemFilePath(t *testing.T) {
	assert.Equal(t, "/snapshots/abc/mem", MemFilePath("abc"))
	id := uuid.NewString()
	assert.Equal(t, "/snapshots/"+id+"/mem", MemFilePath(id))
}

func TestStateFilePath(t *testing.T) {
	assert.Equal(t, "/snapshots/abc/state", StateFilePath("abc"))
	id := uuid.NewString()
	assert.Equal(t, "/snapshots/"+id+"/state", StateFilePath(id))
}

func TestMakeLocalReference(t *testing.T) {
	chroot := t.TempDir()

	ref := MakeLocalReference(chroot)

	require.NotNil(t, ref)
	_, err := uuid.Parse(ref.SnapshotID)
	assert.NoError(t, err, "SnapshotID must be a valid UUID")
	assert.Equal(t, filepath.Join(chroot, "snapshots", ref.SnapshotID, "mem"), ref.MemFilePath)
	assert.Equal(t, filepath.Join(chroot, "snapshots", ref.SnapshotID, "state"), ref.StateFilePath)
}

func TestNewStorePanics(t *testing.T) {
	assert.PanicsWithValue(t, "NewStore: nil durableStore", func() {
		NewStore(nil)
	})
}

func TestStorePut(t *testing.T) {
	store, fake, chroot := newStoreFixture(t)
	ref := MakeLocalReference(chroot)
	writeSnapshotFiles(t, ref)

	require.NoError(t, store.Put(t.Context(), ref))

	assert.Equal(t, 1, fake.putCalls)
	assert.Equal(t, ref.SnapshotID, fake.gotID)
	assert.Equal(t, memPayload, string(fake.gotMemory))
	assert.Equal(t, statePayload, string(fake.gotState))
}

func TestStorePut_MissingLocalFile(t *testing.T) {
	store, _, chroot := newStoreFixture(t)
	ref := MakeLocalReference(chroot)
	// Do NOT write the files.

	err := store.Put(t.Context(), ref)
	assert.Error(t, err)
}

func TestStoreLoad_HappyPath(t *testing.T) {
	store, fake, chroot := newStoreFixture(t)
	fake.memOut = []byte(memPayload)
	fake.stateOut = []byte(statePayload)

	snapshotID := uuid.NewString()
	ref, err := store.Load(t.Context(), chroot, snapshotID)
	require.NoError(t, err)
	require.NotNil(t, ref)
	assert.Equal(t, snapshotID, ref.SnapshotID)

	gotMem, err := os.ReadFile(ref.MemFilePath)
	require.NoError(t, err)
	assert.Equal(t, memPayload, string(gotMem))

	gotState, err := os.ReadFile(ref.StateFilePath)
	require.NoError(t, err)
	assert.Equal(t, statePayload, string(gotState))

	// No .partial leftovers.
	assert.NoFileExists(t, ref.MemFilePath+partialSuffix)
	assert.NoFileExists(t, ref.StateFilePath+partialSuffix)
}

func TestStoreLoad_NoOpWhenLocal(t *testing.T) {
	store, fake, chroot := newStoreFixture(t)
	snapshotID := uuid.NewString()
	ref := localReferenceFor(chroot, snapshotID)
	writeSnapshotFiles(t, ref)

	loaded, err := store.Load(t.Context(), chroot, snapshotID)
	require.NoError(t, err)
	assert.Equal(t, ref.MemFilePath, loaded.MemFilePath)
	assert.Equal(t, ref.StateFilePath, loaded.StateFilePath)
	assert.Zero(t, fake.getCalls, "durable Get must not be called when files exist locally")
}

func TestStoreLoad_NotFound(t *testing.T) {
	store, fake, chroot := newStoreFixture(t)
	fake.getErr = ErrSnapshotNotFound

	snapshotID := uuid.NewString()
	ref, err := store.Load(t.Context(), chroot, snapshotID)
	assert.Nil(t, ref)
	assert.ErrorIs(t, err, ErrSnapshotNotFound)

	// Partial files must not linger after the failed Get.
	memPartial := filepath.Join(chroot, "snapshots", snapshotID, "mem"+partialSuffix)
	statePartial := filepath.Join(chroot, "snapshots", snapshotID, "state"+partialSuffix)
	assert.NoFileExists(t, memPartial)
	assert.NoFileExists(t, statePartial)
}

func TestStoreLoad_PropagatesArbitraryError(t *testing.T) {
	store, fake, chroot := newStoreFixture(t)
	custom := errors.New("network down")
	fake.getErr = custom

	_, err := store.Load(t.Context(), chroot, uuid.NewString())
	assert.ErrorIs(t, err, custom)
}
