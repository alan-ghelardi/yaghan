---
title: yaghan/control_plane/v1alpha1/sandbox.proto version not set
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

<h1 id="yaghan-control_plane-v1alpha1-sandbox-proto">yaghan/control_plane/v1alpha1/sandbox.proto version not set</h1>

> Scroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

<h1 id="yaghan-control_plane-v1alpha1-sandbox-proto-sandboxservice">SandboxService</h1>

## SandboxService_ListSandboxes

<a id="opIdSandboxService_ListSandboxes"></a>

`GET /v1alpha1/sandboxes`

<h3 id="sandboxservice_listsandboxes-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|namespace|query|string|false|Filters sandboxes by namespace.|
|nodeId|query|string|false|Filters sandboxes by the ID of the node where they are scheduled or running.|
|statusPhase|query|string|false|Filters sandboxes by their current lifecycle phase.|
|continuationToken|query|string|false|Token used for pagination.|
|pageSize|query|integer(int32)|false|Maximum number of sandboxes to return in this request.|
|sortOrder|query|string|false|Sort order applied to the results based on last_modified_at.|

#### Detailed descriptions

**namespace**: Filters sandboxes by namespace.
When provided, must match the format:
- starts with a lowercase letter
- contains only lowercase alphanumeric characters or hyphens
- ends with an alphanumeric character
Example: "default", "team-a"
May be empty when node_id is supplied (e.g. the data-plane daemon's
per-node resync scan).

**statusPhase**: Filters sandboxes by their current lifecycle phase.
If unset, sandboxes in all phases are returned.

**continuationToken**: Token used for pagination.
Pass the value returned in a previous response to retrieve the next page of results.
Leave empty to start listing from the beginning.

**pageSize**: Maximum number of sandboxes to return in this request.
Defaults to 30 if not specified.
The maximum allowed value is 1000.

**sortOrder**: Sort order applied to the results based on last_modified_at.

 - ORDER_NEWEST_FIRST: Most recently modified sandboxes first.
 - ORDER_OLDEST_FIRST: Least recently modified sandboxes first.

#### Enumerated Values

|Parameter|Value|
|---|---|
|statusPhase|PHASE_UNSPECIFIED|
|statusPhase|PHASE_PENDING|
|statusPhase|PHASE_RUNNING|
|statusPhase|PHASE_PAUSING|
|statusPhase|PHASE_PAUSED|
|statusPhase|PHASE_RESUMING|
|statusPhase|PHASE_SNAPSHOTTING|
|statusPhase|PHASE_DELETING|
|statusPhase|PHASE_DELETED|
|statusPhase|PHASE_FAILED|
|sortOrder|ORDER_UNSPECIFIED|
|sortOrder|ORDER_NEWEST_FIRST|
|sortOrder|ORDER_OLDEST_FIRST|

> Example responses

> 200 Response

```json
{
  "sandboxes": [
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
  ],
  "continuationToken": "string"
}
```

<h3 id="sandboxservice_listsandboxes-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1ListSandboxesResponse](#schemav1alpha1listsandboxesresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

## SandboxService_CreateSandbox

<a id="opIdSandboxService_CreateSandbox"></a>

`POST /v1alpha1/sandboxes`

> Body parameter

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

<h3 id="sandboxservice_createsandbox-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[v1alpha1CreateSandboxRequest](#schemav1alpha1createsandboxrequest)|true|none|

> Example responses

> 200 Response

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

<h3 id="sandboxservice_createsandbox-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1CreateSandboxResponse](#schemav1alpha1createsandboxresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

## SandboxService_GetSandbox

<a id="opIdSandboxService_GetSandbox"></a>

`GET /v1alpha1/sandboxes/{sandboxId}`

<h3 id="sandboxservice_getsandbox-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|sandboxId|path|string|true|none|

> Example responses

> 200 Response

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

<h3 id="sandboxservice_getsandbox-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1GetSandboxResponse](#schemav1alpha1getsandboxresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

## SandboxService_DeleteSandbox

<a id="opIdSandboxService_DeleteSandbox"></a>

`DELETE /v1alpha1/sandboxes/{sandboxId}`

<h3 id="sandboxservice_deletesandbox-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|sandboxId|path|string|true|none|
|version|query|string(int64)|false|none|

> Example responses

> 200 Response

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

<h3 id="sandboxservice_deletesandbox-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1DeleteSandboxResponse](#schemav1alpha1deletesandboxresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

## SandboxService_PauseSandbox

<a id="opIdSandboxService_PauseSandbox"></a>

`PUT /v1alpha1/sandboxes/{sandboxId}/pause`

<h3 id="sandboxservice_pausesandbox-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|sandboxId|path|string|true|none|
|version|query|string(int64)|false|none|

> Example responses

> 200 Response

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

<h3 id="sandboxservice_pausesandbox-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1PauseSandboxResponse](#schemav1alpha1pausesandboxresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

## SandboxService_ResumeSandbox

<a id="opIdSandboxService_ResumeSandbox"></a>

`PUT /v1alpha1/sandboxes/{sandboxId}/resume`

<h3 id="sandboxservice_resumesandbox-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|sandboxId|path|string|true|none|
|version|query|string(int64)|false|none|

> Example responses

> 200 Response

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

<h3 id="sandboxservice_resumesandbox-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1ResumeSandboxResponse](#schemav1alpha1resumesandboxresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

## SandboxService_StartSnapshot

<a id="opIdSandboxService_StartSnapshot"></a>

`POST /v1alpha1/sandboxes/{sandboxId}/snapshots`

> Body parameter

```json
{
  "version": "string",
  "description": "string"
}
```

<h3 id="sandboxservice_startsnapshot-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|sandboxId|path|string|true|none|
|body|body|[SandboxServiceStartSnapshotBody](#schemasandboxservicestartsnapshotbody)|true|none|

> Example responses

> 200 Response

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

<h3 id="sandboxservice_startsnapshot-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|A successful response.|[v1alpha1StartSnapshotResponse](#schemav1alpha1startsnapshotresponse)|
|default|Default|An unexpected error response.|[rpcStatus](#schemarpcstatus)|

<aside class="success">
This operation does not require authentication
</aside>

# Schemas

<h2 id="tocS_SandboxServiceStartSnapshotBody">SandboxServiceStartSnapshotBody</h2>
<!-- backwards compatibility -->
<a id="schemasandboxservicestartsnapshotbody"></a>
<a id="schema_SandboxServiceStartSnapshotBody"></a>
<a id="tocSsandboxservicestartsnapshotbody"></a>
<a id="tocssandboxservicestartsnapshotbody"></a>

```json
{
  "version": "string",
  "description": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|version|string(int64)|false|none|none|
|description|string|false|none|none|

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

<h2 id="tocS_v1alpha1CreateSandboxRequest">v1alpha1CreateSandboxRequest</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1createsandboxrequest"></a>
<a id="schema_v1alpha1CreateSandboxRequest"></a>
<a id="tocSv1alpha1createsandboxrequest"></a>
<a id="tocsv1alpha1createsandboxrequest"></a>

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

<h2 id="tocS_v1alpha1CreateSandboxResponse">v1alpha1CreateSandboxResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1createsandboxresponse"></a>
<a id="schema_v1alpha1CreateSandboxResponse"></a>
<a id="tocSv1alpha1createsandboxresponse"></a>
<a id="tocsv1alpha1createsandboxresponse"></a>

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

<h2 id="tocS_v1alpha1DeleteSandboxResponse">v1alpha1DeleteSandboxResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1deletesandboxresponse"></a>
<a id="schema_v1alpha1DeleteSandboxResponse"></a>
<a id="tocSv1alpha1deletesandboxresponse"></a>
<a id="tocsv1alpha1deletesandboxresponse"></a>

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

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|allow|[v1alpha1EgressTargets](#schemav1alpha1egresstargets)|false|none|none|
|deny|[v1alpha1EgressTargets](#schemav1alpha1egresstargets)|false|none|none|

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

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|ipAddresses|[string]|false|none|none|
|cidrBlocks|[string]|false|none|none|
|domainNames|[string]|false|none|none|

<h2 id="tocS_v1alpha1GetSandboxResponse">v1alpha1GetSandboxResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1getsandboxresponse"></a>
<a id="schema_v1alpha1GetSandboxResponse"></a>
<a id="tocSv1alpha1getsandboxresponse"></a>
<a id="tocsv1alpha1getsandboxresponse"></a>

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

<h2 id="tocS_v1alpha1ListSandboxesRequestOrder">v1alpha1ListSandboxesRequestOrder</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1listsandboxesrequestorder"></a>
<a id="schema_v1alpha1ListSandboxesRequestOrder"></a>
<a id="tocSv1alpha1listsandboxesrequestorder"></a>
<a id="tocsv1alpha1listsandboxesrequestorder"></a>

```json
"ORDER_UNSPECIFIED"

```

Controls how results are ordered by last modification time.

 - ORDER_NEWEST_FIRST: Most recently modified sandboxes first.
 - ORDER_OLDEST_FIRST: Least recently modified sandboxes first.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|Controls how results are ordered by last modification time.<br><br> - ORDER_NEWEST_FIRST: Most recently modified sandboxes first.<br> - ORDER_OLDEST_FIRST: Least recently modified sandboxes first.|

#### Enumerated Values

|Property|Value|
|---|---|
|*anonymous*|ORDER_UNSPECIFIED|
|*anonymous*|ORDER_NEWEST_FIRST|
|*anonymous*|ORDER_OLDEST_FIRST|

<h2 id="tocS_v1alpha1ListSandboxesResponse">v1alpha1ListSandboxesResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1listsandboxesresponse"></a>
<a id="schema_v1alpha1ListSandboxesResponse"></a>
<a id="tocSv1alpha1listsandboxesresponse"></a>
<a id="tocsv1alpha1listsandboxesresponse"></a>

```json
{
  "sandboxes": [
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
  ],
  "continuationToken": "string"
}

```

Response message containing a page of sandboxes.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|sandboxes|[[v1alpha1Sandbox](#schemav1alpha1sandbox)]|false|none|The list of sandboxes matching the request filters.|
|continuationToken|string|false|none|Token to retrieve the next page of results.<br>Empty if there are no more results.|

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

<h2 id="tocS_v1alpha1PauseSandboxResponse">v1alpha1PauseSandboxResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1pausesandboxresponse"></a>
<a id="schema_v1alpha1PauseSandboxResponse"></a>
<a id="tocSv1alpha1pausesandboxresponse"></a>
<a id="tocsv1alpha1pausesandboxresponse"></a>

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

<h2 id="tocS_v1alpha1ResumeSandboxResponse">v1alpha1ResumeSandboxResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1resumesandboxresponse"></a>
<a id="schema_v1alpha1ResumeSandboxResponse"></a>
<a id="tocSv1alpha1resumesandboxresponse"></a>
<a id="tocsv1alpha1resumesandboxresponse"></a>

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
|egressPolicy|[v1alpha1EgressPolicy](#schemav1alpha1egresspolicy)|false|none|none|
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

<h2 id="tocS_v1alpha1StartSnapshotResponse">v1alpha1StartSnapshotResponse</h2>
<!-- backwards compatibility -->
<a id="schemav1alpha1startsnapshotresponse"></a>
<a id="schema_v1alpha1StartSnapshotResponse"></a>
<a id="tocSv1alpha1startsnapshotresponse"></a>
<a id="tocsv1alpha1startsnapshotresponse"></a>

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

