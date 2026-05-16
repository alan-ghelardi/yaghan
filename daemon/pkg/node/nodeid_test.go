package node

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeIDStore_LoadMissingReturnsEmpty(t *testing.T) {
	store := newNodeIDStore(filepath.Join(t.TempDir(), "node.id"))
	id, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, id, "missing file should yield an empty id (first-run signal)")
}

func TestNodeIDStore_SaveThenLoadRoundTrips(t *testing.T) {
	store := newNodeIDStore(filepath.Join(t.TempDir(), "node.id"))
	require.NoError(t, store.Save("abc-123"))

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "abc-123", got)
}

func TestNodeIDStore_SaveCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tree", "node.id")
	store := newNodeIDStore(path)
	require.NoError(t, store.Save("xyz"))

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "xyz", got)
}

func TestNodeIDStore_SaveOverwritesExisting(t *testing.T) {
	store := newNodeIDStore(filepath.Join(t.TempDir(), "node.id"))
	require.NoError(t, store.Save("first"))
	require.NoError(t, store.Save("second"))

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "second", got)
}
