---
title: yaghan/control_plane/v1alpha1/cluster.proto version not set
language_tabs:
  - shell: cURL
  - python: Python
  - javascript: JavaScript
  - go: Go
language_clients:
  - shell: ""
  - python: ""
  - javascript: ""
  - go: ""
toc_footers: []
includes: []
search: true
highlight_theme: darkula
headingLevel: 2

---

<!-- Generator: Widdershins v4.0.1 -->

<h1 id="yaghan-control_plane-v1alpha1-cluster-proto">yaghan/control_plane/v1alpha1/cluster.proto version not set</h1>

> Scroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

<h1 id="yaghan-control_plane-v1alpha1-cluster-proto-clusterservice">ClusterService</h1>

## ClusterService_ListNodes

<a id="opIdClusterService_ListNodes"></a>

`GET /v1alpha1/nodes`

<h3 id="clusterservice_listnodes-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|statusPhase|query|string|false|Filters nodes by their current lifecycle phase.|
|continuationToken|query|string|false|Token used for pagination.|
|pageSize|query|integer(int32)|false|Maximum number of nodes to return in this request.|
|sortOrder|query|string|false|Sort order applied to the results based on last_modified_at.|

#### Detailed descriptions

**statusPhase**: Filters nodes by their current lifecycle phase.
If unset, nodes in all phases are returned.

 - PHASE_HEALTHY: Node is healthy and able to accept workloads.
 - PHASE_UNHEALTHY: Node is reachable but degraded.
 - PHASE_LOST: Node has not reported recently and is considered lost.
 - PHASE_DELETED: Node is being removed or is no longer active.
 - PHASE_UNKNOWN: Status could not be determined due to transient failures.

**continuationToken**: Token used for pagination.
Pass the value returned in a previous response to retrieve the next page of results.
Leave empty to start listing from the beginning.

**pageSize**: Maximum number of nodes to return in this request.
Defaults to 30 if not specified.
The maximum allowed value is 1000.

**sortOrder**: Sort order applied to the results based on last_modified_at.

 - ORDER_NEWEST_FIRST: Most recently modified nodes first.
 - ORDER_OLDEST_FIRST: Least recently modified nodes first.

#### Enumerated Values

|Parameter|Value|
|---|---|
|statusPhase|PHASE_UNSPECIFIED|
|statusPhase|PHASE_HEALTHY|
|statusPhase|PHASE_UNHEALTHY|
|statusPhase|PHASE_LOST|
|statusPhase|PHASE_DELETED|
|statusPhase|PHASE_UNKNOWN|
|sortOrder|ORDER_UNSPECIFIED|
|sortOrder|ORDER_NEWEST_FIRST|
|sortOrder|ORDER_OLDEST_FIRST|

> Example responses

> 200 Response

```json
{
  "nodes": [
    {
      "metadata": {
        "id": "string",
        "version": "string",
        "createdAt": "2019-08-24T14:15:22Z",
        "lastModifiedAt": "2019-08-24T14:15:22Z"
      },
      "resources": {
        "cpuCapacityMillicores": 0,
        "memoryCapacityBytes": "string",
        "diskCapacityBytes": "string"
      },
      "metrics": {
        "sampledAt": "2019-08-24T14:15:22Z",
        "activeSandboxCount": 0,
        "cpuUsedMillicores": 0,
        "memoryUsedBytes": "string",
        "diskUsedBytes": "string"
      },
      "status": {
        "phase": "PHASE_UNSPECIFIED",
        "message": "string"
      },
      "awsEc2": {
        "instanceId": "string",
        "instanceType": "string",
        "imageId": "string",
        "accountId": "string",
        "region": "string",
        "availabilityZone": "string",
        "privateIp": "string",
        "kernelId": "string",
        "architecture": "string"
      }
    }
  ],
  "continuationToken": "string"
}
```

<h3 id="clusterservice_listnodes-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1ListNodesResponse](#schemav1alpha1listnodesresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

## ClusterService_GetNode

<a id="opIdClusterService_GetNode"></a>

`GET /v1alpha1/nodes/{nodeId}`

<h3 id="clusterservice_getnode-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|nodeId|path|string|true|none|

> Example responses

> 200 Response

```json
{
  "node": {
    "metadata": {
      "id": "string",
      "version": "string",
      "createdAt": "2019-08-24T14:15:22Z",
      "lastModifiedAt": "2019-08-24T14:15:22Z"
    },
    "resources": {
      "cpuCapacityMillicores": 0,
      "memoryCapacityBytes": "string",
      "diskCapacityBytes": "string"
    },
    "metrics": {
      "sampledAt": "2019-08-24T14:15:22Z",
      "activeSandboxCount": 0,
      "cpuUsedMillicores": 0,
      "memoryUsedBytes": "string",
      "diskUsedBytes": "string"
    },
    "status": {
      "phase": "PHASE_UNSPECIFIED",
      "message": "string"
    },
    "awsEc2": {
      "instanceId": "string",
      "instanceType": "string",
      "imageId": "string",
      "accountId": "string",
      "region": "string",
      "availabilityZone": "string",
      "privateIp": "string",
      "kernelId": "string",
      "architecture": "string"
    }
  }
}
```

<h3 id="clusterservice_getnode-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1GetNodeResponse](#schemav1alpha1getnoderesponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

# Schemas

<h2 id="tocS_protobufAny">protobufAny</h2>
<!-- backwards compatibility -->
<a id="schemaprotobufany"></a>
<a id="schema_protobufAny"></a>
<a id="tocSprotobufany"></a>
<a id="tocsprotobufany"></a>

```json
{
  "@type": "string",
  "property1": null,
  "property2": null
}

```

`Any` contains an arbitrary serialized protocol buffer message along with a
URL that describes the type of the serialized message.

Protobuf library provides support to pack/unpack Any values in the form
of utility functions or additional generated methods of the Any type.

Example 1: Pack and unpack a message in C++.

    Foo foo = ...;
    Any any;
    any.PackFrom(foo);
    ...
    if (any.UnpackTo(&foo)) {
      ...
    }

Example 2: Pack and unpack a message in Java.

    Foo foo = ...;
    Any any = Any.pack(foo);
    ...
    if (any.is(Foo.class)) {
      foo = any.unpack(Foo.class);
    }
    // or ...
    if (any.isSameTypeAs(Foo.getDefaultInstance())) {
      foo = any.unpack(Foo.getDefaultInstance());
    }

 Example 3: Pack and unpack a message in Python.

    foo = Foo(...)
    any = Any()
    any.Pack(foo)
    ...
    if any.Is(Foo.DESCRIPTOR):
      any.Unpack(foo)
      ...

 Example 4: Pack and unpack a message in Go

     foo := &pb.Foo{...}
     any, err := anypb.New(foo)
     if err != nil {
       ...
     }
     ...
     foo := &pb.Foo{}
     if err := any.UnmarshalTo(foo); err != nil {
       ...
     }

The pack methods provided by protobuf library will by default use
'type.googleapis.com/full.type.name' as the type URL and the unpack
methods only use the fully qualified type name after the last '/'
in the type URL, for example "foo.bar.com/x/y.z" will yield type
name "y.z".

JSON
====
The JSON representation of an `Any` value uses the regular
representation of the deserialized, embedded message, with an
additional field `@type` which contains the type URL. Example:

    package google.profile;
    message Person {
      string first_name = 1;
      string last_name = 2;
    }

    {
      "@type": "type.googleapis.com/google.profile.Person",
      "firstName": <string>,
      "lastName": <string>
    }

If the embedded message type is well-known and has a custom JSON
representation, that representation will be embedded adding a field
`value` which holds the custom JSON in addition to the `@type`
field. Example (for message [google.protobuf.Duration][]):

    {
      "@type": "type.googleapis.com/google.protobuf.Duration",
      "value": "1.212s"
    }

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|**additionalProperties**|any|false|none|none|
|@type|string|false|none|A URL/resource name that uniquely identifies the type of the serialized<br>protocol buffer message. This string must contain at least<br>one "/" character. The last segment of the URL's path must represent<br>the fully qualified name of the type (as in<br>`path/google.protobuf.Duration`). The name should be in a canonical form<br>(e.g., leading "." is not accepted).<br><br>In practice, teams usually precompile into the binary all types that they<br>expect it to use in the context of Any. However, for URLs which use the<br>scheme `http`, `https`, or no scheme, one can optionally set up a type<br>server that maps type URLs to message definitions as follows:<br><br>* If no scheme is provided, `https` is assumed.<br>* An HTTP GET on the URL must yield a [google.protobuf.Type][]<br>  value in binary format, or produce an error.<br>* Applications are allowed to cache lookup results based on the<br>  URL, or have them precompiled into a binary to avoid any<br>  lookup. Therefore, binary compatibility needs to be preserved<br>  on changes to types. (Use versioned type names to manage<br>  breaking changes.)<br><br>Note: this functionality is not currently available in the official<br>protobuf release, and it is not used for type URLs beginning with<br>type.googleapis.com. As of May 2023, there are no widely used type server<br>implementations and no plans to implement one.<br><br>Schemes other than `http`, `https` (or the empty scheme) might be<br>used with implementation specific semantics.|

<h2 id="tocS_rpcStatus">rpcStatus</h2>
<!-- backwards compatibility -->
<a id="schemarpcstatus"></a>
<a id="schema_rpcStatus"></a>
<a id="tocSrpcstatus"></a>
<a id="tocsrpcstatus"></a>

```json
{
  "code": 0,
  "message": "string",
  "details": [
    {
      "@type": "string",
      "property1": null,
      "property2": null
    }
  ]
}

```

The `Status` type defines a logical error model that is suitable for
different programming environments, including REST APIs and RPC APIs. It is
used by [gRPC](https://github.com/grpc). Each `Status` message contains
three pieces of data: error code, error message, and error details.

You can find out more about this error model and how to work with it in the
[API Design Guide](https://cloud.google.com/apis/design/errors).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|code|integer(int32)|false|none|The status code, which should be an enum value of<br>[google.rpc.Code][google.rpc.Code].|
|message|string|false|none|A developer-facing error message, which should be in English. Any<br>user-facing error message should be localized and sent in the<br>[google.rpc.Status.details][google.rpc.Status.details] field, or localized<br>by the client.|
|details|[[protobufAny](#schemaprotobufany)]|false|none|A list of messages that carry the error details.  There is a common set of<br>message types for APIs to use.|

<h2 id="tocS_v1alpha1ConnectionRequest">v1alpha1ConnectionRequest</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1connectionrequest"></a>
<a id="schema_v1alpha1ConnectionRequest"></a>
<a id="tocSv1alpha1connectionrequest"></a>
<a id="tocsv1alpha1connectionrequest"></a>

```json
{
  "sessionId": "string",
  "node": {
    "metadata": {
      "id": "string",
      "version": "string",
      "createdAt": "2019-08-24T14:15:22Z",
      "lastModifiedAt": "2019-08-24T14:15:22Z"
    },
    "resources": {
      "cpuCapacityMillicores": 0,
      "memoryCapacityBytes": "string",
      "diskCapacityBytes": "string"
    },
    "metrics": {
      "sampledAt": "2019-08-24T14:15:22Z",
      "activeSandboxCount": 0,
      "cpuUsedMillicores": 0,
      "memoryUsedBytes": "string",
      "diskUsedBytes": "string"
    },
    "status": {
      "phase": "PHASE_UNSPECIFIED",
      "message": "string"
    },
    "awsEc2": {
      "instanceId": "string",
      "instanceType": "string",
      "imageId": "string",
      "accountId": "string",
      "region": "string",
      "availabilityZone": "string",
      "privateIp": "string",
      "kernelId": "string",
      "architecture": "string"
    }
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|sessionId|string(int64)|false|none|Identifier of the session. This value is optional. If omitted by the client,<br>the server will auto-generate a unique identifier and return it in the response.<br>If the client crashes and later reconnects to the API, it may send the same identifier<br>to attempt resuming events from where it left off. Note, however, that there is no<br>guarantee all missed events will be available after reconnecting, as some events<br>may have been discarded if the retention thresholds were reached.|
|node|[v1alpha1Node](#schemav1alpha1node)|false|none|none|

<h2 id="tocS_v1alpha1ConnectionResponse">v1alpha1ConnectionResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1connectionresponse"></a>
<a id="schema_v1alpha1ConnectionResponse"></a>
<a id="tocSv1alpha1connectionresponse"></a>
<a id="tocsv1alpha1connectionresponse"></a>

```json
{
  "sessionId": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|sessionId|string(int64)|false|none|none|

<h2 id="tocS_v1alpha1EC2InstanceMeta">v1alpha1EC2InstanceMeta</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1ec2instancemeta"></a>
<a id="schema_v1alpha1EC2InstanceMeta"></a>
<a id="tocSv1alpha1ec2instancemeta"></a>
<a id="tocsv1alpha1ec2instancemeta"></a>

```json
{
  "instanceId": "string",
  "instanceType": "string",
  "imageId": "string",
  "accountId": "string",
  "region": "string",
  "availabilityZone": "string",
  "privateIp": "string",
  "kernelId": "string",
  "architecture": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|instanceId|string|false|none|none|
|instanceType|string|false|none|none|
|imageId|string|false|none|none|
|accountId|string|false|none|none|
|region|string|false|none|none|
|availabilityZone|string|false|none|none|
|privateIp|string|false|none|none|
|kernelId|string|false|none|none|
|architecture|string|false|none|none|

<h2 id="tocS_v1alpha1EgressPolicy">v1alpha1EgressPolicy</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1egresspolicy"></a>
<a id="schema_v1alpha1EgressPolicy"></a>
<a id="tocSv1alpha1egresspolicy"></a>
<a id="tocsv1alpha1egresspolicy"></a>

```json
{
  "allow": {
    "ipAddresses": [
      "string"
    ],
    "cidrBlocks": [
      "string"
    ],
    "domainNames": [
      "string"
    ]
  },
  "deny": {
    "ipAddresses": [
      "string"
    ],
    "cidrBlocks": [
      "string"
    ],
    "domainNames": [
      "string"
    ]
  }
}

```

EgressPolicy constrains the external destinations a sandbox can
reach from inside its guest. It is enforced by the data-plane daemon
at sandbox-boot time as netfilter rules in the VM's network
namespace; the api-server only validates syntax.

Semantics:
  - Unset (the default): no restriction. The sandbox can reach any
    external target reachable from the host.
  - `allow` set: only destinations matching one of the listed targets
    are reachable. Everything else is rejected with ICMP admin-
    prohibited (TCP fails fast with EHOSTUNREACH-class errors).
  - `deny` set: every destination is reachable except those matching
    one of the listed targets, which are rejected as above.

There is intentionally no implicit DNS pass-through: under an
`allow` policy a sandbox cannot reach its resolver unless the
resolver's IP is listed.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|allow|[v1alpha1EgressTargets](#schemav1alpha1egresstargets)|false|none|EgressTargets is the union of destinations referenced by an<br>EgressPolicy allow- or deny-rule.<br><br>Rule precedence is `ip_addresses` → `cidr_blocks` → `domain_names`:<br>a destination that matches an `ip_addresses` entry takes effect<br>before any overlapping `cidr_blocks` entry, which in turn takes<br>effect before any overlapping `domain_names` entry. Within a single<br>policy arm (`allow` or `deny`) the verdict is the same across all<br>three, so precedence is currently observable only in logs; it<br>becomes load-bearing once future iterations let policies mix<br>verdicts.<br><br>Note: `domain_names` is accepted and validated for syntax today but<br>is **not yet enforced** by the data plane — DNS resolution + cache<br>lifecycle is a separate iteration. The daemon logs a warning when a<br>sandbox boots with non-empty `domain_names` so the gap is not<br>silent.|
|deny|[v1alpha1EgressTargets](#schemav1alpha1egresstargets)|false|none|EgressTargets is the union of destinations referenced by an<br>EgressPolicy allow- or deny-rule.<br><br>Rule precedence is `ip_addresses` → `cidr_blocks` → `domain_names`:<br>a destination that matches an `ip_addresses` entry takes effect<br>before any overlapping `cidr_blocks` entry, which in turn takes<br>effect before any overlapping `domain_names` entry. Within a single<br>policy arm (`allow` or `deny`) the verdict is the same across all<br>three, so precedence is currently observable only in logs; it<br>becomes load-bearing once future iterations let policies mix<br>verdicts.<br><br>Note: `domain_names` is accepted and validated for syntax today but<br>is **not yet enforced** by the data plane — DNS resolution + cache<br>lifecycle is a separate iteration. The daemon logs a warning when a<br>sandbox boots with non-empty `domain_names` so the gap is not<br>silent.|

<h2 id="tocS_v1alpha1EgressTargets">v1alpha1EgressTargets</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1egresstargets"></a>
<a id="schema_v1alpha1EgressTargets"></a>
<a id="tocSv1alpha1egresstargets"></a>
<a id="tocsv1alpha1egresstargets"></a>

```json
{
  "ipAddresses": [
    "string"
  ],
  "cidrBlocks": [
    "string"
  ],
  "domainNames": [
    "string"
  ]
}

```

EgressTargets is the union of destinations referenced by an
EgressPolicy allow- or deny-rule.

Rule precedence is `ip_addresses` → `cidr_blocks` → `domain_names`:
a destination that matches an `ip_addresses` entry takes effect
before any overlapping `cidr_blocks` entry, which in turn takes
effect before any overlapping `domain_names` entry. Within a single
policy arm (`allow` or `deny`) the verdict is the same across all
three, so precedence is currently observable only in logs; it
becomes load-bearing once future iterations let policies mix
verdicts.

Note: `domain_names` is accepted and validated for syntax today but
is **not yet enforced** by the data plane — DNS resolution + cache
lifecycle is a separate iteration. The daemon logs a warning when a
sandbox boots with non-empty `domain_names` so the gap is not
silent.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|ipAddresses|[string]|false|none|none|
|cidrBlocks|[string]|false|none|none|
|domainNames|[string]|false|none|none|

<h2 id="tocS_v1alpha1EstablishSessionResponse">v1alpha1EstablishSessionResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1establishsessionresponse"></a>
<a id="schema_v1alpha1EstablishSessionResponse"></a>
<a id="tocSv1alpha1establishsessionresponse"></a>
<a id="tocsv1alpha1establishsessionresponse"></a>

```json
{
  "acknowledge": {
    "sessionId": "string"
  },
  "event": {
    "id": "string",
    "emittedAt": "2019-08-24T14:15:22Z",
    "sandbox": {
      "metadata": {
        "id": "string",
        "namespace": "string",
        "source": {
          "snapshotId": "string",
          "imageId": "string"
        },
        "version": "string",
        "createdAt": "2019-08-24T14:15:22Z",
        "lastModifiedAt": "2019-08-24T14:15:22Z",
        "labels": {
          "property1": "string",
          "property2": "string"
        }
      },
      "egressPolicy": {
        "allow": {
          "ipAddresses": [
            "string"
          ],
          "cidrBlocks": [
            "string"
          ],
          "domainNames": [
            "string"
          ]
        },
        "deny": {
          "ipAddresses": [
            "string"
          ],
          "cidrBlocks": [
            "string"
          ],
          "domainNames": [
            "string"
          ]
        }
      },
      "resources": {
        "vcpuCount": 0,
        "memoryMib": "string",
        "diskMib": "string"
      },
      "node": {
        "id": "string"
      },
      "intent": {
        "phase": "PHASE_UNSPECIFIED",
        "startSnapshot": {
          "description": "string"
        }
      },
      "lastSnapshot": {
        "snapshotId": "string",
        "createdAt": "2019-08-24T14:15:22Z",
        "error": {
          "code": 0,
          "message": "string",
          "details": [
            {
              "@type": "string",
              "property1": null,
              "property2": null
            }
          ]
        }
      },
      "status": {
        "phase": "PHASE_UNSPECIFIED",
        "message": "string"
      }
    }
  },
  "error": {
    "code": 0,
    "message": "string",
    "details": [
      {
        "@type": "string",
        "property1": null,
        "property2": null
      }
    ]
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|acknowledge|[v1alpha1ConnectionResponse](#schemav1alpha1connectionresponse)|false|none|none|
|event|[v1alpha1Event](#schemav1alpha1event)|false|none|Event represents an activity in the system to which clients may subscribe.<br>It provides enough context for consumers to react, audit, or replicate the change.|
|error|[rpcStatus](#schemarpcstatus)|false|none|The `Status` type defines a logical error model that is suitable for<br>different programming environments, including REST APIs and RPC APIs. It is<br>used by [gRPC](https://github.com/grpc). Each `Status` message contains<br>three pieces of data: error code, error message, and error details.<br><br>You can find out more about this error model and how to work with it in the<br>[API Design Guide](https://cloud.google.com/apis/design/errors).|

<h2 id="tocS_v1alpha1Event">v1alpha1Event</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1event"></a>
<a id="schema_v1alpha1Event"></a>
<a id="tocSv1alpha1event"></a>
<a id="tocsv1alpha1event"></a>

```json
{
  "id": "string",
  "emittedAt": "2019-08-24T14:15:22Z",
  "sandbox": {
    "metadata": {
      "id": "string",
      "namespace": "string",
      "source": {
        "snapshotId": "string",
        "imageId": "string"
      },
      "version": "string",
      "createdAt": "2019-08-24T14:15:22Z",
      "lastModifiedAt": "2019-08-24T14:15:22Z",
      "labels": {
        "property1": "string",
        "property2": "string"
      }
    },
    "egressPolicy": {
      "allow": {
        "ipAddresses": [
          "string"
        ],
        "cidrBlocks": [
          "string"
        ],
        "domainNames": [
          "string"
        ]
      },
      "deny": {
        "ipAddresses": [
          "string"
        ],
        "cidrBlocks": [
          "string"
        ],
        "domainNames": [
          "string"
        ]
      }
    },
    "resources": {
      "vcpuCount": 0,
      "memoryMib": "string",
      "diskMib": "string"
    },
    "node": {
      "id": "string"
    },
    "intent": {
      "phase": "PHASE_UNSPECIFIED",
      "startSnapshot": {
        "description": "string"
      }
    },
    "lastSnapshot": {
      "snapshotId": "string",
      "createdAt": "2019-08-24T14:15:22Z",
      "error": {
        "code": 0,
        "message": "string",
        "details": [
          {
            "@type": "string",
            "property1": null,
            "property2": null
          }
        ]
      }
    },
    "status": {
      "phase": "PHASE_UNSPECIFIED",
      "message": "string"
    }
  }
}

```

Event represents an activity in the system to which clients may subscribe.
It provides enough context for consumers to react, audit, or replicate the change.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|false|none|A unique identifier for this event.|
|emittedAt|string(date-time)|false|none|The timestamp at which this event was emitted by the system.|
|sandbox|[v1alpha1Sandbox](#schemav1alpha1sandbox)|false|none|none|

<h2 id="tocS_v1alpha1GetNodeResponse">v1alpha1GetNodeResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1getnoderesponse"></a>
<a id="schema_v1alpha1GetNodeResponse"></a>
<a id="tocSv1alpha1getnoderesponse"></a>
<a id="tocsv1alpha1getnoderesponse"></a>

```json
{
  "node": {
    "metadata": {
      "id": "string",
      "version": "string",
      "createdAt": "2019-08-24T14:15:22Z",
      "lastModifiedAt": "2019-08-24T14:15:22Z"
    },
    "resources": {
      "cpuCapacityMillicores": 0,
      "memoryCapacityBytes": "string",
      "diskCapacityBytes": "string"
    },
    "metrics": {
      "sampledAt": "2019-08-24T14:15:22Z",
      "activeSandboxCount": 0,
      "cpuUsedMillicores": 0,
      "memoryUsedBytes": "string",
      "diskUsedBytes": "string"
    },
    "status": {
      "phase": "PHASE_UNSPECIFIED",
      "message": "string"
    },
    "awsEc2": {
      "instanceId": "string",
      "instanceType": "string",
      "imageId": "string",
      "accountId": "string",
      "region": "string",
      "availabilityZone": "string",
      "privateIp": "string",
      "kernelId": "string",
      "architecture": "string"
    }
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|node|[v1alpha1Node](#schemav1alpha1node)|false|none|none|

<h2 id="tocS_v1alpha1Intent">v1alpha1Intent</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1intent"></a>
<a id="schema_v1alpha1Intent"></a>
<a id="tocSv1alpha1intent"></a>
<a id="tocsv1alpha1intent"></a>

```json
{
  "phase": "PHASE_UNSPECIFIED",
  "startSnapshot": {
    "description": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|phase|[v1alpha1SandboxStatusPhase](#schemav1alpha1sandboxstatusphase)|false|none|none|
|startSnapshot|[v1alpha1StartSnapshotInput](#schemav1alpha1startsnapshotinput)|false|none|none|

<h2 id="tocS_v1alpha1ListNodesRequestOrder">v1alpha1ListNodesRequestOrder</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1listnodesrequestorder"></a>
<a id="schema_v1alpha1ListNodesRequestOrder"></a>
<a id="tocSv1alpha1listnodesrequestorder"></a>
<a id="tocsv1alpha1listnodesrequestorder"></a>

```json
"ORDER_UNSPECIFIED"

```

Controls how results are ordered by last modification time.

 - ORDER_NEWEST_FIRST: Most recently modified nodes first.
 - ORDER_OLDEST_FIRST: Least recently modified nodes first.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|Controls how results are ordered by last modification time.<br><br> - ORDER_NEWEST_FIRST: Most recently modified nodes first.<br> - ORDER_OLDEST_FIRST: Least recently modified nodes first.|

#### Enumerated Values

|Property|Value|
|---|---|
|*anonymous*|ORDER_UNSPECIFIED|
|*anonymous*|ORDER_NEWEST_FIRST|
|*anonymous*|ORDER_OLDEST_FIRST|

<h2 id="tocS_v1alpha1ListNodesResponse">v1alpha1ListNodesResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1listnodesresponse"></a>
<a id="schema_v1alpha1ListNodesResponse"></a>
<a id="tocSv1alpha1listnodesresponse"></a>
<a id="tocsv1alpha1listnodesresponse"></a>

```json
{
  "nodes": [
    {
      "metadata": {
        "id": "string",
        "version": "string",
        "createdAt": "2019-08-24T14:15:22Z",
        "lastModifiedAt": "2019-08-24T14:15:22Z"
      },
      "resources": {
        "cpuCapacityMillicores": 0,
        "memoryCapacityBytes": "string",
        "diskCapacityBytes": "string"
      },
      "metrics": {
        "sampledAt": "2019-08-24T14:15:22Z",
        "activeSandboxCount": 0,
        "cpuUsedMillicores": 0,
        "memoryUsedBytes": "string",
        "diskUsedBytes": "string"
      },
      "status": {
        "phase": "PHASE_UNSPECIFIED",
        "message": "string"
      },
      "awsEc2": {
        "instanceId": "string",
        "instanceType": "string",
        "imageId": "string",
        "accountId": "string",
        "region": "string",
        "availabilityZone": "string",
        "privateIp": "string",
        "kernelId": "string",
        "architecture": "string"
      }
    }
  ],
  "continuationToken": "string"
}

```

Response message containing a page of nodes.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|nodes|[[v1alpha1Node](#schemav1alpha1node)]|false|none|The list of nodes matching the request filters.|
|continuationToken|string|false|none|Token to retrieve the next page of results.<br>Empty if there are no more results.|

<h2 id="tocS_v1alpha1Node">v1alpha1Node</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1node"></a>
<a id="schema_v1alpha1Node"></a>
<a id="tocSv1alpha1node"></a>
<a id="tocsv1alpha1node"></a>

```json
{
  "metadata": {
    "id": "string",
    "version": "string",
    "createdAt": "2019-08-24T14:15:22Z",
    "lastModifiedAt": "2019-08-24T14:15:22Z"
  },
  "resources": {
    "cpuCapacityMillicores": 0,
    "memoryCapacityBytes": "string",
    "diskCapacityBytes": "string"
  },
  "metrics": {
    "sampledAt": "2019-08-24T14:15:22Z",
    "activeSandboxCount": 0,
    "cpuUsedMillicores": 0,
    "memoryUsedBytes": "string",
    "diskUsedBytes": "string"
  },
  "status": {
    "phase": "PHASE_UNSPECIFIED",
    "message": "string"
  },
  "awsEc2": {
    "instanceId": "string",
    "instanceType": "string",
    "imageId": "string",
    "accountId": "string",
    "region": "string",
    "availabilityZone": "string",
    "privateIp": "string",
    "kernelId": "string",
    "architecture": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|metadata|[v1alpha1NodeMeta](#schemav1alpha1nodemeta)|false|none|none|
|resources|[v1alpha1NodeResources](#schemav1alpha1noderesources)|false|none|none|
|metrics|[v1alpha1NodeMetrics](#schemav1alpha1nodemetrics)|false|none|none|
|status|[v1alpha1NodeStatus](#schemav1alpha1nodestatus)|false|none|none|
|awsEc2|[v1alpha1EC2InstanceMeta](#schemav1alpha1ec2instancemeta)|false|none|none|

<h2 id="tocS_v1alpha1NodeMeta">v1alpha1NodeMeta</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1nodemeta"></a>
<a id="schema_v1alpha1NodeMeta"></a>
<a id="tocSv1alpha1nodemeta"></a>
<a id="tocsv1alpha1nodemeta"></a>

```json
{
  "id": "string",
  "version": "string",
  "createdAt": "2019-08-24T14:15:22Z",
  "lastModifiedAt": "2019-08-24T14:15:22Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|false|none|none|
|version|string(int64)|false|none|none|
|createdAt|string(date-time)|false|none|none|
|lastModifiedAt|string(date-time)|false|none|none|

<h2 id="tocS_v1alpha1NodeMetrics">v1alpha1NodeMetrics</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1nodemetrics"></a>
<a id="schema_v1alpha1NodeMetrics"></a>
<a id="tocSv1alpha1nodemetrics"></a>
<a id="tocsv1alpha1nodemetrics"></a>

```json
{
  "sampledAt": "2019-08-24T14:15:22Z",
  "activeSandboxCount": 0,
  "cpuUsedMillicores": 0,
  "memoryUsedBytes": "string",
  "diskUsedBytes": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|sampledAt|string(date-time)|false|none|Time at which the metrics were sampled.|
|activeSandboxCount|integer(int64)|false|none|Number of currently active sandboxes.|
|cpuUsedMillicores|integer(int64)|false|none|CPU currently in use.|
|memoryUsedBytes|string(uint64)|false|none|Memory currently in use.|
|diskUsedBytes|string(uint64)|false|none|Disk currently in use.|

<h2 id="tocS_v1alpha1NodeRef">v1alpha1NodeRef</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1noderef"></a>
<a id="schema_v1alpha1NodeRef"></a>
<a id="tocSv1alpha1noderef"></a>
<a id="tocsv1alpha1noderef"></a>

```json
{
  "id": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|false|none|none|

<h2 id="tocS_v1alpha1NodeResources">v1alpha1NodeResources</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1noderesources"></a>
<a id="schema_v1alpha1NodeResources"></a>
<a id="tocSv1alpha1noderesources"></a>
<a id="tocsv1alpha1noderesources"></a>

```json
{
  "cpuCapacityMillicores": 0,
  "memoryCapacityBytes": "string",
  "diskCapacityBytes": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|cpuCapacityMillicores|integer(int64)|false|none|Total allocatable vCPUs available for workloads.|
|memoryCapacityBytes|string(uint64)|false|none|Total allocatable memory available for workloads.|
|diskCapacityBytes|string(uint64)|false|none|Total allocatable disk available for workloads.|

<h2 id="tocS_v1alpha1NodeStatus">v1alpha1NodeStatus</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1nodestatus"></a>
<a id="schema_v1alpha1NodeStatus"></a>
<a id="tocSv1alpha1nodestatus"></a>
<a id="tocsv1alpha1nodestatus"></a>

```json
{
  "phase": "PHASE_UNSPECIFIED",
  "message": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|phase|[v1alpha1NodeStatusPhase](#schemav1alpha1nodestatusphase)|false|none|- PHASE_HEALTHY: Node is healthy and able to accept workloads.<br> - PHASE_UNHEALTHY: Node is reachable but degraded.<br> - PHASE_LOST: Node has not reported recently and is considered lost.<br> - PHASE_DELETED: Node is being removed or is no longer active.<br> - PHASE_UNKNOWN: Status could not be determined due to transient failures.|
|message|string|false|none|Human-readable status message.|

<h2 id="tocS_v1alpha1NodeStatusPhase">v1alpha1NodeStatusPhase</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1nodestatusphase"></a>
<a id="schema_v1alpha1NodeStatusPhase"></a>
<a id="tocSv1alpha1nodestatusphase"></a>
<a id="tocsv1alpha1nodestatusphase"></a>

```json
"PHASE_UNSPECIFIED"

```

 - PHASE_HEALTHY: Node is healthy and able to accept workloads.
 - PHASE_UNHEALTHY: Node is reachable but degraded.
 - PHASE_LOST: Node has not reported recently and is considered lost.
 - PHASE_DELETED: Node is being removed or is no longer active.
 - PHASE_UNKNOWN: Status could not be determined due to transient failures.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|- PHASE_HEALTHY: Node is healthy and able to accept workloads.<br> - PHASE_UNHEALTHY: Node is reachable but degraded.<br> - PHASE_LOST: Node has not reported recently and is considered lost.<br> - PHASE_DELETED: Node is being removed or is no longer active.<br> - PHASE_UNKNOWN: Status could not be determined due to transient failures.|

#### Enumerated Values

|Property|Value|
|---|---|
|*anonymous*|PHASE_UNSPECIFIED|
|*anonymous*|PHASE_HEALTHY|
|*anonymous*|PHASE_UNHEALTHY|
|*anonymous*|PHASE_LOST|
|*anonymous*|PHASE_DELETED|
|*anonymous*|PHASE_UNKNOWN|

<h2 id="tocS_v1alpha1PatchNodeRequest">v1alpha1PatchNodeRequest</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1patchnoderequest"></a>
<a id="schema_v1alpha1PatchNodeRequest"></a>
<a id="tocSv1alpha1patchnoderequest"></a>
<a id="tocsv1alpha1patchnoderequest"></a>

```json
{
  "nodeMetrics": {
    "sampledAt": "2019-08-24T14:15:22Z",
    "activeSandboxCount": 0,
    "cpuUsedMillicores": 0,
    "memoryUsedBytes": "string",
    "diskUsedBytes": "string"
  },
  "nodeStatus": {
    "phase": "PHASE_UNSPECIFIED",
    "message": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|nodeMetrics|[v1alpha1NodeMetrics](#schemav1alpha1nodemetrics)|false|none|none|
|nodeStatus|[v1alpha1NodeStatus](#schemav1alpha1nodestatus)|false|none|none|

<h2 id="tocS_v1alpha1Resources">v1alpha1Resources</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1resources"></a>
<a id="schema_v1alpha1Resources"></a>
<a id="tocSv1alpha1resources"></a>
<a id="tocsv1alpha1resources"></a>

```json
{
  "vcpuCount": 0,
  "memoryMib": "string",
  "diskMib": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|vcpuCount|integer(int64)|false|none|none|
|memoryMib|string(uint64)|false|none|Memory in MiB. Lower bound matches the smallest useful<br>Firecracker VM; upper bound leaves room for 128 GiB sandboxes.|
|diskMib|string(uint64)|false|none|Root disk size in MiB. Optional: 0 means "use the daemon's<br>configured default". When set, the daemon resizes the per-VM<br>copy of the base rootfs image up to this size at provision time<br>(ext4 grow on a sparse file — metadata-only, no eager allocation).<br>Lower bound is the base image size; the upper bound is generous<br>(1 TiB) on the spec side, with hosts further constrained by<br>their advertised disk_capacity_bytes.|

<h2 id="tocS_v1alpha1Sandbox">v1alpha1Sandbox</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1sandbox"></a>
<a id="schema_v1alpha1Sandbox"></a>
<a id="tocSv1alpha1sandbox"></a>
<a id="tocsv1alpha1sandbox"></a>

```json
{
  "metadata": {
    "id": "string",
    "namespace": "string",
    "source": {
      "snapshotId": "string",
      "imageId": "string"
    },
    "version": "string",
    "createdAt": "2019-08-24T14:15:22Z",
    "lastModifiedAt": "2019-08-24T14:15:22Z",
    "labels": {
      "property1": "string",
      "property2": "string"
    }
  },
  "egressPolicy": {
    "allow": {
      "ipAddresses": [
        "string"
      ],
      "cidrBlocks": [
        "string"
      ],
      "domainNames": [
        "string"
      ]
    },
    "deny": {
      "ipAddresses": [
        "string"
      ],
      "cidrBlocks": [
        "string"
      ],
      "domainNames": [
        "string"
      ]
    }
  },
  "resources": {
    "vcpuCount": 0,
    "memoryMib": "string",
    "diskMib": "string"
  },
  "node": {
    "id": "string"
  },
  "intent": {
    "phase": "PHASE_UNSPECIFIED",
    "startSnapshot": {
      "description": "string"
    }
  },
  "lastSnapshot": {
    "snapshotId": "string",
    "createdAt": "2019-08-24T14:15:22Z",
    "error": {
      "code": 0,
      "message": "string",
      "details": [
        {
          "@type": "string",
          "property1": null,
          "property2": null
        }
      ]
    }
  },
  "status": {
    "phase": "PHASE_UNSPECIFIED",
    "message": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|metadata|[v1alpha1SandboxMeta](#schemav1alpha1sandboxmeta)|false|none|none|
|egressPolicy|[v1alpha1EgressPolicy](#schemav1alpha1egresspolicy)|false|none|EgressPolicy constrains the external destinations a sandbox can<br>reach from inside its guest. It is enforced by the data-plane daemon<br>at sandbox-boot time as netfilter rules in the VM's network<br>namespace; the api-server only validates syntax.<br><br>Semantics:<br>  - Unset (the default): no restriction. The sandbox can reach any<br>    external target reachable from the host.<br>  - `allow` set: only destinations matching one of the listed targets<br>    are reachable. Everything else is rejected with ICMP admin-<br>    prohibited (TCP fails fast with EHOSTUNREACH-class errors).<br>  - `deny` set: every destination is reachable except those matching<br>    one of the listed targets, which are rejected as above.<br><br>There is intentionally no implicit DNS pass-through: under an<br>`allow` policy a sandbox cannot reach its resolver unless the<br>resolver's IP is listed.|
|resources|[v1alpha1Resources](#schemav1alpha1resources)|false|none|none|
|node|[v1alpha1NodeRef](#schemav1alpha1noderef)|false|none|none|
|intent|[v1alpha1Intent](#schemav1alpha1intent)|false|none|none|
|lastSnapshot|[v1alpha1SnapshotOutput](#schemav1alpha1snapshotoutput)|false|none|none|
|status|[v1alpha1SandboxStatus](#schemav1alpha1sandboxstatus)|false|none|none|

<h2 id="tocS_v1alpha1SandboxMeta">v1alpha1SandboxMeta</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1sandboxmeta"></a>
<a id="schema_v1alpha1SandboxMeta"></a>
<a id="tocSv1alpha1sandboxmeta"></a>
<a id="tocsv1alpha1sandboxmeta"></a>

```json
{
  "id": "string",
  "namespace": "string",
  "source": {
    "snapshotId": "string",
    "imageId": "string"
  },
  "version": "string",
  "createdAt": "2019-08-24T14:15:22Z",
  "lastModifiedAt": "2019-08-24T14:15:22Z",
  "labels": {
    "property1": "string",
    "property2": "string"
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|false|none|none|
|namespace|string|false|none|none|
|source|[v1alpha1SandboxSource](#schemav1alpha1sandboxsource)|false|none|none|
|version|string(int64)|false|none|none|
|createdAt|string(date-time)|false|none|none|
|lastModifiedAt|string(date-time)|false|none|none|
|labels|object|false|none|Arbitrary key/value labels for client-side grouping and<br>filtering (e.g. "project=foo", "ci-run=123"). Keys are required<br>to be 1-63 chars, lowercase alphanumeric with dots/dashes/<br>underscores, starting and ending with an alphanumeric. Values<br>may be empty or follow the same rules (uppercase permitted).|
|» **additionalProperties**|string|false|none|none|

<h2 id="tocS_v1alpha1SandboxSource">v1alpha1SandboxSource</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1sandboxsource"></a>
<a id="schema_v1alpha1SandboxSource"></a>
<a id="tocSv1alpha1sandboxsource"></a>
<a id="tocsv1alpha1sandboxsource"></a>

```json
{
  "snapshotId": "string",
  "imageId": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|snapshotId|string|false|none|none|
|imageId|string|false|none|none|

<h2 id="tocS_v1alpha1SandboxStatus">v1alpha1SandboxStatus</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1sandboxstatus"></a>
<a id="schema_v1alpha1SandboxStatus"></a>
<a id="tocSv1alpha1sandboxstatus"></a>
<a id="tocsv1alpha1sandboxstatus"></a>

```json
{
  "phase": "PHASE_UNSPECIFIED",
  "message": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|phase|[v1alpha1SandboxStatusPhase](#schemav1alpha1sandboxstatusphase)|false|none|none|
|message|string|false|none|none|

<h2 id="tocS_v1alpha1SandboxStatusPhase">v1alpha1SandboxStatusPhase</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1sandboxstatusphase"></a>
<a id="schema_v1alpha1SandboxStatusPhase"></a>
<a id="tocSv1alpha1sandboxstatusphase"></a>
<a id="tocsv1alpha1sandboxstatusphase"></a>

```json
"PHASE_UNSPECIFIED"

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|*anonymous*|PHASE_UNSPECIFIED|
|*anonymous*|PHASE_PENDING|
|*anonymous*|PHASE_RUNNING|
|*anonymous*|PHASE_PAUSING|
|*anonymous*|PHASE_PAUSED|
|*anonymous*|PHASE_RESUMING|
|*anonymous*|PHASE_SNAPSHOTTING|
|*anonymous*|PHASE_DELETING|
|*anonymous*|PHASE_DELETED|
|*anonymous*|PHASE_FAILED|

<h2 id="tocS_v1alpha1SnapshotOutput">v1alpha1SnapshotOutput</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1snapshotoutput"></a>
<a id="schema_v1alpha1SnapshotOutput"></a>
<a id="tocSv1alpha1snapshotoutput"></a>
<a id="tocsv1alpha1snapshotoutput"></a>

```json
{
  "snapshotId": "string",
  "createdAt": "2019-08-24T14:15:22Z",
  "error": {
    "code": 0,
    "message": "string",
    "details": [
      {
        "@type": "string",
        "property1": null,
        "property2": null
      }
    ]
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|snapshotId|string|false|none|none|
|createdAt|string(date-time)|false|none|none|
|error|[rpcStatus](#schemarpcstatus)|false|none|The `Status` type defines a logical error model that is suitable for<br>different programming environments, including REST APIs and RPC APIs. It is<br>used by [gRPC](https://github.com/grpc). Each `Status` message contains<br>three pieces of data: error code, error message, and error details.<br><br>You can find out more about this error model and how to work with it in the<br>[API Design Guide](https://cloud.google.com/apis/design/errors).|

<h2 id="tocS_v1alpha1StartSnapshotInput">v1alpha1StartSnapshotInput</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1startsnapshotinput"></a>
<a id="schema_v1alpha1StartSnapshotInput"></a>
<a id="tocSv1alpha1startsnapshotinput"></a>
<a id="tocsv1alpha1startsnapshotinput"></a>

```json
{
  "description": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|description|string|false|none|none|

<h2 id="tocS_v1alpha1UpdateSandboxRequest">v1alpha1UpdateSandboxRequest</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1updatesandboxrequest"></a>
<a id="schema_v1alpha1UpdateSandboxRequest"></a>
<a id="tocSv1alpha1updatesandboxrequest"></a>
<a id="tocsv1alpha1updatesandboxrequest"></a>

```json
{
  "sandbox": {
    "metadata": {
      "id": "string",
      "namespace": "string",
      "source": {
        "snapshotId": "string",
        "imageId": "string"
      },
      "version": "string",
      "createdAt": "2019-08-24T14:15:22Z",
      "lastModifiedAt": "2019-08-24T14:15:22Z",
      "labels": {
        "property1": "string",
        "property2": "string"
      }
    },
    "egressPolicy": {
      "allow": {
        "ipAddresses": [
          "string"
        ],
        "cidrBlocks": [
          "string"
        ],
        "domainNames": [
          "string"
        ]
      },
      "deny": {
        "ipAddresses": [
          "string"
        ],
        "cidrBlocks": [
          "string"
        ],
        "domainNames": [
          "string"
        ]
      }
    },
    "resources": {
      "vcpuCount": 0,
      "memoryMib": "string",
      "diskMib": "string"
    },
    "node": {
      "id": "string"
    },
    "intent": {
      "phase": "PHASE_UNSPECIFIED",
      "startSnapshot": {
        "description": "string"
      }
    },
    "lastSnapshot": {
      "snapshotId": "string",
      "createdAt": "2019-08-24T14:15:22Z",
      "error": {
        "code": 0,
        "message": "string",
        "details": [
          {
            "@type": "string",
            "property1": null,
            "property2": null
          }
        ]
      }
    },
    "status": {
      "phase": "PHASE_UNSPECIFIED",
      "message": "string"
    }
  }
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|sandbox|[v1alpha1Sandbox](#schemav1alpha1sandbox)|false|none|none|

