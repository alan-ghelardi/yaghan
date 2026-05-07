package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.nuinfra.net/daemon/pkg/config"
)

func TestMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector(&config.Bundle{})
	metrics, err := mc.Collect(t.Context())
	require.NoError(t, err)

	// This is essentially a smoke test to make sure that our collector
	// extracts coherent metrics without failures.
	assert.NotZero(t, metrics.CPUCapacityMillicores)
	assert.NotZero(t, metrics.CPUUsedMillicores)
	assert.Less(t, metrics.CPUUsedMillicores, metrics.CPUCapacityMillicores)
	assert.NotZero(t, metrics.MemoryCapacityBytes)
	assert.NotZero(t, metrics.MemoryUsedBytes)
	assert.Less(t, metrics.MemoryUsedBytes, metrics.MemoryCapacityBytes)
	assert.NotZero(t, metrics.DiskCapacityBytes)
	assert.NotZero(t, metrics.DiskUsedBytes)
	assert.Less(t, metrics.DiskUsedBytes, metrics.DiskCapacityBytes)
}
