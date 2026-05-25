# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [yaghan/auth/v1/auth.proto](#yaghan_auth_v1_auth-proto)
    - [Actor](#yaghan-auth-v1-Actor)
    - [InsufficientScopes](#yaghan-auth-v1-InsufficientScopes)
  
    - [File-level Extensions](#yaghan_auth_v1_auth-proto-extensions)
  
- [yaghan/control_plane/v1alpha1/sandbox.proto](#yaghan_control_plane_v1alpha1_sandbox-proto)
    - [CreateSandboxRequest](#yaghan-control_plane-v1alpha1-CreateSandboxRequest)
    - [CreateSandboxResponse](#yaghan-control_plane-v1alpha1-CreateSandboxResponse)
    - [DeleteSandboxRequest](#yaghan-control_plane-v1alpha1-DeleteSandboxRequest)
    - [DeleteSandboxResponse](#yaghan-control_plane-v1alpha1-DeleteSandboxResponse)
    - [EgressPolicy](#yaghan-control_plane-v1alpha1-EgressPolicy)
    - [EgressTargets](#yaghan-control_plane-v1alpha1-EgressTargets)
    - [GetSandboxRequest](#yaghan-control_plane-v1alpha1-GetSandboxRequest)
    - [GetSandboxResponse](#yaghan-control_plane-v1alpha1-GetSandboxResponse)
    - [Intent](#yaghan-control_plane-v1alpha1-Intent)
    - [ListSandboxesRequest](#yaghan-control_plane-v1alpha1-ListSandboxesRequest)
    - [ListSandboxesResponse](#yaghan-control_plane-v1alpha1-ListSandboxesResponse)
    - [NodeRef](#yaghan-control_plane-v1alpha1-NodeRef)
    - [PauseSandboxRequest](#yaghan-control_plane-v1alpha1-PauseSandboxRequest)
    - [PauseSandboxResponse](#yaghan-control_plane-v1alpha1-PauseSandboxResponse)
    - [Resources](#yaghan-control_plane-v1alpha1-Resources)
    - [ResumeSandboxRequest](#yaghan-control_plane-v1alpha1-ResumeSandboxRequest)
    - [ResumeSandboxResponse](#yaghan-control_plane-v1alpha1-ResumeSandboxResponse)
    - [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox)
    - [SandboxMeta](#yaghan-control_plane-v1alpha1-SandboxMeta)
    - [SandboxMeta.LabelsEntry](#yaghan-control_plane-v1alpha1-SandboxMeta-LabelsEntry)
    - [SandboxSource](#yaghan-control_plane-v1alpha1-SandboxSource)
    - [SandboxStatus](#yaghan-control_plane-v1alpha1-SandboxStatus)
    - [SnapshotOutput](#yaghan-control_plane-v1alpha1-SnapshotOutput)
    - [StartSnapshotInput](#yaghan-control_plane-v1alpha1-StartSnapshotInput)
    - [StartSnapshotRequest](#yaghan-control_plane-v1alpha1-StartSnapshotRequest)
    - [StartSnapshotResponse](#yaghan-control_plane-v1alpha1-StartSnapshotResponse)
  
    - [ListSandboxesRequest.Order](#yaghan-control_plane-v1alpha1-ListSandboxesRequest-Order)
    - [SandboxStatus.Phase](#yaghan-control_plane-v1alpha1-SandboxStatus-Phase)
  
    - [SandboxService](#yaghan-control_plane-v1alpha1-SandboxService)
  
- [yaghan/control_plane/v1alpha1/cluster.proto](#yaghan_control_plane_v1alpha1_cluster-proto)
    - [ConnectionRequest](#yaghan-control_plane-v1alpha1-ConnectionRequest)
    - [ConnectionResponse](#yaghan-control_plane-v1alpha1-ConnectionResponse)
    - [EC2InstanceMeta](#yaghan-control_plane-v1alpha1-EC2InstanceMeta)
    - [EstablishSessionRequest](#yaghan-control_plane-v1alpha1-EstablishSessionRequest)
    - [EstablishSessionResponse](#yaghan-control_plane-v1alpha1-EstablishSessionResponse)
    - [Event](#yaghan-control_plane-v1alpha1-Event)
    - [GetNodeRequest](#yaghan-control_plane-v1alpha1-GetNodeRequest)
    - [GetNodeResponse](#yaghan-control_plane-v1alpha1-GetNodeResponse)
    - [ListNodesRequest](#yaghan-control_plane-v1alpha1-ListNodesRequest)
    - [ListNodesResponse](#yaghan-control_plane-v1alpha1-ListNodesResponse)
    - [Node](#yaghan-control_plane-v1alpha1-Node)
    - [NodeMeta](#yaghan-control_plane-v1alpha1-NodeMeta)
    - [NodeMetrics](#yaghan-control_plane-v1alpha1-NodeMetrics)
    - [NodeResources](#yaghan-control_plane-v1alpha1-NodeResources)
    - [NodeStatus](#yaghan-control_plane-v1alpha1-NodeStatus)
    - [PatchNodeRequest](#yaghan-control_plane-v1alpha1-PatchNodeRequest)
    - [UpdateSandboxRequest](#yaghan-control_plane-v1alpha1-UpdateSandboxRequest)
  
    - [ListNodesRequest.Order](#yaghan-control_plane-v1alpha1-ListNodesRequest-Order)
    - [NodeStatus.Phase](#yaghan-control_plane-v1alpha1-NodeStatus-Phase)
  
    - [ClusterService](#yaghan-control_plane-v1alpha1-ClusterService)
  
- [yaghan/control_plane/v1alpha1/snapshot.proto](#yaghan_control_plane_v1alpha1_snapshot-proto)
    - [CreateSnapshotRequest](#yaghan-control_plane-v1alpha1-CreateSnapshotRequest)
    - [CreateSnapshotResponse](#yaghan-control_plane-v1alpha1-CreateSnapshotResponse)
    - [DeleteSnapshotRequest](#yaghan-control_plane-v1alpha1-DeleteSnapshotRequest)
    - [DeleteSnapshotResponse](#yaghan-control_plane-v1alpha1-DeleteSnapshotResponse)
    - [GetSnapshotRequest](#yaghan-control_plane-v1alpha1-GetSnapshotRequest)
    - [GetSnapshotResponse](#yaghan-control_plane-v1alpha1-GetSnapshotResponse)
    - [ListSnapshotsRequest](#yaghan-control_plane-v1alpha1-ListSnapshotsRequest)
    - [ListSnapshotsResponse](#yaghan-control_plane-v1alpha1-ListSnapshotsResponse)
    - [SandboxRef](#yaghan-control_plane-v1alpha1-SandboxRef)
    - [Snapshot](#yaghan-control_plane-v1alpha1-Snapshot)
    - [SnapshotMeta](#yaghan-control_plane-v1alpha1-SnapshotMeta)
  
    - [ListSnapshotsRequest.Order](#yaghan-control_plane-v1alpha1-ListSnapshotsRequest-Order)
  
    - [SnapshotService](#yaghan-control_plane-v1alpha1-SnapshotService)
  
- [yaghan/data_plane/v1alpha1/daemon.proto](#yaghan_data_plane_v1alpha1_daemon-proto)
    - [CancelRequest](#yaghan-data_plane-v1alpha1-CancelRequest)
    - [DownloadFileRequest](#yaghan-data_plane-v1alpha1-DownloadFileRequest)
    - [DownloadFileResponse](#yaghan-data_plane-v1alpha1-DownloadFileResponse)
    - [ExecProcess](#yaghan-data_plane-v1alpha1-ExecProcess)
    - [ExecProcess.EnvEntry](#yaghan-data_plane-v1alpha1-ExecProcess-EnvEntry)
    - [ExecRequest](#yaghan-data_plane-v1alpha1-ExecRequest)
    - [ExecResponse](#yaghan-data_plane-v1alpha1-ExecResponse)
    - [ProcessResult](#yaghan-data_plane-v1alpha1-ProcessResult)
    - [ResizePTY](#yaghan-data_plane-v1alpha1-ResizePTY)
    - [StdinChunk](#yaghan-data_plane-v1alpha1-StdinChunk)
    - [StreamChunk](#yaghan-data_plane-v1alpha1-StreamChunk)
    - [UploadFileRequest](#yaghan-data_plane-v1alpha1-UploadFileRequest)
    - [UploadFileResponse](#yaghan-data_plane-v1alpha1-UploadFileResponse)
  
    - [StreamChunk.StreamType](#yaghan-data_plane-v1alpha1-StreamChunk-StreamType)
  
    - [DaemonService](#yaghan-data_plane-v1alpha1-DaemonService)
  
- [yaghan/data_plane/v1alpha1/agent.proto](#yaghan_data_plane_v1alpha1_agent-proto)
    - [AgentRequest](#yaghan-data_plane-v1alpha1-AgentRequest)
    - [AgentResponse](#yaghan-data_plane-v1alpha1-AgentResponse)
  
- [Scalar Value Types](#scalar-value-types)



<a name="yaghan_auth_v1_auth-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## yaghan/auth/v1/auth.proto



<a name="yaghan-auth-v1-Actor"></a>

### Actor
Actor represents the identity that performed an action on a Resource,
such as creation, deletion or update. This may be an automation tool or a human user.

The identity is derived from the authentication context.

- For automation: `actor_id` corresponds to the `sub` claim of the
  automation&#39;s identity and `app_name` identifies the automation client.

- For human users: `actor_id` is the user&#39;s `sub`, `username` is the
  individual&#39;s login name, and `app_name` identifies the IDP app used
  during login.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| actor_id | [string](#string) |  | A stable unique identifier of the actor, typically the `sub` claim from the oauth token. This ID is consistent across sessions. |
| app_name | [string](#string) | optional | The name of the client application that initiated the action. This may refer to an automation tool or an IDP application. |
| username | [string](#string) | optional | The username of the human actor, if applicable. This field is only populated when the actor is a human. |






<a name="yaghan-auth-v1-InsufficientScopes"></a>

### InsufficientScopes
InsufficientScopes provides further details on unauthorized errors.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| required_scopes | [string](#string) | repeated | Scopes required for a given operation. |
| client_scopes | [string](#string) | repeated | Scopes present in the client token. |





 

 


<a name="yaghan_auth_v1_auth-proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| required_scopes | string | .google.protobuf.MethodOptions | 51234 | Scopes required by the gRPC method. Multiple scopes can be specified by separating them with spaces. For example: `option (required_scopes) = &#34;resource_type:write resource_type:read&#34;;` |

 

 



<a name="yaghan_control_plane_v1alpha1_sandbox-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## yaghan/control_plane/v1alpha1/sandbox.proto



<a name="yaghan-control_plane-v1alpha1-CreateSandboxRequest"></a>

### CreateSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |






<a name="yaghan-control_plane-v1alpha1-CreateSandboxResponse"></a>

### CreateSandboxResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |






<a name="yaghan-control_plane-v1alpha1-DeleteSandboxRequest"></a>

### DeleteSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |






<a name="yaghan-control_plane-v1alpha1-DeleteSandboxResponse"></a>

### DeleteSandboxResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |






<a name="yaghan-control_plane-v1alpha1-EgressPolicy"></a>

### EgressPolicy



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| allow | [EgressTargets](#yaghan-control_plane-v1alpha1-EgressTargets) |  |  |
| deny | [EgressTargets](#yaghan-control_plane-v1alpha1-EgressTargets) |  |  |






<a name="yaghan-control_plane-v1alpha1-EgressTargets"></a>

### EgressTargets



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ip_addresses | [string](#string) | repeated |  |
| cidr_blocks | [string](#string) | repeated |  |
| domain_names | [string](#string) | repeated |  |






<a name="yaghan-control_plane-v1alpha1-GetSandboxRequest"></a>

### GetSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-GetSandboxResponse"></a>

### GetSandboxResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |






<a name="yaghan-control_plane-v1alpha1-Intent"></a>

### Intent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| phase | [SandboxStatus.Phase](#yaghan-control_plane-v1alpha1-SandboxStatus-Phase) |  |  |
| start_snapshot | [StartSnapshotInput](#yaghan-control_plane-v1alpha1-StartSnapshotInput) |  |  |






<a name="yaghan-control_plane-v1alpha1-ListSandboxesRequest"></a>

### ListSandboxesRequest
Request message for listing sandboxes with optional filtering and pagination.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| namespace | [string](#string) |  | Filters sandboxes by namespace. When provided, must match the format: - starts with a lowercase letter - contains only lowercase alphanumeric characters or hyphens - ends with an alphanumeric character Example: &#34;default&#34;, &#34;team-a&#34; May be empty when node_id is supplied (e.g. the data-plane daemon&#39;s per-node resync scan). |
| node_id | [string](#string) |  | Filters sandboxes by the ID of the node where they are scheduled or running. |
| status_phase | [SandboxStatus.Phase](#yaghan-control_plane-v1alpha1-SandboxStatus-Phase) |  | Filters sandboxes by their current lifecycle phase. If unset, sandboxes in all phases are returned. |
| continuation_token | [string](#string) |  | Token used for pagination. Pass the value returned in a previous response to retrieve the next page of results. Leave empty to start listing from the beginning. |
| page_size | [int32](#int32) |  | Maximum number of sandboxes to return in this request. Defaults to 30 if not specified. The maximum allowed value is 1000. |
| sort_order | [ListSandboxesRequest.Order](#yaghan-control_plane-v1alpha1-ListSandboxesRequest-Order) |  | Sort order applied to the results based on last_modified_at. |






<a name="yaghan-control_plane-v1alpha1-ListSandboxesResponse"></a>

### ListSandboxesResponse
Response message containing a page of sandboxes.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandboxes | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) | repeated | The list of sandboxes matching the request filters. |
| continuation_token | [string](#string) |  | Token to retrieve the next page of results. Empty if there are no more results. |






<a name="yaghan-control_plane-v1alpha1-NodeRef"></a>

### NodeRef



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-PauseSandboxRequest"></a>

### PauseSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |






<a name="yaghan-control_plane-v1alpha1-PauseSandboxResponse"></a>

### PauseSandboxResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |






<a name="yaghan-control_plane-v1alpha1-Resources"></a>

### Resources



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| vcpu_count | [uint32](#uint32) |  |  |
| memory_mib | [uint64](#uint64) |  | Memory in MiB. Lower bound matches the smallest useful Firecracker VM; upper bound leaves room for 128 GiB sandboxes. |
| disk_mib | [uint64](#uint64) |  | Root disk size in MiB. Optional: 0 means &#34;use the daemon&#39;s configured default&#34;. When set, the daemon resizes the per-VM copy of the base rootfs image up to this size at provision time (ext4 grow on a sparse file — metadata-only, no eager allocation). Lower bound is the base image size; the upper bound is generous (1 TiB) on the spec side, with hosts further constrained by their advertised disk_capacity_bytes. |






<a name="yaghan-control_plane-v1alpha1-ResumeSandboxRequest"></a>

### ResumeSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |






<a name="yaghan-control_plane-v1alpha1-ResumeSandboxResponse"></a>

### ResumeSandboxResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |






<a name="yaghan-control_plane-v1alpha1-Sandbox"></a>

### Sandbox



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| metadata | [SandboxMeta](#yaghan-control_plane-v1alpha1-SandboxMeta) |  |  |
| egress_policy | [EgressPolicy](#yaghan-control_plane-v1alpha1-EgressPolicy) |  |  |
| resources | [Resources](#yaghan-control_plane-v1alpha1-Resources) |  | Sandbox.resources is required on the wire only for image-sourced sandboxes; snapshot-sourced sandboxes inherit their resources from the snapshot record and MUST leave this field unset on CreateSandbox. The api-server validates the conditional rule and stamps the inherited values before persistence — so every persisted row carries a populated Resources regardless of how it was created. |
| node | [NodeRef](#yaghan-control_plane-v1alpha1-NodeRef) |  |  |
| intent | [Intent](#yaghan-control_plane-v1alpha1-Intent) |  |  |
| last_snapshot | [SnapshotOutput](#yaghan-control_plane-v1alpha1-SnapshotOutput) |  |  |
| status | [SandboxStatus](#yaghan-control_plane-v1alpha1-SandboxStatus) |  |  |






<a name="yaghan-control_plane-v1alpha1-SandboxMeta"></a>

### SandboxMeta



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| namespace | [string](#string) |  |  |
| source | [SandboxSource](#yaghan-control_plane-v1alpha1-SandboxSource) |  |  |
| version | [int64](#int64) |  |  |
| created_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| last_modified_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| labels | [SandboxMeta.LabelsEntry](#yaghan-control_plane-v1alpha1-SandboxMeta-LabelsEntry) | repeated | Arbitrary key/value labels for client-side grouping and filtering (e.g. &#34;project=foo&#34;, &#34;ci-run=123&#34;). Keys are required to be 1-63 chars, lowercase alphanumeric with dots/dashes/ underscores, starting and ending with an alphanumeric. Values may be empty or follow the same rules (uppercase permitted). |






<a name="yaghan-control_plane-v1alpha1-SandboxMeta-LabelsEntry"></a>

### SandboxMeta.LabelsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-SandboxSource"></a>

### SandboxSource



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshot_id | [string](#string) |  |  |
| image_id | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-SandboxStatus"></a>

### SandboxStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| phase | [SandboxStatus.Phase](#yaghan-control_plane-v1alpha1-SandboxStatus-Phase) |  |  |
| message | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-SnapshotOutput"></a>

### SnapshotOutput



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshot_id | [string](#string) |  |  |
| created_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| error | [google.rpc.Status](#google-rpc-Status) |  |  |






<a name="yaghan-control_plane-v1alpha1-StartSnapshotInput"></a>

### StartSnapshotInput



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| description | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-StartSnapshotRequest"></a>

### StartSnapshotRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |
| description | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-StartSnapshotResponse"></a>

### StartSnapshotResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |





 


<a name="yaghan-control_plane-v1alpha1-ListSandboxesRequest-Order"></a>

### ListSandboxesRequest.Order
Controls how results are ordered by last modification time.

| Name | Number | Description |
| ---- | ------ | ----------- |
| ORDER_UNSPECIFIED | 0 |  |
| ORDER_NEWEST_FIRST | 1 | Most recently modified sandboxes first. |
| ORDER_OLDEST_FIRST | 2 | Least recently modified sandboxes first. |



<a name="yaghan-control_plane-v1alpha1-SandboxStatus-Phase"></a>

### SandboxStatus.Phase


| Name | Number | Description |
| ---- | ------ | ----------- |
| PHASE_UNSPECIFIED | 0 |  |
| PHASE_PENDING | 1 |  |
| PHASE_RUNNING | 2 |  |
| PHASE_PAUSING | 3 |  |
| PHASE_PAUSED | 4 |  |
| PHASE_RESUMING | 5 |  |
| PHASE_SNAPSHOTTING | 6 |  |
| PHASE_DELETING | 7 |  |
| PHASE_DELETED | 8 |  |
| PHASE_FAILED | 9 |  |


 

 


<a name="yaghan-control_plane-v1alpha1-SandboxService"></a>

### SandboxService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| CreateSandbox | [CreateSandboxRequest](#yaghan-control_plane-v1alpha1-CreateSandboxRequest) | [CreateSandboxResponse](#yaghan-control_plane-v1alpha1-CreateSandboxResponse) |  |
| GetSandbox | [GetSandboxRequest](#yaghan-control_plane-v1alpha1-GetSandboxRequest) | [GetSandboxResponse](#yaghan-control_plane-v1alpha1-GetSandboxResponse) |  |
| ListSandboxes | [ListSandboxesRequest](#yaghan-control_plane-v1alpha1-ListSandboxesRequest) | [ListSandboxesResponse](#yaghan-control_plane-v1alpha1-ListSandboxesResponse) |  |
| PauseSandbox | [PauseSandboxRequest](#yaghan-control_plane-v1alpha1-PauseSandboxRequest) | [PauseSandboxResponse](#yaghan-control_plane-v1alpha1-PauseSandboxResponse) |  |
| ResumeSandbox | [ResumeSandboxRequest](#yaghan-control_plane-v1alpha1-ResumeSandboxRequest) | [ResumeSandboxResponse](#yaghan-control_plane-v1alpha1-ResumeSandboxResponse) |  |
| DeleteSandbox | [DeleteSandboxRequest](#yaghan-control_plane-v1alpha1-DeleteSandboxRequest) | [DeleteSandboxResponse](#yaghan-control_plane-v1alpha1-DeleteSandboxResponse) |  |
| StartSnapshot | [StartSnapshotRequest](#yaghan-control_plane-v1alpha1-StartSnapshotRequest) | [StartSnapshotResponse](#yaghan-control_plane-v1alpha1-StartSnapshotResponse) |  |

 



<a name="yaghan_control_plane_v1alpha1_cluster-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## yaghan/control_plane/v1alpha1/cluster.proto



<a name="yaghan-control_plane-v1alpha1-ConnectionRequest"></a>

### ConnectionRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| session_id | [int64](#int64) |  | Identifier of the session. This value is optional. If omitted by the client, the server will auto-generate a unique identifier and return it in the response. If the client crashes and later reconnects to the API, it may send the same identifier to attempt resuming events from where it left off. Note, however, that there is no guarantee all missed events will be available after reconnecting, as some events may have been discarded if the retention thresholds were reached. |
| node | [Node](#yaghan-control_plane-v1alpha1-Node) |  |  |






<a name="yaghan-control_plane-v1alpha1-ConnectionResponse"></a>

### ConnectionResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| session_id | [int64](#int64) |  |  |






<a name="yaghan-control_plane-v1alpha1-EC2InstanceMeta"></a>

### EC2InstanceMeta



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| instance_id | [string](#string) |  |  |
| instance_type | [string](#string) |  |  |
| image_id | [string](#string) |  |  |
| account_id | [string](#string) |  |  |
| region | [string](#string) |  |  |
| availability_zone | [string](#string) |  |  |
| private_ip | [string](#string) |  |  |
| kernel_id | [string](#string) |  |  |
| architecture | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-EstablishSessionRequest"></a>

### EstablishSessionRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| connect | [ConnectionRequest](#yaghan-control_plane-v1alpha1-ConnectionRequest) |  |  |
| patch_node | [PatchNodeRequest](#yaghan-control_plane-v1alpha1-PatchNodeRequest) |  |  |
| update_sandbox | [UpdateSandboxRequest](#yaghan-control_plane-v1alpha1-UpdateSandboxRequest) |  |  |






<a name="yaghan-control_plane-v1alpha1-EstablishSessionResponse"></a>

### EstablishSessionResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| acknowledge | [ConnectionResponse](#yaghan-control_plane-v1alpha1-ConnectionResponse) |  |  |
| event | [Event](#yaghan-control_plane-v1alpha1-Event) |  |  |
| error | [google.rpc.Status](#google-rpc-Status) |  |  |






<a name="yaghan-control_plane-v1alpha1-Event"></a>

### Event
Event represents an activity in the system to which clients may subscribe.
It provides enough context for consumers to react, audit, or replicate the change.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | A unique identifier for this event. |
| emitted_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | The timestamp at which this event was emitted by the system. |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |






<a name="yaghan-control_plane-v1alpha1-GetNodeRequest"></a>

### GetNodeRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| node_id | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-GetNodeResponse"></a>

### GetNodeResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| node | [Node](#yaghan-control_plane-v1alpha1-Node) |  |  |






<a name="yaghan-control_plane-v1alpha1-ListNodesRequest"></a>

### ListNodesRequest
Request message for listing nodes with optional filtering and pagination.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status_phase | [NodeStatus.Phase](#yaghan-control_plane-v1alpha1-NodeStatus-Phase) |  | Filters nodes by their current lifecycle phase. If unset, nodes in all phases are returned. |
| continuation_token | [string](#string) |  | Token used for pagination. Pass the value returned in a previous response to retrieve the next page of results. Leave empty to start listing from the beginning. |
| page_size | [int32](#int32) |  | Maximum number of nodes to return in this request. Defaults to 30 if not specified. The maximum allowed value is 1000. |
| sort_order | [ListNodesRequest.Order](#yaghan-control_plane-v1alpha1-ListNodesRequest-Order) |  | Sort order applied to the results based on last_modified_at. |






<a name="yaghan-control_plane-v1alpha1-ListNodesResponse"></a>

### ListNodesResponse
Response message containing a page of nodes.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nodes | [Node](#yaghan-control_plane-v1alpha1-Node) | repeated | The list of nodes matching the request filters. |
| continuation_token | [string](#string) |  | Token to retrieve the next page of results. Empty if there are no more results. |






<a name="yaghan-control_plane-v1alpha1-Node"></a>

### Node



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| metadata | [NodeMeta](#yaghan-control_plane-v1alpha1-NodeMeta) |  |  |
| resources | [NodeResources](#yaghan-control_plane-v1alpha1-NodeResources) |  | Static/allocatable characteristics of the node. |
| metrics | [NodeMetrics](#yaghan-control_plane-v1alpha1-NodeMetrics) |  | Dynamic periodically sampled metrics. |
| status | [NodeStatus](#yaghan-control_plane-v1alpha1-NodeStatus) |  | Health and lifecycle state. |
| aws_ec2 | [EC2InstanceMeta](#yaghan-control_plane-v1alpha1-EC2InstanceMeta) |  |  |






<a name="yaghan-control_plane-v1alpha1-NodeMeta"></a>

### NodeMeta



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |
| created_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| last_modified_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |






<a name="yaghan-control_plane-v1alpha1-NodeMetrics"></a>

### NodeMetrics



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sampled_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Time at which the metrics were sampled. |
| active_sandbox_count | [uint32](#uint32) |  | Number of currently active sandboxes. |
| cpu_used_millicores | [uint32](#uint32) |  | CPU currently in use. |
| memory_used_bytes | [uint64](#uint64) |  | Memory currently in use. |
| disk_used_bytes | [uint64](#uint64) |  | Disk currently in use. |






<a name="yaghan-control_plane-v1alpha1-NodeResources"></a>

### NodeResources



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cpu_capacity_millicores | [uint32](#uint32) |  | Total allocatable vCPUs available for workloads. |
| memory_capacity_bytes | [uint64](#uint64) |  | Total allocatable memory available for workloads. |
| disk_capacity_bytes | [uint64](#uint64) |  | Total allocatable disk available for workloads. |






<a name="yaghan-control_plane-v1alpha1-NodeStatus"></a>

### NodeStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| phase | [NodeStatus.Phase](#yaghan-control_plane-v1alpha1-NodeStatus-Phase) |  |  |
| message | [string](#string) |  | Human-readable status message. |






<a name="yaghan-control_plane-v1alpha1-PatchNodeRequest"></a>

### PatchNodeRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| node_metrics | [NodeMetrics](#yaghan-control_plane-v1alpha1-NodeMetrics) |  |  |
| node_status | [NodeStatus](#yaghan-control_plane-v1alpha1-NodeStatus) |  |  |






<a name="yaghan-control_plane-v1alpha1-UpdateSandboxRequest"></a>

### UpdateSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#yaghan-control_plane-v1alpha1-Sandbox) |  |  |





 


<a name="yaghan-control_plane-v1alpha1-ListNodesRequest-Order"></a>

### ListNodesRequest.Order
Controls how results are ordered by last modification time.

| Name | Number | Description |
| ---- | ------ | ----------- |
| ORDER_UNSPECIFIED | 0 |  |
| ORDER_NEWEST_FIRST | 1 | Most recently modified nodes first. |
| ORDER_OLDEST_FIRST | 2 | Least recently modified nodes first. |



<a name="yaghan-control_plane-v1alpha1-NodeStatus-Phase"></a>

### NodeStatus.Phase


| Name | Number | Description |
| ---- | ------ | ----------- |
| PHASE_UNSPECIFIED | 0 |  |
| PHASE_HEALTHY | 1 | Node is healthy and able to accept workloads. |
| PHASE_UNHEALTHY | 2 | Node is reachable but degraded. |
| PHASE_LOST | 3 | Node has not reported recently and is considered lost. |
| PHASE_DELETED | 4 | Node is being removed or is no longer active. |
| PHASE_UNKNOWN | 5 | Status could not be determined due to transient failures. |


 

 


<a name="yaghan-control_plane-v1alpha1-ClusterService"></a>

### ClusterService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| EstablishSession | [EstablishSessionRequest](#yaghan-control_plane-v1alpha1-EstablishSessionRequest) stream | [EstablishSessionResponse](#yaghan-control_plane-v1alpha1-EstablishSessionResponse) stream |  |
| GetNode | [GetNodeRequest](#yaghan-control_plane-v1alpha1-GetNodeRequest) | [GetNodeResponse](#yaghan-control_plane-v1alpha1-GetNodeResponse) |  |
| ListNodes | [ListNodesRequest](#yaghan-control_plane-v1alpha1-ListNodesRequest) | [ListNodesResponse](#yaghan-control_plane-v1alpha1-ListNodesResponse) |  |

 



<a name="yaghan_control_plane_v1alpha1_snapshot-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## yaghan/control_plane/v1alpha1/snapshot.proto



<a name="yaghan-control_plane-v1alpha1-CreateSnapshotRequest"></a>

### CreateSnapshotRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshot | [Snapshot](#yaghan-control_plane-v1alpha1-Snapshot) |  |  |






<a name="yaghan-control_plane-v1alpha1-CreateSnapshotResponse"></a>

### CreateSnapshotResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshot | [Snapshot](#yaghan-control_plane-v1alpha1-Snapshot) |  |  |






<a name="yaghan-control_plane-v1alpha1-DeleteSnapshotRequest"></a>

### DeleteSnapshotRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshot_id | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-DeleteSnapshotResponse"></a>

### DeleteSnapshotResponse







<a name="yaghan-control_plane-v1alpha1-GetSnapshotRequest"></a>

### GetSnapshotRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshot_id | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-GetSnapshotResponse"></a>

### GetSnapshotResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshot | [Snapshot](#yaghan-control_plane-v1alpha1-Snapshot) |  |  |






<a name="yaghan-control_plane-v1alpha1-ListSnapshotsRequest"></a>

### ListSnapshotsRequest
Request message for listing snapshots with optional filtering and pagination.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| namespace | [string](#string) |  | Filters snapshots by namespace. When provided, must match the format: - starts with a lowercase letter - contains only lowercase alphanumeric characters or hyphens - ends with an alphanumeric character Example: &#34;default&#34;, &#34;team-a&#34; May be empty when sandbox_id is supplied. |
| sandbox_id | [string](#string) |  | Filters snapshots by the ID of the sandbox from which they were taken. |
| continuation_token | [string](#string) |  | Token used for pagination. Pass the value returned in a previous response to retrieve the next page of results. Leave empty to start listing from the beginning. |
| page_size | [int32](#int32) |  | Maximum number of snapshots to return in this request. Defaults to 30 if not specified. The maximum allowed value is 1000. |
| sort_order | [ListSnapshotsRequest.Order](#yaghan-control_plane-v1alpha1-ListSnapshotsRequest-Order) |  | Sort order applied to the results based on created_at. |






<a name="yaghan-control_plane-v1alpha1-ListSnapshotsResponse"></a>

### ListSnapshotsResponse
Response message containing a page of snapshots.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| snapshots | [Snapshot](#yaghan-control_plane-v1alpha1-Snapshot) | repeated | The list of snapshots matching the request filters. |
| continuation_token | [string](#string) |  | Token to retrieve the next page of results. Empty if there are no more results. |






<a name="yaghan-control_plane-v1alpha1-SandboxRef"></a>

### SandboxRef



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="yaghan-control_plane-v1alpha1-Snapshot"></a>

### Snapshot



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| metadata | [SnapshotMeta](#yaghan-control_plane-v1alpha1-SnapshotMeta) |  |  |
| sandbox | [SandboxRef](#yaghan-control_plane-v1alpha1-SandboxRef) |  |  |
| resources | [Resources](#yaghan-control_plane-v1alpha1-Resources) |  | Resources captures the vCPU / memory configuration the source sandbox was running with at snapshot time. Firecracker bakes these into the snapshot&#39;s state file and forbids changing them on restore (PATCH /machine-config is pre-boot only), so the api-server stamps this onto any sandbox derived from the snapshot. Required so a future scheduler can treat sandbox.Resources as the single source of truth without branching on how the sandbox was created. |






<a name="yaghan-control_plane-v1alpha1-SnapshotMeta"></a>

### SnapshotMeta



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| namespace | [string](#string) |  |  |
| description | [string](#string) |  |  |
| created_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |





 


<a name="yaghan-control_plane-v1alpha1-ListSnapshotsRequest-Order"></a>

### ListSnapshotsRequest.Order
Controls how results are ordered by creation time.

| Name | Number | Description |
| ---- | ------ | ----------- |
| ORDER_UNSPECIFIED | 0 |  |
| ORDER_NEWEST_FIRST | 1 | Most recently created snapshots first. |
| ORDER_OLDEST_FIRST | 2 | Least recently created snapshots first. |


 

 


<a name="yaghan-control_plane-v1alpha1-SnapshotService"></a>

### SnapshotService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| CreateSnapshot | [CreateSnapshotRequest](#yaghan-control_plane-v1alpha1-CreateSnapshotRequest) | [CreateSnapshotResponse](#yaghan-control_plane-v1alpha1-CreateSnapshotResponse) |  |
| GetSnapshot | [GetSnapshotRequest](#yaghan-control_plane-v1alpha1-GetSnapshotRequest) | [GetSnapshotResponse](#yaghan-control_plane-v1alpha1-GetSnapshotResponse) |  |
| ListSnapshots | [ListSnapshotsRequest](#yaghan-control_plane-v1alpha1-ListSnapshotsRequest) | [ListSnapshotsResponse](#yaghan-control_plane-v1alpha1-ListSnapshotsResponse) |  |
| DeleteSnapshot | [DeleteSnapshotRequest](#yaghan-control_plane-v1alpha1-DeleteSnapshotRequest) | [DeleteSnapshotResponse](#yaghan-control_plane-v1alpha1-DeleteSnapshotResponse) |  |

 



<a name="yaghan_data_plane_v1alpha1_daemon-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## yaghan/data_plane/v1alpha1/daemon.proto



<a name="yaghan-data_plane-v1alpha1-CancelRequest"></a>

### CancelRequest







<a name="yaghan-data_plane-v1alpha1-DownloadFileRequest"></a>

### DownloadFileRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| source | [string](#string) |  |  |






<a name="yaghan-data_plane-v1alpha1-DownloadFileResponse"></a>

### DownloadFileResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| file_content | [bytes](#bytes) |  |  |






<a name="yaghan-data_plane-v1alpha1-ExecProcess"></a>

### ExecProcess



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| command | [string](#string) |  |  |
| args | [string](#string) | repeated |  |
| env | [ExecProcess.EnvEntry](#yaghan-data_plane-v1alpha1-ExecProcess-EnvEntry) | repeated |  |
| cwd | [string](#string) |  |  |
| tty | [bool](#bool) |  |  |






<a name="yaghan-data_plane-v1alpha1-ExecProcess-EnvEntry"></a>

### ExecProcess.EnvEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="yaghan-data_plane-v1alpha1-ExecRequest"></a>

### ExecRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| exec_process | [ExecProcess](#yaghan-data_plane-v1alpha1-ExecProcess) |  |  |
| stdin | [StdinChunk](#yaghan-data_plane-v1alpha1-StdinChunk) |  |  |
| resize | [ResizePTY](#yaghan-data_plane-v1alpha1-ResizePTY) |  |  |






<a name="yaghan-data_plane-v1alpha1-ExecResponse"></a>

### ExecResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process_result | [ProcessResult](#yaghan-data_plane-v1alpha1-ProcessResult) |  |  |
| stream_chunk | [StreamChunk](#yaghan-data_plane-v1alpha1-StreamChunk) |  |  |






<a name="yaghan-data_plane-v1alpha1-ProcessResult"></a>

### ProcessResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| exit_code | [int32](#int32) |  |  |






<a name="yaghan-data_plane-v1alpha1-ResizePTY"></a>

### ResizePTY



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cols | [uint32](#uint32) |  |  |
| rows | [uint32](#uint32) |  |  |






<a name="yaghan-data_plane-v1alpha1-StdinChunk"></a>

### StdinChunk



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| data | [bytes](#bytes) |  |  |
| eof | [bool](#bool) |  |  |






<a name="yaghan-data_plane-v1alpha1-StreamChunk"></a>

### StreamChunk



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pid | [int32](#int32) |  |  |
| stream | [StreamChunk.StreamType](#yaghan-data_plane-v1alpha1-StreamChunk-StreamType) |  |  |
| data | [bytes](#bytes) |  |  |
| eof | [bool](#bool) |  |  |






<a name="yaghan-data_plane-v1alpha1-UploadFileRequest"></a>

### UploadFileRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| source | [bytes](#bytes) |  |  |
| dest | [string](#string) |  |  |






<a name="yaghan-data_plane-v1alpha1-UploadFileResponse"></a>

### UploadFileResponse






 


<a name="yaghan-data_plane-v1alpha1-StreamChunk-StreamType"></a>

### StreamChunk.StreamType


| Name | Number | Description |
| ---- | ------ | ----------- |
| STREAM_TYPE_UNSPECIFIED | 0 |  |
| STREAM_TYPE_STDOUT | 1 |  |
| STREAM_TYPE_STDERR | 2 |  |


 

 


<a name="yaghan-data_plane-v1alpha1-DaemonService"></a>

### DaemonService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Exec | [ExecRequest](#yaghan-data_plane-v1alpha1-ExecRequest) stream | [ExecResponse](#yaghan-data_plane-v1alpha1-ExecResponse) stream |  |
| UploadFile | [UploadFileRequest](#yaghan-data_plane-v1alpha1-UploadFileRequest) | [UploadFileResponse](#yaghan-data_plane-v1alpha1-UploadFileResponse) |  |
| DownloadFile | [DownloadFileRequest](#yaghan-data_plane-v1alpha1-DownloadFileRequest) | [DownloadFileResponse](#yaghan-data_plane-v1alpha1-DownloadFileResponse) |  |

 



<a name="yaghan_data_plane_v1alpha1_agent-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## yaghan/data_plane/v1alpha1/agent.proto



<a name="yaghan-data_plane-v1alpha1-AgentRequest"></a>

### AgentRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [uint64](#uint64) |  |  |
| exec_request | [ExecRequest](#yaghan-data_plane-v1alpha1-ExecRequest) |  |  |
| cancel | [CancelRequest](#yaghan-data_plane-v1alpha1-CancelRequest) |  |  |
| stdin | [StdinChunk](#yaghan-data_plane-v1alpha1-StdinChunk) |  |  |
| resize | [ResizePTY](#yaghan-data_plane-v1alpha1-ResizePTY) |  |  |
| upload_file | [UploadFileRequest](#yaghan-data_plane-v1alpha1-UploadFileRequest) |  |  |
| download_file | [DownloadFileRequest](#yaghan-data_plane-v1alpha1-DownloadFileRequest) |  |  |






<a name="yaghan-data_plane-v1alpha1-AgentResponse"></a>

### AgentResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [uint64](#uint64) |  |  |
| error | [google.rpc.Status](#google-rpc-Status) |  |  |
| exec_response | [ExecResponse](#yaghan-data_plane-v1alpha1-ExecResponse) |  |  |
| upload_file | [UploadFileResponse](#yaghan-data_plane-v1alpha1-UploadFileResponse) |  |  |
| download_file | [DownloadFileResponse](#yaghan-data_plane-v1alpha1-DownloadFileResponse) |  |  |





 

 

 

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

