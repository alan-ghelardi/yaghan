package service

import (
	"sync"

	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
)

// nodeRegistry tracks Nodes that currently hold an active EstablishSession.
// Entries are keyed by node id and tagged with the watcher (session) id that
// owns them so a stale-resume cleanup cannot evict a fresh session that took
// over the same node.
type nodeRegistry struct {
	mu      sync.Mutex
	entries map[string]nodeEntry
}

type nodeEntry struct {
	node      *cpv1.Node
	watcherID int64
}

func newNodeRegistry() *nodeRegistry {
	return &nodeRegistry{entries: make(map[string]nodeEntry)}
}

// put records that the supplied node owns the session identified by watcherID.
// A subsequent call for the same node id overwrites the previous entry — this
// matches the WatchableStream's "replace existing watcher with same id"
// behaviour.
func (r *nodeRegistry) put(node *cpv1.Node, watcherID int64) {
	id := node.GetMetadata().GetId()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[id] = nodeEntry{node: node, watcherID: watcherID}
}

// deleteIfOwned removes the entry for nodeID only if the recorded watcherID
// matches. This prevents an exiting handler from clearing an entry that has
// already been claimed by a newer session that resumed under the same id.
func (r *nodeRegistry) deleteIfOwned(nodeID string, watcherID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[nodeID]; ok && entry.watcherID == watcherID {
		delete(r.entries, nodeID)
	}
}
