package controller

import (
	"sync"

	controlplanev1alpha1 "github.com/alan-ghelardi/yaghan/apis/gen/yaghan/control_plane/v1alpha1"
)

// indexer is a thread-safe in-memory store of the latest observed
// Sandbox proto, keyed by Metadata.Id. Concurrent reconcile workers
// fetch from it; the stream reader writes into it as events arrive.
//
// Out-of-order delivery is handled at insertion time: an incoming
// Sandbox replaces the stored one only when its Metadata.Version is
// strictly higher. Equal-version events are accepted (the api-server
// re-publishes after restart with the same version) so a re-delivered
// event still triggers a reconcile pass.
type indexer struct {
	sandboxes map[string]*controlplanev1alpha1.Sandbox
	mu        sync.RWMutex
}

// newIndexer returns an empty indexer ready to accept Put calls.
func newIndexer() *indexer {
	return &indexer{sandboxes: make(map[string]*controlplanev1alpha1.Sandbox)}
}

// Put stores sandbox under its Metadata.Id and returns true if the
// stored value advanced. Returns false when the incoming sandbox is
// strictly older than what's already indexed; callers should drop the
// associated work-queue item in that case to avoid reconciling stale
// state.
func (i *indexer) Put(sandbox *controlplanev1alpha1.Sandbox) bool {
	id := sandbox.GetMetadata().GetId()
	if id == "" {
		return false
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if existing, ok := i.sandboxes[id]; ok {
		if sandbox.GetMetadata().GetVersion() < existing.GetMetadata().GetVersion() {
			return false
		}
	}
	i.sandboxes[id] = sandbox
	return true
}

// Get returns the indexed sandbox for id, or nil when no entry exists.
func (i *indexer) Get(id string) *controlplanev1alpha1.Sandbox {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.sandboxes[id]
}

// Delete removes an entry. Used when the sandbox reaches a terminal
// phase and no further reconciles are warranted.
func (i *indexer) Delete(id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.sandboxes, id)
}
