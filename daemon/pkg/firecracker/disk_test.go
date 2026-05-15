package firecracker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResizeRootfsZeroOrNegativeIsNoop(t *testing.T) {
	f := writeTempFile(t, 1024)
	for _, n := range []int64{0, -1} {
		assert.NoError(t, resizeRootfs(f, n))
	}
	fi, err := os.Stat(f)
	require.NoError(t, err)
	assert.EqualValues(t, 1024, fi.Size(), "no-op preserved size")
}

func TestResizeRootfsRefusesShrink(t *testing.T) {
	// 10 MiB file; asking for 4 MiB must refuse without touching the file.
	f := writeTempFile(t, 10*1024*1024)
	err := resizeRootfs(f, 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to shrink")
	fi, err := os.Stat(f)
	require.NoError(t, err)
	assert.EqualValues(t, 10*1024*1024, fi.Size(), "size unchanged on refused shrink")
}

func TestResizeRootfsExactSizeIsNoop(t *testing.T) {
	// 4 MiB exactly; asking for 4 MiB should skip both truncate and resize2fs.
	f := writeTempFile(t, 4*1024*1024)
	require.NoError(t, resizeRootfs(f, 4))
	fi, err := os.Stat(f)
	require.NoError(t, err)
	assert.EqualValues(t, 4*1024*1024, fi.Size())
}

// writeTempFile creates an empty file of the requested size in t.TempDir
// and registers cleanup. Used by the size-validation tests where the
// file contents don't matter — only the file's reported size does.
func writeTempFile(t *testing.T, size int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rootfs.bin")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(size))
	require.NoError(t, f.Close())
	return path
}
