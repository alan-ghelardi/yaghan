package metrics_test

import (
	"testing"

	"github.com/alan-ghelardi/yaghan/daemon/pkg/config"
	"github.com/alan-ghelardi/yaghan/daemon/pkg/node/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector(t *testing.T) {
	c := metrics.NewCollector(&config.Bundle{})
	sample, err := c.Collect(t.Context())
	require.NoError(t, err)

	// This is essentially a smoke test to make sure that our collector
	// extracts coherent metrics without failures.
	assert.NotZero(t, sample.CPUCapacityMillicores)
	assert.NotZero(t, sample.CPUUsedMillicores)
	assert.Less(t, sample.CPUUsedMillicores, sample.CPUCapacityMillicores)
	assert.NotZero(t, sample.MemoryCapacityBytes)
	assert.NotZero(t, sample.MemoryUsedBytes)
	assert.Less(t, sample.MemoryUsedBytes, sample.MemoryCapacityBytes)
	assert.NotZero(t, sample.DiskCapacityBytes)
	assert.NotZero(t, sample.DiskUsedBytes)
	assert.Less(t, sample.DiskUsedBytes, sample.DiskCapacityBytes)
}
