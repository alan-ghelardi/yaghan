package controller

import (
	"testing"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sandboxAt(id string, version int64) *controlplanev1alpha1.Sandbox { //nolint:unparam // id stays a parameter so tests asserting on multiple ids stay readable
	return &controlplanev1alpha1.Sandbox{
		Metadata: &controlplanev1alpha1.SandboxMeta{Id: id, Version: version},
	}
}

func TestIndexer_PutNewerVersionWins(t *testing.T) {
	idx := newIndexer()

	require.True(t, idx.Put(sandboxAt("sb-1", 1)))
	require.True(t, idx.Put(sandboxAt("sb-1", 2)))

	got := idx.Get("sb-1")
	require.NotNil(t, got)
	assert.Equal(t, int64(2), got.GetMetadata().GetVersion())
}

func TestIndexer_PutOlderVersionRejected(t *testing.T) {
	idx := newIndexer()

	require.True(t, idx.Put(sandboxAt("sb-1", 5)))
	assert.False(t, idx.Put(sandboxAt("sb-1", 4)),
		"older version must not displace the indexed entry")

	got := idx.Get("sb-1")
	require.NotNil(t, got)
	assert.Equal(t, int64(5), got.GetMetadata().GetVersion())
}

func TestIndexer_PutEqualVersionAccepted(t *testing.T) {
	// The api-server may republish the same version after a restart;
	// re-running the reconcile pass is harmless and idempotent.
	idx := newIndexer()

	require.True(t, idx.Put(sandboxAt("sb-1", 3)))
	assert.True(t, idx.Put(sandboxAt("sb-1", 3)))
}

func TestIndexer_PutRejectsEmptyID(t *testing.T) {
	idx := newIndexer()
	assert.False(t, idx.Put(&controlplanev1alpha1.Sandbox{Metadata: &controlplanev1alpha1.SandboxMeta{}}))
}

func TestIndexer_GetMissingIsNil(t *testing.T) {
	idx := newIndexer()
	assert.Nil(t, idx.Get("nope"))
}

func TestIndexer_Delete(t *testing.T) {
	idx := newIndexer()
	require.True(t, idx.Put(sandboxAt("sb-1", 1)))
	idx.Delete("sb-1")
	assert.Nil(t, idx.Get("sb-1"))
}
