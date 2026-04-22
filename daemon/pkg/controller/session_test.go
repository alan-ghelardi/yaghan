package controller

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_LoadMissingReturnsZero(t *testing.T) {
	store := newSessionStore(filepath.Join(t.TempDir(), "session.id"))
	id, err := store.Load()
	require.NoError(t, err)
	assert.Zero(t, id, "missing file should yield session id 0")
}

func TestSessionStore_SaveThenLoadRoundTrips(t *testing.T) {
	store := newSessionStore(filepath.Join(t.TempDir(), "session.id"))
	require.NoError(t, store.Save(7777))

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, int64(7777), got)
}

func TestSessionStore_SaveCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tree", "session.id")
	store := newSessionStore(path)
	require.NoError(t, store.Save(1))

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)
}

func TestSessionStore_SaveOverwritesExisting(t *testing.T) {
	store := newSessionStore(filepath.Join(t.TempDir(), "session.id"))
	require.NoError(t, store.Save(1))
	require.NoError(t, store.Save(2))

	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, int64(2), got)
}
