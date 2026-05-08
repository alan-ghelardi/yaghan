package service

import (
	"context"
	"errors"

	db "golang.nuinfra.api-server/pkg/db"
	nodedb "golang.nuinfra.api-server/pkg/db/node"
	cpv1 "golang.nuinfra.net/apis/gen/nuinfra/control_plane/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// defaultListNodesPageSize is the page size applied when ListNodes callers
// leave the field unset. Mirrors the documented default in the proto.
const defaultListNodesPageSize int32 = 30

// nodeWriteMaxAttempts caps the read-modify-write retry budget for node
// writes. Conflicts here are rare — the only racing writer in this iteration
// is a future server-side reconciler — so a small bound keeps tail latency
// bounded without livelocking on persistent contention.
const nodeWriteMaxAttempts = 3

// GetNode implements [cpv1.ClusterServiceServer].
func (a *apiServer) GetNode(ctx context.Context, req *cpv1.GetNodeRequest) (*cpv1.GetNodeResponse, error) {
	id := req.GetNodeId()
	n, err := a.nodeDB.Get(ctx, id)
	if err != nil {
		return nil, dbErrToStatus(ctx, "node", "get", id, err)
	}
	return &cpv1.GetNodeResponse{Node: n}, nil
}

// ListNodes implements [cpv1.ClusterServiceServer]. The protovalidate
// interceptor has already enforced the page-size bounds; this layer only
// fills in defaults for fields the client left unset and forwards the rest
// to the DB. Empty results are valid.
func (a *apiServer) ListNodes(ctx context.Context, req *cpv1.ListNodesRequest) (*cpv1.ListNodesResponse, error) {
	opts := nodedb.ListOptions{
		StatusPhase:       req.GetStatusPhase(),
		SortOrder:         req.GetSortOrder(),
		PageSize:          req.GetPageSize(),
		ContinuationToken: req.GetContinuationToken(),
	}
	if opts.PageSize == 0 {
		opts.PageSize = defaultListNodesPageSize
	}
	if opts.SortOrder == cpv1.ListNodesRequest_ORDER_UNSPECIFIED {
		opts.SortOrder = cpv1.ListNodesRequest_ORDER_NEWEST_FIRST
	}

	nodes, nextToken, err := a.nodeDB.List(ctx, opts)
	if err != nil {
		return nil, dbErrToStatus(ctx, "node", "list", "", err)
	}
	return &cpv1.ListNodesResponse{
		Nodes:             nodes,
		ContinuationToken: nextToken,
	}, nil
}

// registerNode persists the node carried in a ConnectionRequest. New nodes
// are created at version 1; reconnecting nodes have their daemon-owned fields
// (resources, provider metadata, status, metrics) refreshed and their version
// bumped, while the server-managed metadata.created_at is preserved.
//
// Bounded retry covers two races:
//   - Two daemons connecting to the same fresh node id concurrently — the
//     loser's create lands on an existing row and falls through to the
//     overlay branch on the next iteration.
//   - A server-side writer (e.g. a future PHASE_LOST reaper) bumping the
//     row's version between our Get and Put.
func (a *apiServer) registerNode(ctx context.Context, payload *cpv1.Node) error {
	nodeID := payload.GetMetadata().GetId()

	for attempt := 0; attempt < nodeWriteMaxAttempts; attempt++ {
		persisted, err := a.nodeDB.Get(ctx, nodeID)
		switch {
		case errors.Is(err, db.ErrNotFound):
			// First registration. Force version 0 so Put hits the create
			// branch; the server stamps created_at and last_modified_at.
			payload.Metadata = &cpv1.NodeMeta{Id: nodeID}
			putErr := a.nodeDB.Put(ctx, payload)
			if putErr == nil {
				return nil
			}
			if errors.Is(putErr, db.ErrAlreadyExists) {
				continue // raced; retry as an overlay
			}
			return putErr

		case err != nil:
			return err

		default:
			persisted.Resources = payload.GetResources()
			persisted.ProviderMetadata = payload.GetProviderMetadata()
			persisted.Status = payload.GetStatus()
			persisted.Metrics = payload.GetMetrics()
			putErr := a.nodeDB.Put(ctx, persisted)
			if putErr == nil {
				return nil
			}
			if errors.Is(putErr, db.ErrVersionConflict) {
				continue
			}
			return putErr
		}
	}
	return db.ErrVersionConflict
}

// applyNodePatch applies a single PatchNodeRequest using server-side
// read-modify-write. Last-writer-wins is intentional for metrics and status
// — the freshest sample is the most accurate, and CAS would do the opposite.
// The retry loop tolerates concurrent writers within the same budget as
// registerNode.
func (a *apiServer) applyNodePatch(ctx context.Context, nodeID string, req *cpv1.PatchNodeRequest) error {
	if req.GetPatch() == nil {
		return status.Error(codes.InvalidArgument,
			"PatchNodeRequest must set node_metrics or node_status")
	}

	for attempt := 0; attempt < nodeWriteMaxAttempts; attempt++ {
		node, err := a.nodeDB.Get(ctx, nodeID)
		if err != nil {
			return err
		}
		switch p := req.GetPatch().(type) {
		case *cpv1.PatchNodeRequest_NodeMetrics:
			node.Metrics = p.NodeMetrics
		case *cpv1.PatchNodeRequest_NodeStatus:
			node.Status = p.NodeStatus
		}
		putErr := a.nodeDB.Put(ctx, node)
		if putErr == nil {
			return nil
		}
		if errors.Is(putErr, db.ErrVersionConflict) {
			continue
		}
		return putErr
	}
	return db.ErrVersionConflict
}
