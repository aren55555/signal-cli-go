# signal-cli 0.14.7 JSON-RPC daemon: raw observations (zero accounts)

## Sources

- Reference binary: signal-cli 0.14.7 release (`/work/third_party/signal-cli-0.14.7/bin/signal-cli`,
  installed by the repo's oracle installer), Java 25, run inside the sandbox on 2026-10-23.
- All runs used `--config <CONFIG>` with an empty config directory (no accounts), so every
  daemon ran in **multi-account mode**. Every run had
  `JAVA_OPTS="-Djava.io.tmpdir=<writable dir>"` set (the sandbox's /tmp is noexec and
  libsignal's JNI library must be extracted to an executable temp dir).
- No Signal server was contacted; no account was registered or linked.
- Upstream source for cross-reference: https://github.com/AsamK/signal-cli/tree/v0.14.7

Placeholders: `<CONFIG>` = config dir, `<PORT>` = the TCP/HTTP port used. HTTP `Date:` headers removed.
The JDK HTTP server emits header names as `Content-type` / `Content-length` /
`Transfer-encoding` (only first letter capitalised) for handler responses; its own built-in
error pages use `Content-Type` / `Content-Length`.

### Daemons started

    signal-cli --config <CONFIG> daemon --http 127.0.0.1:<PORT> --tcp 127.0.0.1:<PORT> --socket <CONFIG>/../rpc-spec.sock

stderr at startup:

    INFO  DaemonCommand - Starting daemon in multi-account mode
    INFO  SocketHandler - Started JSON-RPC server on <CONFIG>/../rpc-spec.sock
    INFO  SocketHandler - Started JSON-RPC server on /127.0.0.1:<PORT>
    INFO  HttpServerHandler - Started HTTP server on /127.0.0.1:<PORT>
    WARN  HttpServerHandler - HTTP server has no authentication; Host header is pinned to [localhost, ::1, 127.0.0.1]

On SIGTERM stderr shows `INFO  Shutdown - Received SIGTERM signal, shutting down ...`, and the
UNIX socket file is removed.

Second daemon (manual receive mode, HTTP bound to the wildcard address briefly to observe the
Host-check bypass; it was killed immediately afterwards):

    signal-cli --config <CONFIG> daemon --receive-mode manual --tcp 127.0.0.1:<PORT> --http 0.0.0.0:<PORT>

    INFO  DaemonCommand - Starting daemon in multi-account mode
    INFO  SocketHandler - Started JSON-RPC server on /127.0.0.1:<PORT>
    INFO  HttpServerHandler - Started HTTP server on /0.0.0.0:<PORT>
    WARN  HttpServerHandler - HTTP server has no authentication; Host header validation DISABLED because listening on /0.0.0.0:<PORT>

## 1. HTTP `POST /api/v1/rpc`: request-level parsing

Each block: request line + Content-Type, raw body bytes, then the raw response.
Requests sent with `curl -s -i -X POST -H "Content-Type: ..." --data-binary BODY`.

```
=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
not json
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 289

{"jsonrpc":"2.0","error":{"code":-32700,"message":"Unrecognized token 'not': was expecting (JSON String, Number, Array, Object or token 'null', 'true' or 'false')\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 1, column: 1]","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json

--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 100

{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: MISSING","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
   
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 100

{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: MISSING","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[]
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 91

{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid request","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
42
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 99

{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: NUMBER","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
"str"
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 99

{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: STRING","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
null
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 97

{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: NULL","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 113

{"jsonrpc":"2.0","error":{"code":-32600,"message":"only jsonrpc version 2.0 is supported","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","id":5}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 97

{"jsonrpc":"2.0","error":{"code":-32600,"message":"method field must be set","data":null},"id":5}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"1.0","method":"version","id":6}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 110

{"jsonrpc":"2.0","error":{"code":-32600,"message":"only jsonrpc version 2.0 is supported","data":null},"id":6}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"method":"version","id":7}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 110

{"jsonrpc":"2.0","error":{"code":-32600,"message":"only jsonrpc version 2.0 is supported","data":null},"id":7}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version"}
--- RESPONSE
HTTP/1.1 201 Created
Content-length: 0



=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":"abc"}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 58

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":"abc"}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":null}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 57

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":1.5}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 56

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1.5}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":[1]}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 544

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Problem deserializing property 'id' (expected type: [simple type, class com.fasterxml.jackson.databind.node.ValueNode]; actual type: `com.fasterxml.jackson.databind.node.ArrayNode`), problem: Can not set com.fasterxml.jackson.databind.node.ValueNode field org.asamk.signal.jsonrpc.JsonRpcRequest.id to com.fasterxml.jackson.databind.node.ArrayNode\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest[\"id\"])","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":{"a":1}}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 546

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Problem deserializing property 'id' (expected type: [simple type, class com.fasterxml.jackson.databind.node.ValueNode]; actual type: `com.fasterxml.jackson.databind.node.ObjectNode`), problem: Can not set com.fasterxml.jackson.databind.node.ValueNode field org.asamk.signal.jsonrpc.JsonRpcRequest.id to com.fasterxml.jackson.databind.node.ObjectNode\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest[\"id\"])","data":null},"id":null}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":null,"id":8}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":8}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":[],"id":9}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 261

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot deserialize value of type `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":9}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":"x","id":10}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 578

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Problem deserializing property 'params' (expected type: [simple type, class com.fasterxml.jackson.databind.node.ContainerNode<java.lang.Object>]; actual type: `com.fasterxml.jackson.databind.node.TextNode`), problem: Can not set com.fasterxml.jackson.databind.node.ContainerNode field org.asamk.signal.jsonrpc.JsonRpcRequest.params to com.fasterxml.jackson.databind.node.TextNode\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest[\"params\"])","data":null},"id":10}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":{"foo":1},"id":11}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 55

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":11}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":123,"id":12}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":12}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":13,"extra":true}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 365

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Unrecognized field \"extra\" (class org.asamk.signal.jsonrpc.JsonRpcRequest), not marked as ignorable (4 known properties: \"method\", \"id\", \"jsonrpc\", \"params\")\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest[\"extra\"])","data":null},"id":13}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":2.0,"method":"version","id":14}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 55

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":14}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":15} trailing
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 55

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":15}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":16}
{"jsonrpc":"2.0","method":"version","id":17}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 55

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":16}

```

## 2. HTTP `POST /api/v1/rpc`: batches, responses-as-input, notifications, account selection, method names

```
=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[{"jsonrpc":"2.0","method":"version","id":1},{"jsonrpc":"2.0","method":"listAccounts","id":2}]
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 93

[{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1},{"jsonrpc":"2.0","result":[],"id":2}]

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[{"jsonrpc":"2.0","method":"version"}]
--- RESPONSE
HTTP/1.1 201 Created
Content-length: 0



=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[{"jsonrpc":"2.0","method":"version"},{"jsonrpc":"2.0","method":"version","id":3}]
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 56

[{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":3}]

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[1,2]
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 635

[{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` (although at least one Creator exists): no int/Int-argument constructor/factory method to deserialize from Number value (1)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":null},{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` (although at least one Creator exists): no int/Int-argument constructor/factory method to deserialize from Number value (2)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":null}]

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[{"jsonrpc":"2.0","method":"version","id":1},"x",{"jsonrpc":"1.0","method":"version","id":4},{"jsonrpc":"2.0","method":"nope","id":5}]
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 581

[{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` (although at least one Creator exists): no String-argument constructor/factory method to deserialize from String value ('x')\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":null},{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1},{"jsonrpc":"2.0","error":{"code":-32600,"message":"only jsonrpc version 2.0 is supported","data":null},"id":4},{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":5}]

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[[]]
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 247

[{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot deserialize value of type `org.asamk.signal.jsonrpc.JsonRpcRequest` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":null}]

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
[{"jsonrpc":"2.0","result":1,"id":1}]
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 368

[{"jsonrpc":"2.0","error":{"code":-32600,"message":"Unrecognized field \"result\" (class org.asamk.signal.jsonrpc.JsonRpcRequest), not marked as ignorable (4 known properties: \"method\", \"id\", \"jsonrpc\", \"params\")\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest[\"result\"])","data":null},"id":1}]

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","result":1,"id":1}
--- RESPONSE
HTTP/1.1 201 Created
Content-length: 0



=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","error":{"code":1,"message":"x"},"id":1}
--- RESPONSE
HTTP/1.1 201 Created
Content-length: 0



=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"nope"}
--- RESPONSE
HTTP/1.1 201 Created
Content-length: 0



=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"send","id":20}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 113

{"jsonrpc":"2.0","error":{"code":-32602,"message":"Method requires valid account parameter","data":null},"id":20}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"send","params":{"account":"+15550000000"},"id":21}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 106

{"jsonrpc":"2.0","error":{"code":-32602,"message":"Specified account does not exist","data":null},"id":21}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"send","params":{"account":null},"id":22}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 113

{"jsonrpc":"2.0","error":{"code":-32602,"message":"Method requires valid account parameter","data":null},"id":22}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":{"account":"+15550000000"},"id":23}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 106

{"jsonrpc":"2.0","error":{"code":-32602,"message":"Specified account does not exist","data":null},"id":23}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"listAccounts","params":{"account":"+15550000000"},"id":24}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 37

{"jsonrpc":"2.0","result":[],"id":24}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"subscribeReceive","id":25}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":25}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"unsubscribeReceive","id":26}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":26}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"unsubscribeReceive","params":{"subscription":999},"id":27}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":27}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"subscribeCallEvents","id":28}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":28}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"unsubscribeCallEvents","params":[0],"id":29}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":29}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"link","id":30}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":30}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"daemon","id":31}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":31}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"jsonRpc","id":32}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":32}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"listContacts","params":{"account":"+15550000000"},"id":33}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 106

{"jsonrpc":"2.0","error":{"code":-32602,"message":"Specified account does not exist","data":null},"id":33}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"VERSION","id":34}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":34}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"list-accounts","id":35}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 96

{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":35}

```

## 3. HTTP paths, verbs, Content-Type, Host header, /check, /events

Each block header is the curl argument list (`curl -s -i -m 5 ARGS`).

```
=== http://127.0.0.1:<PORT>/api/v1/check
HTTP/1.1 200 OK
Content-length: 0


=== -X POST http://127.0.0.1:<PORT>/api/v1/check
HTTP/1.1 405 Method Not Allowed
Content-length: 0


=== -X HEAD http://127.0.0.1:<PORT>/api/v1/check
HTTP/1.1 405 Method Not Allowed


=== http://127.0.0.1:<PORT>/api/v1/check/
HTTP/1.1 404 Not Found
Content-length: 0


=== http://127.0.0.1:<PORT>/api/v1/check?x=1
HTTP/1.1 200 OK
Content-length: 0


=== http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 405 Method Not Allowed
Content-length: 0


=== -X PUT -H Content-Type: application/json --data-binary {"jsonrpc":"2.0","method":"version","id":1} http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 405 Method Not Allowed
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 415 Unsupported Media Type
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: text/plain http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 415 Unsupported Media Type
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 415 Unsupported Media Type
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json; charset=utf-8 http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json-rpc http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: Application/JSON http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 415 Unsupported Media Type
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json http://127.0.0.1:<PORT>/api/v1/rpc/
HTTP/1.1 404 Not Found
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json http://127.0.0.1:<PORT>/api/v1/rpcx
HTTP/1.1 404 Not Found
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json http://127.0.0.1:<PORT>/api/v1/rpc?foo=bar
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== http://127.0.0.1:<PORT>/
HTTP/1.1 404 Not Found
Content-Length: 50
Content-Type: text/html
Connection: close

<h1>404 Not Found</h1>No context found for request

=== http://127.0.0.1:<PORT>/api/v1/
HTTP/1.1 404 Not Found
Content-Length: 50
Content-Type: text/html
Connection: close

<h1>404 Not Found</h1>No context found for request

=== http://127.0.0.1:<PORT>/api/v1/unknown
HTTP/1.1 404 Not Found
Content-Length: 50
Content-Type: text/html
Connection: close

<h1>404 Not Found</h1>No context found for request

=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json -H Host: evil.example http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 421 
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json -H Host: localhost:<PORT> http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json -H Host: localhost http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json -H Host: 127.0.0.1:9999 http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 421 
Content-length: 0


=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json -H Host: [::1]:<PORT> http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== -X POST --data-binary {"jsonrpc":"2.0","method":"version","id":1} -H Content-Type: application/json -H Host: LOCALHOST http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== -H Host: evil.example http://127.0.0.1:<PORT>/api/v1/check
HTTP/1.1 200 OK
Content-length: 0


=== -H Host: evil.example http://127.0.0.1:<PORT>/api/v1/events
HTTP/1.1 421 
Content-length: 0


=== -X POST http://127.0.0.1:<PORT>/api/v1/events
HTTP/1.1 405 Method Not Allowed
Content-length: 0


=== http://127.0.0.1:<PORT>/api/v1/events?account=%2B15550000000
HTTP/1.1 400 Bad Request
Content-length: 0


=== http://127.0.0.1:<PORT>/api/v1/events/x
HTTP/1.1 404 Not Found
Content-length: 0


```

## 4. HTTP: missing Host header, SSE stream bytes, empty account query

SSE stream captured with `curl -s -i -N -m 20 http://127.0.0.1:<PORT>/api/v1/events | od -c`. After the headers, nothing is sent until the first keep-alive (`:` + LF) roughly 15 s later; curl was stopped at 20 s.

```
=== no Host header
HTTP/1.1 400 Bad Request
Content-Length: 43
Content-Type: text/html
Connection: close

<h1>400 Bad Request</h1>Missing Host header

=== SSE GET /api/v1/events (20s)
0000000   H   T   T   P   /   1   .   1       2   0   0       O   K  \r
0000020  \n   D   a   t   e   :       F   r   i   ,       2   3       O
0000040   c   t       2   0   2   6       0   6   :   1   0   :   5   7
0000060       G   M   T  \r  \n   T   r   a   n   s   f   e   r   -   e
0000100   n   c   o   d   i   n   g   :       c   h   u   n   k   e   d
0000120  \r  \n   C   o   n   t   e   n   t   -   t   y   p   e   :    
0000140   t   e   x   t   /   e   v   e   n   t   -   s   t   r   e   a
0000160   m  \r  \n  \r  \n   :  \n
0000167
=== SSE account empty
HTTP/1.1 200 OK
Transfer-encoding: chunked
Content-type: text/event-stream

=== HEAD events
HTTP/1.1 405 Method Not Allowed

=== GET rpc with HEAD
```

## 5. TCP socket (`--tcp`): one connection, many lines

All of the following bytes were written on **one** TCP connection (bash `/dev/tcp`), then the
connection was read for 4 s. Input lines (each terminated by LF unless noted; line 3 is
empty, line 4 is three spaces; the second-to-last line ends with CR LF; the last request is
split across two lines):

```
{"jsonrpc":"2.0","method":"version","id":1}<LF>
not json<LF>
<LF>
   <LF>
[]<LF>
42<LF>
null<LF>
{}<LF>
{"jsonrpc":"2.0","method":"version"}<LF>
{"jsonrpc":"2.0","method":"nope","id":2}<LF>
[{"jsonrpc":"2.0","method":"version","id":3},{"jsonrpc":"2.0","method":"version"}]<LF>
[{"jsonrpc":"2.0","method":"version"}]<LF>
{"jsonrpc":"2.0","method":"version","id":4} {"jsonrpc":"2.0","method":"version","id":5}<LF>
{"jsonrpc":"2.0","result":{},"id":9}<LF>
{"jsonrpc":"2.0","method":"subscribeReceive","id":6}<LF>
{"jsonrpc":"2.0","method":"subscribeReceive","id":7}<LF>
{"jsonrpc":"2.0","method":"unsubscribeReceive","params":{"subscription":999},"id":8}<LF>
{"jsonrpc":"2.0","method":"unsubscribeReceive","id":10}<LF>
{"jsonrpc":"2.0","method":"unsubscribeReceive","params":[1],"id":11}<LF>
{"jsonrpc":"2.0","method":"unsubscribeReceive","params":{},"id":12}<LF>
{"jsonrpc":"2.0","method":"subscribeCallEvents","id":13}<LF>
{"jsonrpc":"2.0","method":"unsubscribeCallEvents","id":14}<LF>
{"jsonrpc":"2.0","method":"unsubscribeCallEvents","params":{"subscription":"abc"},"id":15}<LF>
{"jsonrpc":"2.0","method":"listAccounts","id":16}<LF>
{"jsonrpc":"2.0","method":"version","id":"crlf"}<CR><LF>
{"jsonrpc":"2.0",<LF>
"method":"version","id":"split"}<LF>
```

Output, exactly as received (order is the daemon's: requests are handled concurrently, so
responses are **not** in request order; parse/validation errors detected while reading come
out immediately):

```
{"jsonrpc":"2.0","error":{"code":-32700,"message":"Unrecognized token 'not': was expecting (JSON String, Number, Array, Object or token 'null', 'true' or 'false')\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 1, column: 1]","data":null},"id":null}
{"jsonrpc":"2.0","error":{"code":-32700,"message":"Empty input line","data":null},"id":null}
{"jsonrpc":"2.0","error":{"code":-32700,"message":"Empty input line","data":null},"id":null}
{"jsonrpc":"2.0","error":{"code":-32600,"message":"invalid request","data":null},"id":null}
{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: NUMBER","data":null},"id":null}
{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: NULL","data":null},"id":null}
{"jsonrpc":"2.0","error":{"code":-32600,"message":"only jsonrpc version 2.0 is supported","data":null},"id":null}
{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}
{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":4}
{"jsonrpc":"2.0","result":1,"id":6}
{"jsonrpc":"2.0","error":{"code":-1,"message":"Missing subscription parameter with subscription id","data":null},"id":10}
{"jsonrpc":"2.0","result":3,"id":13}
{"jsonrpc":"2.0","error":{"code":-1,"message":"Missing subscription parameter with subscription id","data":null},"id":14}
{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not implemented","data":null},"id":2}
{"jsonrpc":"2.0","error":{"code":-32700,"message":"Unexpected end-of-input within/between Object entries\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 1, column: 18]","data":null},"id":null}
[{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":3}]
{"jsonrpc":"2.0","error":{"code":-32600,"message":"unexpected type: STRING","data":null},"id":null}
{"jsonrpc":"2.0","result":2,"id":7}
{"jsonrpc":"2.0","error":{"code":-1,"message":"Unknown subscription id","data":null},"id":8}
{"jsonrpc":"2.0","error":{"code":-32603,"message":"Cannot invoke \"com.fasterxml.jackson.databind.JsonNode.asInt()\" because the return value of \"com.fasterxml.jackson.databind.node.ObjectNode.get(String)\" is null (NullPointerException)","data":null},"id":12}
{"jsonrpc":"2.0","result":{},"id":11}
{"jsonrpc":"2.0","result":[],"id":16}
{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":"crlf"}
{"jsonrpc":"2.0","error":{"code":-1,"message":"Unknown subscription id","data":null},"id":15}
```

Notes on this transcript:
- No response for the notification (`{"jsonrpc":"2.0","method":"version"}`), for the batch
  consisting only of a notification, nor for the line that was a JSON-RPC *response* object
  (`{"jsonrpc":"2.0","result":{},"id":9}`), which is silently ignored.
- The line with two JSON objects produced only the response for id 4; the trailing object
  (id 5) was ignored.
- Subscription ids: this connection's automatic (on-start) subscription consumed id 0, then
  `subscribeReceive` returned 1 and 2, `subscribeCallEvents` returned 3. `unsubscribeReceive`
  with `params:[1]` succeeded with result `{}`. `{"subscription":"abc"}` was read as 0, which is
  not a call-event subscription, hence `Unknown subscription id`.

## 6. UNIX socket (`--socket`)

Client: a short perl one-liner using IO::Socket::UNIX (no socat/nc in the sandbox), sending:

```
{"jsonrpc":"2.0","method":"version","id":1}<LF>
{"jsonrpc":"2.0","method":"subscribeReceive","id":2}<LF>
```

Response:

```
{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}
{"jsonrpc":"2.0","result":5,"id":2}
```

(id 5 because the subscription-id counter is process-wide: the earlier TCP connection used
0-3 and this connection's own automatic subscription took 4.) The daemon logged
`Accepted new client connection 0:  UnixDomainPrincipal[user=agent, group=agent]`.
Socket file mode as created: `srwxr-xr-x`.

## 7. TCP on a `--receive-mode manual` daemon

One connection, input:

```
{"jsonrpc":"2.0","method":"subscribeReceive","id":1}
{"jsonrpc":"2.0","method":"unsubscribeReceive","params":{"subscription":0},"id":2}
{"jsonrpc":"2.0","method":"subscribeReceive","params":{"account":"+15550000000"},"id":3}
{"jsonrpc":"2.0","method":"subscribeReceive","params":[],"id":4}
```

Output (as received):

```
{"jsonrpc":"2.0","error":{"code":-1,"message":"Unknown subscription id","data":null},"id":2}
{"jsonrpc":"2.0","error":{"code":-32602,"message":"Specified account does not exist","data":null},"id":3}
{"jsonrpc":"2.0","result":1,"id":4}
{"jsonrpc":"2.0","result":0,"id":1}
```

(In manual mode no automatic subscription is created, so the first explicit subscription is 0.
Response for id 2 raced ahead of id 1's subscription being created.)

HTTP on the wildcard-bound port with a foreign Host header:

```
=== -X POST -H 'Host: evil.example' -H 'Content-Type: application/json' --data-binary '{"jsonrpc":"2.0","method":"version","id":1}' http://127.0.0.1:<PORT>/api/v1/rpc
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}
```

## 8. stdio `jsonRpc` command

```
$ printf '%s\n' '{"jsonrpc":"2.0","method":"version","id":1}' 'bad' '{"jsonrpc":"2.0","method":"subscribeReceive","id":2}' '{"jsonrpc":"2.0","method":"listAccounts","id":3}' | signal-cli --config <CONFIG> jsonRpc
{"jsonrpc":"2.0","error":{"code":-32700,"message":"Unrecognized token 'bad': was expecting (JSON String, Number, Array, Object or token 'null', 'true' or 'false')\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: 1, column: 1]","data":null},"id":null}
{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}
{"jsonrpc":"2.0","result":1,"id":2}
{"jsonrpc":"2.0","result":[],"id":3}
exit=0

$ echo '{"jsonrpc":"2.0","method":"version","id":1}' | signal-cli --config <CONFIG> -o plain-text jsonRpc
Command doesn't support output type plain-text          (stderr)
exit=1
```

## 9. Daemon start-up errors (stderr, exit code)

```
$ signal-cli --config <CONFIG> daemon
INFO  DaemonCommand - Starting daemon in multi-account mode
At least one channel parameter is required, e.g. --socket or --dbus.
exit=1

$ signal-cli --config <CONFIG> daemon --tcp nocolon
INFO  DaemonCommand - Starting daemon in multi-account mode
Invalid tcp bind address (expected host:port): nocolon
exit=1

$ signal-cli --config <CONFIG> daemon --tcp 127.0.0.1:abc
INFO  DaemonCommand - Starting daemon in multi-account mode
Invalid tcp port: abc
exit=1

$ signal-cli --config <CONFIG> daemon --tcp 127.0.0.1:<PORT>        (port already bound)
INFO  DaemonCommand - Starting daemon in multi-account mode
Failed to bind socket /127.0.0.1:<PORT>: Address already in use
exit=3

$ signal-cli --config <CONFIG> daemon --http 127.0.0.1:<PORT>       (port already bound)
INFO  DaemonCommand - Starting daemon in multi-account mode
Failed to initialize HTTP Server
exit=3

$ signal-cli --config <CONFIG> daemon --socket <CONFIG>/../rpc-spec.sock   (path exists)
INFO  DaemonCommand - Starting daemon in multi-account mode
Failed to bind socket <CONFIG>/../rpc-spec.sock: Address already in use
exit=3

$ signal-cli --config <CONFIG> -a +15550000000 daemon --tcp 127.0.0.1:<PORT>
User +15550000000 is not registered.
exit=1

$ signal-cli --config <CONFIG> daemon --receive-mode foo --tcp 127.0.0.1:<PORT>
usage: signal-cli daemon [-h] [--dbus] [--dbus-system]
                  [--bus-name BUS_NAME] [--socket [SOCKET]] [--tcp [TCP]]
                  [--http [HTTP]] [--no-receive-stdout]
                  [--receive-mode {on-start,on-connection,manual}]
                  [--ignore-attachments] [--ignore-stories]
                  [--ignore-avatars] [--ignore-stickers]
                  [--send-read-receipts]
signal-cli:  error:  argument  --receive-mode:   could  not  convert  'foo'
(choose from {on-start,on-connection,manual})
exit=1
```

## 10. Fork C transcripts: `version` and `listAccounts` parameter shapes (HTTP)


```
=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","id":1}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":1}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":{},"id":2}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":2}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":null,"id":3}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":3}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":[],"id":4}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 261

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot deserialize value of type `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":4}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":{"account":"+15550000000"},"id":5}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 105

{"jsonrpc":"2.0","error":{"code":-32602,"message":"Specified account does not exist","data":null},"id":5}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":{"account":null},"id":6}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":6}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":{"foo":1},"id":7}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 54

{"jsonrpc":"2.0","result":{"version":"0.14.7"},"id":7}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":{"account":"8a5f0e4b-1b2c-4d3e-9f00-123456789abc"},"id":8}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 105

{"jsonrpc":"2.0","error":{"code":-32602,"message":"Specified account does not exist","data":null},"id":8}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"listAccounts","id":10}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 37

{"jsonrpc":"2.0","result":[],"id":10}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"listAccounts","params":[],"id":11}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 262

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot deserialize value of type `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":11}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"listAccounts","params":{"account":"+15550000000"},"id":12}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 37

{"jsonrpc":"2.0","result":[],"id":12}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"listAccounts","params":[1,2],"id":13}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 262

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Cannot deserialize value of type `java.util.LinkedHashMap<java.lang.String,java.lang.Object>` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN]","data":null},"id":13}

=== REQUEST: POST /api/v1/rpc  Content-Type: application/json
{"jsonrpc":"2.0","method":"version","params":"x","id":14}
--- RESPONSE
HTTP/1.1 200 OK
Content-type: application/json
Content-length: 578

{"jsonrpc":"2.0","error":{"code":-32600,"message":"Problem deserializing property 'params' (expected type: [simple type, class com.fasterxml.jackson.databind.node.ContainerNode<java.lang.Object>]; actual type: `com.fasterxml.jackson.databind.node.TextNode`), problem: Can not set com.fasterxml.jackson.databind.node.ContainerNode field org.asamk.signal.jsonrpc.JsonRpcRequest.params to com.fasterxml.jackson.databind.node.TextNode\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest[\"params\"])","data":null},"id":14}

```
