# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [nuinfra/auth/v1/auth.proto](#nuinfra_auth_v1_auth-proto)
    - [Actor](#nuinfra-auth-v1-Actor)
    - [InsufficientScopes](#nuinfra-auth-v1-InsufficientScopes)
  
    - [File-level Extensions](#nuinfra_auth_v1_auth-proto-extensions)
  
- [nuinfra/control_plane/v1alpha1/sandbox.proto](#nuinfra_control_plane_v1alpha1_sandbox-proto)
    - [CreateSandboxRequest](#nuinfra-control_plane-v1alpha1-CreateSandboxRequest)
    - [CreateSandboxResponse](#nuinfra-control_plane-v1alpha1-CreateSandboxResponse)
    - [DeleteSandboxRequest](#nuinfra-control_plane-v1alpha1-DeleteSandboxRequest)
    - [DeleteSandboxResponse](#nuinfra-control_plane-v1alpha1-DeleteSandboxResponse)
    - [GetSandboxRequest](#nuinfra-control_plane-v1alpha1-GetSandboxRequest)
    - [GetSandboxResponse](#nuinfra-control_plane-v1alpha1-GetSandboxResponse)
    - [Intent](#nuinfra-control_plane-v1alpha1-Intent)
    - [ListSandboxesRequest](#nuinfra-control_plane-v1alpha1-ListSandboxesRequest)
    - [ListSandboxesResponse](#nuinfra-control_plane-v1alpha1-ListSandboxesResponse)
    - [NodeRef](#nuinfra-control_plane-v1alpha1-NodeRef)
    - [PauseSandboxRequest](#nuinfra-control_plane-v1alpha1-PauseSandboxRequest)
    - [PauseSandboxResponse](#nuinfra-control_plane-v1alpha1-PauseSandboxResponse)
    - [Resources](#nuinfra-control_plane-v1alpha1-Resources)
    - [ResumeSandboxRequest](#nuinfra-control_plane-v1alpha1-ResumeSandboxRequest)
    - [ResumeSandboxResponse](#nuinfra-control_plane-v1alpha1-ResumeSandboxResponse)
    - [Sandbox](#nuinfra-control_plane-v1alpha1-Sandbox)
    - [SandboxMeta](#nuinfra-control_plane-v1alpha1-SandboxMeta)
    - [SandboxMeta.LabelsEntry](#nuinfra-control_plane-v1alpha1-SandboxMeta-LabelsEntry)
    - [SandboxStatus](#nuinfra-control_plane-v1alpha1-SandboxStatus)
  
    - [ListSandboxesRequest.Order](#nuinfra-control_plane-v1alpha1-ListSandboxesRequest-Order)
    - [SandboxStatus.Phase](#nuinfra-control_plane-v1alpha1-SandboxStatus-Phase)
  
    - [SandboxService](#nuinfra-control_plane-v1alpha1-SandboxService)
  
- [nuinfra/control_plane/v1alpha1/cluster.proto](#nuinfra_control_plane_v1alpha1_cluster-proto)
    - [ConnectionRequest](#nuinfra-control_plane-v1alpha1-ConnectionRequest)
    - [ConnectionResponse](#nuinfra-control_plane-v1alpha1-ConnectionResponse)
    - [EstablishSessionRequest](#nuinfra-control_plane-v1alpha1-EstablishSessionRequest)
    - [EstablishSessionResponse](#nuinfra-control_plane-v1alpha1-EstablishSessionResponse)
    - [Event](#nuinfra-control_plane-v1alpha1-Event)
    - [Node](#nuinfra-control_plane-v1alpha1-Node)
    - [NodeMeta](#nuinfra-control_plane-v1alpha1-NodeMeta)
    - [NodeResources](#nuinfra-control_plane-v1alpha1-NodeResources)
    - [UpdateSandboxRequest](#nuinfra-control_plane-v1alpha1-UpdateSandboxRequest)
  
    - [ClusterService](#nuinfra-control_plane-v1alpha1-ClusterService)
  
- [nuinfra/data_plane/v1alpha1/daemon.proto](#nuinfra_data_plane_v1alpha1_daemon-proto)
    - [CancelRequest](#nuinfra-data_plane-v1alpha1-CancelRequest)
    - [DownloadFileRequest](#nuinfra-data_plane-v1alpha1-DownloadFileRequest)
    - [DownloadFileResponse](#nuinfra-data_plane-v1alpha1-DownloadFileResponse)
    - [ExecProcess](#nuinfra-data_plane-v1alpha1-ExecProcess)
    - [ExecProcess.EnvEntry](#nuinfra-data_plane-v1alpha1-ExecProcess-EnvEntry)
    - [ExecRequest](#nuinfra-data_plane-v1alpha1-ExecRequest)
    - [ExecResponse](#nuinfra-data_plane-v1alpha1-ExecResponse)
    - [ProcessResult](#nuinfra-data_plane-v1alpha1-ProcessResult)
    - [ResizePTY](#nuinfra-data_plane-v1alpha1-ResizePTY)
    - [StdinChunk](#nuinfra-data_plane-v1alpha1-StdinChunk)
    - [StreamChunk](#nuinfra-data_plane-v1alpha1-StreamChunk)
    - [UploadFileRequest](#nuinfra-data_plane-v1alpha1-UploadFileRequest)
    - [UploadFileResponse](#nuinfra-data_plane-v1alpha1-UploadFileResponse)
  
    - [StreamChunk.StreamType](#nuinfra-data_plane-v1alpha1-StreamChunk-StreamType)
  
    - [DaemonService](#nuinfra-data_plane-v1alpha1-DaemonService)
  
- [nuinfra/data_plane/v1alpha1/agent.proto](#nuinfra_data_plane_v1alpha1_agent-proto)
    - [AgentRequest](#nuinfra-data_plane-v1alpha1-AgentRequest)
    - [AgentResponse](#nuinfra-data_plane-v1alpha1-AgentResponse)
  
- [Scalar Value Types](#scalar-value-types)



<a name="nuinfra_auth_v1_auth-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## nuinfra/auth/v1/auth.proto



<a name="nuinfra-auth-v1-Actor"></a>

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






<a name="nuinfra-auth-v1-InsufficientScopes"></a>

### InsufficientScopes
InsufficientScopes provides further details on unauthorized errors.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| required_scopes | [string](#string) | repeated | Scopes required for a given operation. |
| client_scopes | [string](#string) | repeated | Scopes present in the client token. |





 

 


<a name="nuinfra_auth_v1_auth-proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| required_scopes | string | .google.protobuf.MethodOptions | 51234 | Scopes required by the gRPC method. Multiple scopes can be specified by separating them with spaces. For example: `option (required_scopes) = &#34;resource_type:write resource_type:read&#34;;` |

 

 



<a name="nuinfra_control_plane_v1alpha1_sandbox-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## nuinfra/control_plane/v1alpha1/sandbox.proto



<a name="nuinfra-control_plane-v1alpha1-CreateSandboxRequest"></a>

### CreateSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#nuinfra-control_plane-v1alpha1-Sandbox) |  |  |






<a name="nuinfra-control_plane-v1alpha1-CreateSandboxResponse"></a>

### CreateSandboxResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#nuinfra-control_plane-v1alpha1-Sandbox) |  |  |






<a name="nuinfra-control_plane-v1alpha1-DeleteSandboxRequest"></a>

### DeleteSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |






<a name="nuinfra-control_plane-v1alpha1-DeleteSandboxResponse"></a>

### DeleteSandboxResponse







<a name="nuinfra-control_plane-v1alpha1-GetSandboxRequest"></a>

### GetSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |






<a name="nuinfra-control_plane-v1alpha1-GetSandboxResponse"></a>

### GetSandboxResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#nuinfra-control_plane-v1alpha1-Sandbox) |  |  |






<a name="nuinfra-control_plane-v1alpha1-Intent"></a>

### Intent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| phase | [SandboxStatus.Phase](#nuinfra-control_plane-v1alpha1-SandboxStatus-Phase) |  |  |
| resources | [Resources](#nuinfra-control_plane-v1alpha1-Resources) |  |  |
| create_snapshot | [bool](#bool) |  |  |






<a name="nuinfra-control_plane-v1alpha1-ListSandboxesRequest"></a>

### ListSandboxesRequest
Request message for listing sandboxes with optional filtering and pagination.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| namespace | [string](#string) |  | Filters sandboxes by namespace. Must match the format: - starts with a lowercase letter - contains only lowercase alphanumeric characters or hyphens - ends with an alphanumeric character Example: &#34;default&#34;, &#34;team-a&#34; |
| node_id | [string](#string) |  | Filters sandboxes by the ID of the node where they are scheduled or running. |
| status_phase | [SandboxStatus.Phase](#nuinfra-control_plane-v1alpha1-SandboxStatus-Phase) |  | Filters sandboxes by their current lifecycle phase. If unset, sandboxes in all phases are returned. |
| continuation_token | [string](#string) |  | Token used for pagination. Pass the value returned in a previous response to retrieve the next page of results. Leave empty to start listing from the beginning. |
| page_size | [int32](#int32) |  | Maximum number of sandboxes to return in this request. Defaults to 30 if not specified. The maximum allowed value is 1000. |
| sort_order | [ListSandboxesRequest.Order](#nuinfra-control_plane-v1alpha1-ListSandboxesRequest-Order) |  | Sort order applied to the results based on last_modified_at. |






<a name="nuinfra-control_plane-v1alpha1-ListSandboxesResponse"></a>

### ListSandboxesResponse
Response message containing a page of sandboxes.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#nuinfra-control_plane-v1alpha1-Sandbox) | repeated | The list of sandboxes matching the request filters. |
| continuation_token | [string](#string) |  | Token to retrieve the next page of results. Empty if there are no more results. |






<a name="nuinfra-control_plane-v1alpha1-NodeRef"></a>

### NodeRef



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="nuinfra-control_plane-v1alpha1-PauseSandboxRequest"></a>

### PauseSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |






<a name="nuinfra-control_plane-v1alpha1-PauseSandboxResponse"></a>

### PauseSandboxResponse







<a name="nuinfra-control_plane-v1alpha1-Resources"></a>

### Resources



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| vcpu_count | [uint32](#uint32) |  |  |
| memory_mib | [uint64](#uint64) |  | Memory in MiB. Lower bound matches the smallest useful Firecracker VM; upper bound leaves room for 128 GiB sandboxes. |






<a name="nuinfra-control_plane-v1alpha1-ResumeSandboxRequest"></a>

### ResumeSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |






<a name="nuinfra-control_plane-v1alpha1-ResumeSandboxResponse"></a>

### ResumeSandboxResponse







<a name="nuinfra-control_plane-v1alpha1-Sandbox"></a>

### Sandbox



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| metadata | [SandboxMeta](#nuinfra-control_plane-v1alpha1-SandboxMeta) |  |  |
| resources | [Resources](#nuinfra-control_plane-v1alpha1-Resources) |  |  |
| node | [NodeRef](#nuinfra-control_plane-v1alpha1-NodeRef) |  |  |
| intent | [Intent](#nuinfra-control_plane-v1alpha1-Intent) |  |  |
| status | [SandboxStatus](#nuinfra-control_plane-v1alpha1-SandboxStatus) |  |  |






<a name="nuinfra-control_plane-v1alpha1-SandboxMeta"></a>

### SandboxMeta



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| namespace | [string](#string) |  |  |
| version | [int64](#int64) |  |  |
| created_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| last_modified_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| labels | [SandboxMeta.LabelsEntry](#nuinfra-control_plane-v1alpha1-SandboxMeta-LabelsEntry) | repeated | Arbitrary key/value labels for client-side grouping and filtering (e.g. &#34;project=foo&#34;, &#34;ci-run=123&#34;). Keys are required to be 1-63 chars, lowercase alphanumeric with dots/dashes/ underscores, starting and ending with an alphanumeric. Values may be empty or follow the same rules (uppercase permitted). |






<a name="nuinfra-control_plane-v1alpha1-SandboxMeta-LabelsEntry"></a>

### SandboxMeta.LabelsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="nuinfra-control_plane-v1alpha1-SandboxStatus"></a>

### SandboxStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| phase | [SandboxStatus.Phase](#nuinfra-control_plane-v1alpha1-SandboxStatus-Phase) |  |  |
| message | [string](#string) |  |  |





 


<a name="nuinfra-control_plane-v1alpha1-ListSandboxesRequest-Order"></a>

### ListSandboxesRequest.Order
Controls how results are ordered by last modification time.

| Name | Number | Description |
| ---- | ------ | ----------- |
| ORDER_UNSPECIFIED | 0 |  |
| ORDER_NEWEST_FIRST | 1 | Most recently modified sandboxes first. |
| ORDER_OLDEST_FIRST | 2 | Least recently modified sandboxes first. |



<a name="nuinfra-control_plane-v1alpha1-SandboxStatus-Phase"></a>

### SandboxStatus.Phase


| Name | Number | Description |
| ---- | ------ | ----------- |
| PHASE_UNSPECIFIED | 0 |  |
| PHASE_PENDING | 1 |  |
| PHASE_RUNNING | 2 |  |
| PHASE_PAUSING | 3 |  |
| PHASE_PAUSED | 4 |  |
| PHASE_RESUMING | 5 |  |
| PHASE_DELETING | 6 |  |
| PHASE_DELETED | 7 |  |
| PHASE_FAILED | 8 |  |


 

 


<a name="nuinfra-control_plane-v1alpha1-SandboxService"></a>

### SandboxService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| CreateSandbox | [CreateSandboxRequest](#nuinfra-control_plane-v1alpha1-CreateSandboxRequest) | [CreateSandboxResponse](#nuinfra-control_plane-v1alpha1-CreateSandboxResponse) |  |
| GetSandbox | [GetSandboxRequest](#nuinfra-control_plane-v1alpha1-GetSandboxRequest) | [GetSandboxResponse](#nuinfra-control_plane-v1alpha1-GetSandboxResponse) |  |
| ListSandboxes | [ListSandboxesRequest](#nuinfra-control_plane-v1alpha1-ListSandboxesRequest) | [ListSandboxesResponse](#nuinfra-control_plane-v1alpha1-ListSandboxesResponse) |  |
| PauseSandbox | [PauseSandboxRequest](#nuinfra-control_plane-v1alpha1-PauseSandboxRequest) | [PauseSandboxResponse](#nuinfra-control_plane-v1alpha1-PauseSandboxResponse) |  |
| ResumeSandbox | [ResumeSandboxRequest](#nuinfra-control_plane-v1alpha1-ResumeSandboxRequest) | [ResumeSandboxResponse](#nuinfra-control_plane-v1alpha1-ResumeSandboxResponse) |  |
| DeleteSandbox | [DeleteSandboxRequest](#nuinfra-control_plane-v1alpha1-DeleteSandboxRequest) | [DeleteSandboxResponse](#nuinfra-control_plane-v1alpha1-DeleteSandboxResponse) |  |

 



<a name="nuinfra_control_plane_v1alpha1_cluster-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## nuinfra/control_plane/v1alpha1/cluster.proto



<a name="nuinfra-control_plane-v1alpha1-ConnectionRequest"></a>

### ConnectionRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| session_id | [int64](#int64) |  | Identifier of the session. This value is optional. If omitted by the client, the server will auto-generate a unique identifier and return it in the response. If the client crashes and later reconnects to the API, it may send the same identifier to attempt resuming events from where it left off. Note, however, that there is no guarantee all missed events will be available after reconnecting, as some events may have been discarded if the retention thresholds were reached. |
| node | [Node](#nuinfra-control_plane-v1alpha1-Node) |  |  |






<a name="nuinfra-control_plane-v1alpha1-ConnectionResponse"></a>

### ConnectionResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| session_id | [int64](#int64) |  |  |






<a name="nuinfra-control_plane-v1alpha1-EstablishSessionRequest"></a>

### EstablishSessionRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| connect | [ConnectionRequest](#nuinfra-control_plane-v1alpha1-ConnectionRequest) |  |  |
| update_sandbox | [UpdateSandboxRequest](#nuinfra-control_plane-v1alpha1-UpdateSandboxRequest) |  |  |






<a name="nuinfra-control_plane-v1alpha1-EstablishSessionResponse"></a>

### EstablishSessionResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| acknowledge | [ConnectionResponse](#nuinfra-control_plane-v1alpha1-ConnectionResponse) |  |  |
| event | [Event](#nuinfra-control_plane-v1alpha1-Event) |  |  |
| error | [google.rpc.Status](#google-rpc-Status) |  |  |






<a name="nuinfra-control_plane-v1alpha1-Event"></a>

### Event
Event represents an activity in the system to which clients may subscribe.
It provides enough context for consumers to react, audit, or replicate the change.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | A unique identifier for this event. |
| emitted_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | The timestamp at which this event was emitted by the system. |
| sandbox | [Sandbox](#nuinfra-control_plane-v1alpha1-Sandbox) |  |  |






<a name="nuinfra-control_plane-v1alpha1-Node"></a>

### Node



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| metadata | [NodeMeta](#nuinfra-control_plane-v1alpha1-NodeMeta) |  |  |
| resources | [NodeResources](#nuinfra-control_plane-v1alpha1-NodeResources) |  |  |






<a name="nuinfra-control_plane-v1alpha1-NodeMeta"></a>

### NodeMeta



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| version | [int64](#int64) |  |  |
| created_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| last_modified_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |






<a name="nuinfra-control_plane-v1alpha1-NodeResources"></a>

### NodeResources



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| vcpu_count | [uint32](#uint32) |  |  |
| memory_mib | [uint64](#uint64) |  |  |






<a name="nuinfra-control_plane-v1alpha1-UpdateSandboxRequest"></a>

### UpdateSandboxRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox | [Sandbox](#nuinfra-control_plane-v1alpha1-Sandbox) |  |  |





 

 

 


<a name="nuinfra-control_plane-v1alpha1-ClusterService"></a>

### ClusterService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| EstablishSession | [EstablishSessionRequest](#nuinfra-control_plane-v1alpha1-EstablishSessionRequest) stream | [EstablishSessionResponse](#nuinfra-control_plane-v1alpha1-EstablishSessionResponse) stream |  |

 



<a name="nuinfra_data_plane_v1alpha1_daemon-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## nuinfra/data_plane/v1alpha1/daemon.proto



<a name="nuinfra-data_plane-v1alpha1-CancelRequest"></a>

### CancelRequest







<a name="nuinfra-data_plane-v1alpha1-DownloadFileRequest"></a>

### DownloadFileRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| source | [string](#string) |  |  |






<a name="nuinfra-data_plane-v1alpha1-DownloadFileResponse"></a>

### DownloadFileResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| file_content | [bytes](#bytes) |  |  |






<a name="nuinfra-data_plane-v1alpha1-ExecProcess"></a>

### ExecProcess



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| command | [string](#string) |  |  |
| args | [string](#string) | repeated |  |
| env | [ExecProcess.EnvEntry](#nuinfra-data_plane-v1alpha1-ExecProcess-EnvEntry) | repeated |  |
| cwd | [string](#string) |  |  |
| tty | [bool](#bool) |  |  |






<a name="nuinfra-data_plane-v1alpha1-ExecProcess-EnvEntry"></a>

### ExecProcess.EnvEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="nuinfra-data_plane-v1alpha1-ExecRequest"></a>

### ExecRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| exec_process | [ExecProcess](#nuinfra-data_plane-v1alpha1-ExecProcess) |  |  |
| stdin | [StdinChunk](#nuinfra-data_plane-v1alpha1-StdinChunk) |  |  |
| resize | [ResizePTY](#nuinfra-data_plane-v1alpha1-ResizePTY) |  |  |






<a name="nuinfra-data_plane-v1alpha1-ExecResponse"></a>

### ExecResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process_result | [ProcessResult](#nuinfra-data_plane-v1alpha1-ProcessResult) |  |  |
| stream_chunk | [StreamChunk](#nuinfra-data_plane-v1alpha1-StreamChunk) |  |  |






<a name="nuinfra-data_plane-v1alpha1-ProcessResult"></a>

### ProcessResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| exit_code | [int32](#int32) |  |  |






<a name="nuinfra-data_plane-v1alpha1-ResizePTY"></a>

### ResizePTY



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cols | [uint32](#uint32) |  |  |
| rows | [uint32](#uint32) |  |  |






<a name="nuinfra-data_plane-v1alpha1-StdinChunk"></a>

### StdinChunk



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| data | [bytes](#bytes) |  |  |
| eof | [bool](#bool) |  |  |






<a name="nuinfra-data_plane-v1alpha1-StreamChunk"></a>

### StreamChunk



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pid | [int32](#int32) |  |  |
| stream | [StreamChunk.StreamType](#nuinfra-data_plane-v1alpha1-StreamChunk-StreamType) |  |  |
| data | [bytes](#bytes) |  |  |
| eof | [bool](#bool) |  |  |






<a name="nuinfra-data_plane-v1alpha1-UploadFileRequest"></a>

### UploadFileRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sandbox_id | [string](#string) |  |  |
| source | [bytes](#bytes) |  |  |
| dest | [string](#string) |  |  |






<a name="nuinfra-data_plane-v1alpha1-UploadFileResponse"></a>

### UploadFileResponse






 


<a name="nuinfra-data_plane-v1alpha1-StreamChunk-StreamType"></a>

### StreamChunk.StreamType


| Name | Number | Description |
| ---- | ------ | ----------- |
| STREAM_TYPE_UNSPECIFIED | 0 |  |
| STREAM_TYPE_STDOUT | 1 |  |
| STREAM_TYPE_STDERR | 2 |  |


 

 


<a name="nuinfra-data_plane-v1alpha1-DaemonService"></a>

### DaemonService


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Exec | [ExecRequest](#nuinfra-data_plane-v1alpha1-ExecRequest) stream | [ExecResponse](#nuinfra-data_plane-v1alpha1-ExecResponse) stream |  |
| UploadFile | [UploadFileRequest](#nuinfra-data_plane-v1alpha1-UploadFileRequest) | [UploadFileResponse](#nuinfra-data_plane-v1alpha1-UploadFileResponse) |  |
| DownloadFile | [DownloadFileRequest](#nuinfra-data_plane-v1alpha1-DownloadFileRequest) | [DownloadFileResponse](#nuinfra-data_plane-v1alpha1-DownloadFileResponse) |  |

 



<a name="nuinfra_data_plane_v1alpha1_agent-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## nuinfra/data_plane/v1alpha1/agent.proto



<a name="nuinfra-data_plane-v1alpha1-AgentRequest"></a>

### AgentRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [uint64](#uint64) |  |  |
| exec_request | [ExecRequest](#nuinfra-data_plane-v1alpha1-ExecRequest) |  |  |
| cancel | [CancelRequest](#nuinfra-data_plane-v1alpha1-CancelRequest) |  |  |
| stdin | [StdinChunk](#nuinfra-data_plane-v1alpha1-StdinChunk) |  |  |
| resize | [ResizePTY](#nuinfra-data_plane-v1alpha1-ResizePTY) |  |  |
| upload_file | [UploadFileRequest](#nuinfra-data_plane-v1alpha1-UploadFileRequest) |  |  |
| download_file | [DownloadFileRequest](#nuinfra-data_plane-v1alpha1-DownloadFileRequest) |  |  |






<a name="nuinfra-data_plane-v1alpha1-AgentResponse"></a>

### AgentResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [uint64](#uint64) |  |  |
| error | [google.rpc.Status](#google-rpc-Status) |  |  |
| exec_response | [ExecResponse](#nuinfra-data_plane-v1alpha1-ExecResponse) |  |  |
| upload_file | [UploadFileResponse](#nuinfra-data_plane-v1alpha1-UploadFileResponse) |  |  |
| download_file | [DownloadFileResponse](#nuinfra-data_plane-v1alpha1-DownloadFileResponse) |  |  |





 

 

 

 



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

