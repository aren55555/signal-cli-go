# signal-cli-go error codes

Generated from `internal/errcode` by `go test ./internal/errcode -run TestTable -update`; do not edit.

Every error the CLI or the JSON-RPC daemon reports is one of these codes. `exit` is the CLI exit status, `rpc` the JSON-RPC error code and `http` the HTTP status (blank when the error cannot occur there). Messages are the reference's exact text with Go `fmt` verbs for the variable parts; a JSON-RPC message is shown separately when it differs. `source` is `golden` when a recorded transcript shows the message, `spec` when it is known from `spec/` only, and `deviation` for signal-cli-go's own errors.

| code | kind | exit | rpc | http | message | cause | source |
|---|---|---|---|---|---|---|---|
| RPCEmptyLine | protocol |  | -32700 |  | `` Empty input line `` | a stream line was empty or only whitespace | golden |
| JSONUnrecognizedToken | protocol |  | -32700 |  | `` Unrecognized token '%s': was expecting (JSON String, Number, Array, Object or token 'null', 'true' or 'false')\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input started with a bare word that is not a JSON literal | golden |
| JSONNonStandardToken | protocol |  | -32700 |  | `` Non-standard token '%s': enable `JsonReadFeature.ALLOW_NON_NUMERIC_NUMBERS` to allow\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | NaN or Infinity used as a JSON number | spec |
| JSONExpectedValue | protocol |  | -32700 |  | `` Unexpected character (%s): expected a valid value (JSON String, Number, Array, Object or token 'null', 'true' or 'false')\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a character that cannot start a JSON value | spec |
| JSONExpectedFieldName | protocol |  | -32700 |  | `` Unexpected character (%s): was expecting double-quote to start field name\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | an object member name was not a quoted string | spec |
| JSONExpectedColon | protocol |  | -32700 |  | `` Unexpected character (%s): was expecting a colon to separate field name and value\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | an object member name was not followed by a colon | spec |
| JSONExpectedObjectComma | protocol |  | -32700 |  | `` Unexpected character (%s): was expecting comma to separate Object entries\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | object members were not separated by a comma | spec |
| JSONExpectedArrayComma | protocol |  | -32700 |  | `` Unexpected character (%s): was expecting comma to separate Array entries\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | array elements were not separated by a comma | spec |
| JSONEOFInObject | protocol |  | -32700 |  | `` Unexpected end-of-input within/between Object entries\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended inside an object | golden |
| JSONEOFInArray | protocol |  | -32700 |  | `` Unexpected end-of-input within/between Array entries\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended inside an array after a comma | spec |
| JSONEOFCloseMarker | protocol |  | -32700 |  | `` Unexpected end-of-input: expected close marker for %s (start marker at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d])\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended before an object or array was closed | spec |
| JSONEOFInString | protocol |  | -32700 |  | `` Unexpected end-of-input: was expecting closing quote for a string value\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended inside a string | spec |
| JSONEOFInFieldName | protocol |  | -32700 |  | `` Unexpected end-of-input in field name\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended inside an object member name | spec |
| JSONEOFInEscape | protocol |  | -32700 |  | `` Unexpected end-of-input in character escape sequence\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended inside a backslash escape | spec |
| JSONMismatchedClose | protocol |  | -32700 |  | `` Unexpected close marker '%s': expected '%s' (for %s starting at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d])\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | an object was closed with ] or an array with } | spec |
| JSONUnmatchedClose | protocol |  | -32700 |  | `` Unexpected close marker '%s': no open %s to close\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input started with a closing bracket | spec |
| JSONLeadingZeroes | protocol |  | -32700 |  | `` Invalid numeric value: Leading zeroes not allowed\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a number had a leading zero | spec |
| JSONNoDigitAfterSign | protocol |  | -32700 |  | `` Unexpected end-of-inputNo digit following sign\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended right after a minus sign | spec |
| JSONMinusNotDigit | protocol |  | -32700 |  | `` Unexpected character (%s) in numeric value: expected digit (0-9) to follow minus sign, for valid numeric value\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a minus sign was not followed by a digit | spec |
| JSONDecimalPoint | protocol |  | -32700 |  | `` Unexpected character (%s) in numeric value: Decimal point not followed by a digit\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a decimal point was not followed by a digit | spec |
| JSONExponentEOF | protocol |  | -32700 |  | `` Unexpected end-of-inputexpected a digit for number exponent\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended inside a number's exponent | spec |
| JSONExponentNotDigit | protocol |  | -32700 |  | `` Unexpected character (%s) in numeric value: Exponent indicator not followed by a digit\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | an exponent marker was not followed by a digit | spec |
| JSONBadEscape | protocol |  | -32700 |  | `` Unrecognized character escape %s\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a backslash escape that JSON does not define | spec |
| JSONBadUnicodeEscape | protocol |  | -32700 |  | `` Unexpected character (%s): expected a hex-digit for character escape sequence\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a \u escape with a non-hex digit | spec |
| JSONControlChar | protocol |  | -32700 |  | `` Illegal unquoted character (%s): has to be escaped using backslash to be included in %s\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a raw control character inside a string | spec |
| JSONIllegalWhitespace | protocol |  | -32700 |  | `` Illegal character (%s): only regular white space (\r, \n, \t) is allowed between tokens\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a control character between tokens | spec |
| JSONComment | protocol |  | -32700 |  | `` Unexpected character ('/' (code 47)): maybe a (non-standard) comment? (not recognized as one since Feature 'ALLOW_COMMENTS' not enabled for parser)\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a slash between tokens | spec |
| JSONPlusSign | protocol |  | -32700 |  | `` Unexpected character ('+' (code 43)) in numeric value: JSON spec does not allow numbers to have plus signs: enable `JsonReadFeature.ALLOW_LEADING_PLUS_SIGN_FOR_NUMBERS` to allow\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a number with a leading plus sign | spec |
| JSONRootSpace | protocol |  | -32700 |  | `` Unexpected character (%s): Expected space separating root-level values\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | a top-level number directly followed by another character | spec |
| JSONEOFInNumber | protocol |  | -32700 |  | `` Unexpected end-of-input in a Number value\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d] `` | input ended inside a number | spec |
| JSONNestingDepth | crash | 2 | -32603 |  | `` java.lang.AssertionError: com.fasterxml.jackson.core.exc.StreamConstraintsException: Document nesting depth (%d) exceeds the maximum allowed (%d, from `StreamReadConstraints.getMaxNestingDepth()`) ``<br>rpc: `` Document nesting depth (%d) exceeds the maximum allowed (%d, from `StreamReadConstraints.getMaxNestingDepth()`) (StreamConstraintsException) `` | input nested deeper than 1000 levels; the reference's reader thread dies | spec |
| RPCInvalidRequest | protocol |  | -32600 |  | `` invalid request `` | the request was an empty batch | golden |
| RPCUnexpectedType | protocol |  | -32600 |  | `` unexpected type: %s `` | the request was a JSON scalar (or nothing) instead of an object or array | golden |
| RPCVersion | protocol |  | -32600 |  | `` only jsonrpc version 2.0 is supported `` | jsonrpc member missing or not "2.0" | golden |
| RPCMethodMissing | protocol |  | -32600 |  | `` method field must be set `` | method member missing or null | golden |
| RPCUnrecognizedField | protocol |  | -32600 |  | `` Unrecognized field "%s" (class %s), not marked as ignorable (%d known properties: %s)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: %s["%s"]) `` | the request object, or a typed-params method's params, had an unknown member | golden |
| RPCPropertyType | protocol |  | -32600 |  | `` Problem deserializing property '%s' (expected type: [simple type, class %s]; actual type: `%s`), problem: Can not set %s field org.asamk.signal.jsonrpc.JsonRpcRequest.%s to %s\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.jsonrpc.JsonRpcRequest["%s"]) `` | id was an array or object, or params was a scalar | golden |
| RPCBatchElementScalar | protocol |  | -32600 |  | `` Cannot construct instance of `org.asamk.signal.jsonrpc.JsonRpcRequest` (although at least one Creator exists): no %s-argument constructor/factory method to deserialize from %s value (%s)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] `` | a batch element was a string, number or boolean | golden |
| RPCDeserializeArray | protocol |  | -32600 |  | `` Cannot deserialize value of type `%s` from Array value (token `JsonToken.START_ARRAY`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] `` | an array where an object was expected: params of a method that takes an object, or a batch element | golden |
| RPCDeserializeMember | protocol |  | -32600 |  | `` Cannot deserialize value of type `%s` from %s value (token `JsonToken.%s`)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: %s["%s"]) `` | a request or typed-params member had the wrong JSON type | spec |
| RPCDeserializeString | protocol |  | -32600 |  | `` Cannot deserialize value of type `%s` from String %s: %s\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: %s["%s"]) `` | a typed-params member was a string that does not convert to its type | spec |
| RPCNumberRange | protocol |  | -32600 |  | `` Numeric value (%s) out of range of int (-2147483648 - 2147483647)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: %s["%s"]) `` | a typed-params integer member was outside the int range | spec |
| RPCMethodNotImplemented | protocol |  | -32601 |  | `` Method not implemented `` | no such JSON-RPC method in this mode or over this transport | golden |
| RPCAccountRequired | protocol |  | -32602 |  | `` Method requires valid account parameter `` | no account given and the daemon does not have exactly one | golden |
| RPCAccountNotFound | protocol |  | -32602 |  | `` Specified account does not exist `` | the account parameter names no loaded account | golden |
| RPCAccountParamMissing | protocol |  | -32602 |  | `` Method requires account parameter `` | a registration method was called without an account key | golden |
| RPCAccountLoadFailed | protocol |  | -32602 |  | `` Failed to load the given account `` | the registration method's account could not be opened or locked | spec |
| RPCInternal | protocol |  | -32603 |  | `` An internal server error has occurred. `` | an exception escaped HTTP request processing | spec |
| RPCClassCast | protocol |  | -32603 |  | `` class %s cannot be cast to class %s (%s and %s are in module java.base of loader 'bootstrap') (ClassCastException) `` | a parameter had the wrong JSON type | spec |
| RPCSubscriptionUnknown | protocol |  | -1 |  | `` Unknown subscription id `` | no subscription with that id on this connection | golden |
| RPCSubscriptionMissing | protocol |  | -1 |  | `` Missing subscription parameter with subscription id `` | unsubscribe without params in multi-account mode | golden |
| RPCNullPointer | protocol |  | -32603 |  | `` Cannot invoke "%s" because %s is null (NullPointerException) `` | a required parameter was missing (a Java NullPointerException in the reference) | golden |
| RPCIllegalArgument | protocol |  | -32603 |  | `` %s (IllegalArgumentException) `` | a parameter value was outside its allowed set | spec |
| RPCNullPointerPlain | protocol |  | -32603 |  | `` null (NullPointerException) `` | a required parameter was missing (a NullPointerException without a message) | spec |
| RPCIndexOutOfBounds | protocol |  | -32603 |  | `` Index %d out of bounds for length %d (StringIndexOutOfBoundsException) `` | an odd-length hex value was decoded | spec |
| HTTPMissingHost | http |  |  | 400 | `` <h1>400 Bad Request</h1>Missing Host header `` | an HTTP/1.1 request without a Host header | golden |
| HTTPNoContext | http |  |  | 404 | `` <h1>404 Not Found</h1>No context found for request `` | the path is outside /api/v1/rpc, /api/v1/events and /api/v1/check | golden |
| HTTPNotFound | http |  |  | 404 | ``  `` | the path extends one of the endpoint paths | golden |
| HTTPMethodNotAllowed | http |  |  | 405 | ``  `` | wrong HTTP method for the endpoint | golden |
| HTTPUnsupportedMediaType | http |  |  | 415 | ``  `` | Content-Type missing or not application/json | golden |
| HTTPMisdirected | http |  |  | 421 | ``  `` | the Host header names neither the bind address nor loopback | golden |
| HTTPUnknownAccount | http |  |  | 400 | ``  `` | the events stream's account parameter names no loaded account | golden |
| HTTPServerError | http |  |  | 500 | ``  `` | an unexpected failure before the events stream started | spec |
| ArgUnrecognized | argparse | 1 |  |  | `` unrecognized arguments: '%s' `` | an argument the command does not take | golden |
| ArgTooFew | argparse | 1 |  |  | `` too few arguments `` | a required positional argument is missing | golden |
| ArgRequired | argparse | 1 |  |  | `` argument %s is required `` | a required option is missing | golden |
| ArgOneOfRequired | argparse | 1 |  |  | `` one of the arguments %s is required `` | none of a required group's options was given | golden |
| ArgNotAllowedWith | argparse | 1 |  |  | `` argument %s: not allowed with argument %s `` | two mutually exclusive options were given | golden |
| ArgExpectedOne | argparse | 1 |  |  | `` argument %s: expected one argument `` | an option that takes a value was given none | golden |
| ArgExpectedN | argparse | 1 |  |  | `` argument %s: expected %d argument(s) `` | a list option was given too few values | golden |
| ArgNotInteger | argparse | 1 |  |  | `` argument %s: could not convert '%s' to integer (%d bits) `` | an integer option got a non-integer or out-of-range value | golden |
| ArgNotDouble | argparse | 1 |  |  | `` argument %s: could not convert '%s' to number (double precision) `` | a numeric option got a non-number | golden |
| ArgConvertChoice | argparse | 1 |  |  | `` argument %s: could not convert '%s' (choose from %s) `` | an enum option got a value outside its choices | golden |
| ArgInvalidChoice | argparse | 1 |  |  | `` argument %s: invalid choice: '%s' (choose from %s) `` | a choice option got a value outside its choices | golden |
| ArgAmbiguousOption | argparse | 1 |  |  | `` ambiguous option: %s could match %s `` | an abbreviated option matches several options | golden |
| ArgAmbiguousCommand | argparse | 1 |  |  | `` ambiguous command: %s could match %s `` | an abbreviated command matches several commands | spec |
| ArgInvalidCommand | argparse | 1 |  |  | `` invalid choice: '%s' (choose from %s) `` | no such command | golden |
| ArgIgnoredValue | argparse | 1 |  |  | `` argument %s: ignore implicit argument '%s' `` | a flag was given a value with = | spec |
| ConfigLoad | startup | 1 |  |  | `` Failed to load config from %s: %s `` | a configuration file is not valid JSON | golden |
| ConfigUnknownField | startup | 1 |  |  | `` Failed to parse configuration file(s): Unrecognized field "%s" (class org.asamk.signal.GlobalConfig), not marked as ignorable (%d known properties: %s)\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.GlobalConfig["%s"]) `` | a configuration file has an unknown key | golden |
| ConfigInvalidValue | startup | 1 |  |  | `` Failed to parse configuration file(s): Cannot construct instance of `%s`, problem: %s\n at [Source: UNKNOWN; byte offset: #UNKNOWN] (through reference chain: org.asamk.signal.GlobalConfig["%s"]) `` | a configuration value has the wrong type or an invalid value | golden |
| OutputTypeUnsupported | startup | 1 |  |  | `` Command doesn't support output type %s `` | the command has only plain-text or only JSON output | golden |
| MultiAccountOnly | startup | 1 |  |  | `` Command only works in multi-account mode `` | a multi-account command was given -a | golden |
| DBusUnavailable | startup | 2 |  |  | `` Dbus client failed: Cannot Resolve Session Bus Address: %s `` | --dbus or --dbus-system without a reachable bus | golden |
| LinkWithAccount | startup | 1 |  |  | `` You cannot specify a account (phone number) when linking `` | link was given -a | golden |
| LiveUnsupported | startup | 1 |  |  | `` The live service environment is not supported by signal-cli-go yet; use --service-environment staging `` | only the staging environment is configured | deviation |
| InvalidProxy | startup | 1 |  |  | `` Invalid proxy setting: %s `` | the proxy environment variable could not be parsed | deviation |
| NotImplemented | unexpected | 2 | -32603 |  | `` Command %s is not implemented in signal-cli-go yet `` | the command has no implementation | deviation |
| DaemonNoChannel | startup | 1 |  |  | `` At least one channel parameter is required, e.g. --socket or --dbus. `` | daemon was started without a transport | spec |
| DaemonBadAddress | startup | 1 |  |  | `` Invalid tcp bind address (expected host:port): %s `` | a bind address without a colon | spec |
| DaemonBadPort | startup | 1 |  |  | `` Invalid tcp port: %s `` | a bind address whose port is not an integer | spec |
| DaemonBadHost | startup | 1 |  |  | `` Invalid tcp bind address, invalid host: %s `` | a bind address whose host does not resolve | spec |
| DaemonBindFailed | startup | 3 |  |  | `` Failed to bind socket %s: %s `` | the TCP port or socket path could not be bound | spec |
| DaemonHTTPInit | startup | 3 |  |  | `` Failed to initialize HTTP Server `` | the HTTP port could not be bound | spec |
| AccountsFileLoad | io | 3 | -3 |  | `` Failed to load local accounts file `` | the accounts.json index could not be read | spec |
| AccountsListRead | io | 3 | -3 |  | `` Failed to read local accounts list `` | the data directory could not be created or read | golden |
| NoLocalUsers | user | 1 | -1 |  | `` No local users found, you first need to register or link an account `` | no -a and no local accounts | golden |
| MultipleUsers | user | 1 | -1 |  | `` Multiple users found, you need to specify an account (phone number) with -a `` | no -a and several local accounts | golden |
| InvalidAccountNumber | user | 1 | -1 |  | `` Invalid account (phone number), make sure you include the country code. `` | -a is neither an E.164 number nor a UUID | golden |
| UserNotRegistered | user | 1 | -1 |  | `` User %s is not registered. `` | the account does not exist locally or is not registered on the server | golden |
| StateFileLocked | unexpected | 2 | -32603 |  | `` Error loading state file for user %s: %s (OverlappingFileLockException) ``<br>rpc: `` Error loading state file for user %s: %s (OverlappingFileLockException) (UnexpectedErrorException) `` | another process holds the account's lock | spec |
| StateFileIO | unexpected | 2 | -32603 |  | `` Error loading state file for user %s: %s (IOException) ``<br>rpc: `` Error loading state file for user %s: %s (IOException) (UnexpectedErrorException) `` | the account file could not be read | spec |
| AccountCheckFailed | io | 3 | -3 |  | `` Error while checking account %s: %s `` | the start-up account check against the server failed | spec |
| NotPrimaryDevice | user | 1 | -1 |  | `` This command doesn't work on linked devices. `` | the command needs the primary device | spec |
| InvalidPhoneNumber | user | 1 | -1 |  | `` Invalid phone number '%s': %s `` | a recipient could not be parsed as a number, UUID, PNI or username | spec |
| RecipientNotRegistered | user | 1 | -1 |  | `` The user %s is not registered. `` | a recipient is not a Signal user | spec |
| NoRecipients | user | 1 | -1 |  | `` No recipients given `` | no recipient, group or note-to-self target | spec |
| NoRecipient | user | 1 | -1 |  | `` No recipient given `` | the command needs exactly one recipient | spec |
| InvalidGroupID | user | 1 | -1 |  | `` Invalid group id: %s `` | a group id is not base64 of 16 or 32 bytes | spec |
| GroupNotFound | user | 1 | -1 |  | `` Group not found: %s `` | no local group with that id | spec |
| GroupNotMember | user | 1 | -1 |  | `` User is not a member in group: %s (%s) `` | the account is not a member of the group | spec |
| GroupSendNotAllowed | user | 1 | -1 |  | `` User is not allowed to send message to group: %s (%s) `` | the group only lets admins send | spec |
| GroupSendFailed | user | 1 | -1 |  | `` Failed to send to group: %s `` | sending to a group failed on a group problem | spec |
| GroupLastAdmin | user | 1 | -1 |  | `` You need to specify a new admin with --admin: User is last admin in group: %s (%s) `` | the last admin tried to leave without naming a successor | spec |
| SendFailed | user | 1 | -1 |  | `` Failed to send message `` | every recipient failed, for mixed reasons | spec |
| SendUntrusted | untrusted | 4 | -4 |  | `` Failed to send message due to untrusted identities `` | every recipient failed on an untrusted identity | spec |
| SendRateLimited | rate-limit | 5 | -5 |  | `` Failed to send message due to rate limiting `` | every recipient failed on a rate limit | spec |
| SendIOFailedUser | user | 1 | -1 |  | `` Failed to send message: %s (IOException) `` | network failure while sending to individual recipients | spec |
| SendIOFailed | unexpected | 2 | -32603 |  | `` Failed to send message: %s (IOException) ``<br>rpc: `` Failed to send message: %s (IOException) (UnexpectedErrorException) `` | network failure while sending | spec |
| SendIOFailedIO | io | 3 | -3 |  | `` Failed to send message: %s (%s) `` | network failure while sending a group change | spec |
| SendAttachmentInvalid | unexpected | 2 | -32603 |  | `` Failed to send message: %s (AttachmentInvalidException) ``<br>rpc: `` Failed to send message: %s (AttachmentInvalidException) (UnexpectedErrorException) `` | an attachment file could not be read | spec |
| SendGroupCreateFailed | unexpected | 2 | -32603 |  | `` Failed to send message: Failed to create group (IOException) ``<br>rpc: `` Failed to send message: Failed to create group (IOException) (UnexpectedErrorException) `` | the server refused to create the group | spec |
| SendGroupError | unexpected | 2 | -32603 |  | `` Failed to send message: %s (%s) ``<br>rpc: `` Failed to send message: %s (%s) (UnexpectedErrorException) `` | a group operation failed | spec |
| EmptyMessage | user | 1 | -1 |  | `` Sending empty message is not allowed, either a message, attachment or sticker must be given. `` | nothing to send | spec |
| StdinReadFailed | user | 1 | -1 |  | `` Failed to read message from stdin: %s `` | the message could not be read from standard input | spec |
| InvalidMention | user | 1 | -1 |  | `` Invalid mention syntax (%s) expected 'start:length:recipientNumber' `` | a mention is not start:length:recipient | spec |
| InvalidTextStyle | user | 1 | -1 |  | `` Invalid textStyle syntax (%s) expected 'start:length:STYLE' `` | a text style is not start:length:STYLE | spec |
| InvalidStyle | user | 1 | -1 |  | `` Invalid style: %s `` | an unknown text style name | spec |
| InvalidSticker | user | 1 | -1 |  | `` Invalid sticker syntax (%s) expected 'stickerPackId:stickerId' `` | a sticker is not packId:stickerId | spec |
| StickerSendFailed | user | 1 | -1 |  | `` Failed to send sticker: %s `` | the sticker is not installed or invalid | spec |
| QuoteAuthorMissing | user | 1 | -1 |  | `` Quote author parameter is missing `` | a quote timestamp without a quote author | spec |
| InvalidQuoteAttachment | user | 1 | -1 |  | `` Invalid attachment syntax (%s) expected 'contentType[:filename[:previewFile]]' `` | a quote attachment is malformed | spec |
| NumberParseFailed | unexpected | 2 | -32603 |  | `` Failed to parse number: %s ``<br>rpc: `` Failed to parse number: %s (UnexpectedErrorException) `` | a numeric field of a structured argument is not a number | spec |
| PollTooFewOptions | user | 1 | -1 |  | `` Poll needs at least two options `` | fewer than two poll options | spec |
| PollTooManyOptions | user | 1 | -1 |  | `` Poll cannot have more than 10 options `` | more than ten poll options | spec |
| PollEmptyOption | user | 1 | -1 |  | `` Poll options must not be empty `` | an empty poll option | spec |
| PollOptionTooLong | user | 1 | -1 |  | `` Poll option "%s" exceeds the maximum length of 100 characters `` | a poll option longer than 100 characters | spec |
| PollAuthorRequired | unexpected | 2 | -32603 |  | `` Poll author is required ``<br>rpc: `` Poll author is required (UnexpectedErrorException) `` | a poll vote without the poll's author | spec |
| AdminDeleteNoGroups | user | 1 | -1 |  | `` Admin delete requires group IDs `` | admin delete without a group | spec |
| ReceiptBase64 | unexpected | 2 | -32603 |  | `` Illegal base64 receipt: %s ``<br>rpc: `` Illegal base64 receipt: %s (UnexpectedErrorException) `` | a payment receipt is not base64 | spec |
| PaymentNoteRequired | unexpected | 2 | -32603 |  | `` Payment note is required ``<br>rpc: `` Payment note is required (UnexpectedErrorException) `` | a payment notification without a note | spec |
| ReceiptTypeUnknown | user | 1 | -1 |  | `` Unknown receipt type: %s `` | a receipt type other than read or viewed | spec |
| MessageRequestFailed | io | 3 | -3 |  | `` Failed to send message request response: %s `` | the message request response could not be sent | spec |
| StoryAttachmentRequired | user | 1 | -1 |  | `` An attachment is required for sending a story. `` | a story without an attachment | spec |
| StoryAttachmentInvalid | unexpected | 2 | -32603 |  | `` Failed to send story: %s: %s (AttachmentInvalidException) ``<br>rpc: `` Failed to send story: %s: %s (AttachmentInvalidException) (UnexpectedErrorException) `` | the story attachment could not be read | spec |
| StorySendFailed | unexpected | 2 | -32603 |  | `` Failed to send story: %s (IOException) ``<br>rpc: `` Failed to send story: %s (IOException) (UnexpectedErrorException) `` | network failure while sending a story | spec |
| BlockSyncFailed | unexpected | 2 | -32603 |  | `` Failed to sync block to linked devices: %s ``<br>rpc: `` Failed to sync block to linked devices: %s (UnexpectedErrorException) `` | the blocked-list sync message could not be sent | spec |
| ReceiveFailed | io | 3 | -3 |  | `` Error while receiving messages: %s `` | network failure while receiving | spec |
| ReceiveBusy | user | 1 | -1 |  | `` Receive command cannot be used if messages are already being received. `` | the daemon is already receiving for this account | spec |
| TrustNeedsSafetyNumber | user | 1 | -1 |  | `` You need to specify the fingerprint/safety number you have verified with -v SAFETY_NUMBER `` | trust without -a or -v | spec |
| SafetyNumberFormat | user | 1 | -1 |  | `` Safety number has invalid format, either specify the old hex fingerprint or the new safety number `` | the safety number is neither hex, 60 digits nor a scannable code | spec |
| TrustAllFailed | user | 1 | -1 |  | `` Failed to set the trust for this number, make sure the number is correct. `` | no identity key is known for the recipient | spec |
| TrustVerifiedFailed | user | 1 | -1 |  | `` Failed to set the trust for this number, make sure the number and the fingerprint/safety number are correct. `` | the safety number does not match the recipient's key | spec |
| CaptchaRequired | user | 1 | -1 |  | `` Captcha required for verification, use --captcha CAPTCHA\nTo get the token, go to https://signalcaptchas.org/registration/generate.html\nAfter solving the captcha, right-click on the "Open Signal" link and copy the link.%s `` | the server requires a captcha before sending a code | spec |
| CaptchaInvalid | user | 1 | -1 |  | `` Invalid captcha given.%s `` | the server rejected the captcha | spec |
| RegisterRateLimited | rate-limit | 5 | -5 |  | `` Rate limit reached%s `` | the server rate-limited the request | spec |
| RegisterMethodUnavailable | user | 1 | -1 |  | `` Failed to register: Invalid verification method%s `` | the server cannot send a code by the chosen method | spec |
| RegisterFailed | io | 3 | -3 |  | `` Failed to register: %s (%s) `` | the registration request failed | spec |
| RegisterAlreadyRegistered | io | 3 | -3 |  | `` Failed to register: Account is already registered (IOException) `` | register without --reregister on a registered account | spec |
| VerifyNoSession | io | 3 | -3 |  | `` Verify error: No registration verification session active `` | verify without a prior register | spec |
| VerifyFailed | io | 3 | -3 |  | `` Verify error: %s `` | the verification code was rejected or the request failed | spec |
| VerifyRegLockFailed | crash | 2 | -32603 |  | `` Exception in thread "main" java.lang.AssertionError: KBS Pin appeared to matched but reg lock still failed! ``<br>rpc: `` KBS Pin appeared to matched but reg lock still failed! (AssertionError) `` | the restored PIN's registration lock token was refused | spec |
| PinLocked | user | 1 | -1 |  | `` Verification failed! This number is locked with a pin. Hours remaining until reset: %d\nUse '--pin PIN_CODE' to specify the registration lock PIN `` | the number has a registration lock and no PIN was given | spec |
| PinMismatch | user | 1 | -1 |  | `` Verification failed! Invalid pin, tries remaining: %d `` | the PIN does not match the SVR backup | spec |
| PinDataDeleted | user | 1 | -1 |  | `` Account is pin locked, but pin data has been deleted on the server. `` | the number is locked but the SVR backup is gone | spec |
| PinUnsupported | user | 1 | -1 |  | `` PIN contains characters signal-cli-go cannot normalize; use digits or ASCII `` | the PIN needs Unicode normalization signal-cli-go does not implement | deviation |
| SetPinFailed | io | 3 | -3 |  | `` Set pin error: %s `` | the PIN backup to SVR failed | spec |
| RemovePinFailed | io | 3 | -3 |  | `` Remove pin error: %s `` | deleting the SVR backup failed | spec |
| LinkUserExists | user | 1 | -1 |  | `` User already exists `` | the linked account already exists locally | spec |
| LinkRequestFailed | io | 3 | -3 |  | `` Link request error: %s `` | the provisioning exchange failed | spec |
| FinishLinkUserExists | user | 1 | -1 |  | `` The user %s already exists\nDelete "%s" before trying again. `` | finishLink for a number that already has a local account | spec |
| InvalidDeviceLinkURI | user | 1 | -1 |  | `` Invalid device link uri. `` | finishLink's deviceLinkUri is not a valid URI | spec |
| LinkTimedOut | user | 1 | -1 |  | `` Link request timed out, please try again. `` | the primary device did not answer in time | spec |
| DeviceLinkTimedOut | user | 1 | -1 |  | `` Device link creation timed out, please try again. `` | no provisioning address arrived in time | spec |
| UnknownDeviceLinkURI | user | 1 | -1 |  | `` Unknown device link uri. `` | finishLink for a URI no startLink produced | spec |
| MissingDeviceLinkURI | user | 1 | -1 |  | `` Missing deviceLinkUri. `` | finishLink without deviceLinkUri | spec |
| NotDeletingRegistered | user | 1 | -1 |  | `` Not deleting account, it is still registered. Use --ignore-registered to delete it anyway. `` | deleteLocalAccountData on a registered account | spec |
| DeletionFailed | io | 3 | -3 |  | `` Deletion error: %s `` | the account files could not be deleted | spec |
| UpdateAccountFailed | io | 3 | -3 |  | `` UpdateAccount error: %s `` | the account attribute update failed | spec |
| UsernameInvalid | user | 1 | -1 |  | `` Invalid username: %s (%s) `` | the nickname is not a valid username | spec |
| UsernameTaken | user | 1 | -1 |  | `` Invalid username: Username is malformed or already taken `` | the server refused to reserve the username | spec |
| UsernameSetFailed | io | 3 | -3 |  | `` Failed to set username: %s `` | network failure while setting the username | spec |
| UsernameDeleteFailed | io | 3 | -3 |  | `` Failed to delete username: %s `` | network failure while deleting the username | spec |
| UsernameLookupCrash | crash | 2 | -32603 |  | `` Exception in thread "main" java.lang.AssertionError: org.signal.libsignal.usernames.%s: %s ``<br>rpc: `` org.signal.libsignal.usernames.%s: %s (AssertionError) `` | a looked-up username is not valid | spec |
| UserStatusRateLimited | rate-limit | 5 | -5 |  | `` Rate limit reached\nNext attempt may be tried at %s `` | the contact discovery service rate-limited the lookup | spec |
| UserStatusFailed | io | 3 | -3 |  | `` Unable to check if users are registered: %s (%s) `` | the number or username lookup failed | spec |
| UpdateProfileFailed | io | 3 | -3 |  | `` Update profile error: %s `` | the profile upload failed or the avatar could not be read | spec |
| ProfileInputTooLong | crash | 2 | -32603 |  | `` Exception in thread "main" java.lang.AssertionError: org.signal.libsignal.zkgroup.InvalidInputException: Input is too long ``<br>rpc: `` org.signal.libsignal.zkgroup.InvalidInputException: Input is too long (AssertionError) `` | a profile field exceeds the largest padding bucket | spec |
| UpdateContactFailed | io | 3 | -3 |  | `` Update contact error: %s `` | sending the contact update failed | spec |
| DeleteSelfRecipient | crash | 2 | -32603 |  | `` Exception in thread "main" java.lang.RuntimeException: Can't delete self recipient ``<br>rpc: `` Can't delete self recipient (RuntimeException) `` | removeContact --forget on the account itself | spec |
| AvatarNotFound | user | 1 | -1 |  | `` Could not find avatar `` | no avatar is stored | spec |
| AvatarReadFailed | unexpected | 2 | -32603 |  | `` An error occurred reading avatar ``<br>rpc: `` An error occurred reading avatar (UnexpectedErrorException) `` | the avatar file could not be read | spec |
| ListDevicesFailed | io | 3 | -3 |  | `` Failed to get linked devices: %s `` | the device list request failed | spec |
| InvalidDeviceLink | user | 1 | -1 |  | `` Invalid device link `` | the device link URI lacks a valid uuid or key | spec |
| TooManyDevices | user | 1 | -1 |  | `` Account has too many linked devices already `` | the server's device limit was reached | spec |
| AddDeviceFailed | io | 3 | -3 |  | `` Add device link failed `` | provisioning the new device failed | spec |
| RemoveDeviceFailed | io | 3 | -3 |  | `` Error while removing device: %s `` | the device removal request failed | spec |
| UpdateDeviceFailed | io | 3 | -3 |  | `` Error while updating device: %s `` | the device rename request failed | spec |
| UnregisterFailed | io | 3 | -3 |  | `` Unregister error: %s `` | the unregister request failed | spec |
| GroupLinkNotALink | user | 1 | -1 |  | `` Link is not a signal group invitation link `` | the URI is not a signal.group link | spec |
| GroupLinkIncompatible | user | 1 | -1 |  | `` Group link was created with an incompatible version: %s `` | the link has no known content version | spec |
| GroupLinkInvalid | user | 1 | -1 |  | `` Group link is invalid: %s `` | the link's fragment could not be decoded | spec |
| GroupLinkNotValid | user | 1 | -1 |  | `` Group link is not valid: %s `` | the server says the link is inactive | spec |
| GroupPendingApproval | user | 1 | -1 |  | `` Pending admin approval: %s `` | a join request is already pending | spec |
| InvalidGroupLinkState | user | 1 | -1 |  | `` Invalid group link state: %s `` | an unknown group link mode | spec |
| InvalidGroupPermission | user | 1 | -1 |  | `` Invalid group permission: %s `` | an unknown group permission value | spec |
| GroupAvatarInvalid | user | 1 | -1 |  | `` Failed to add avatar attachment for group": %s `` | the group avatar file could not be read | spec |
| AttachmentNotFound | user | 1 | -1 |  | `` Could not find attachment with ID: %s `` | no stored attachment with that id | spec |
| AttachmentReadFailed | unexpected | 2 | -32603 |  | `` An error occurred reading attachment: %s ``<br>rpc: `` An error occurred reading attachment: %s (UnexpectedErrorException) `` | the attachment file could not be read | spec |
| MissingAttachmentID | user | 1 | -1 |  | `` Missing attachment id parameter `` | getAttachment without id | spec |
| StickerURIInvalid | user | 1 | -1 |  | `` Sticker pack uri has invalid format: %s `` | the sticker pack URI is not a valid URI | spec |
| StickerPackLinkInvalid | user | 1 | -1 |  | `` Invalid sticker pack link `` | the sticker pack link lacks a valid pack id or key | spec |
| StickerInstallFailed | io | 3 | -3 |  | `` Install sticker pack failed `` | downloading the sticker pack failed | spec |
| StickerNotFound | user | 1 | -1 |  | `` Could not find sticker with ID: %d in pack StickerPackId{%s} `` | the sticker is not stored locally | spec |
| StickerReadFailed | unexpected | 2 | -32603 |  | `` An error occurred reading sticker with ID: %d in pack StickerPackId{%s} ``<br>rpc: `` An error occurred reading sticker with ID: %d in pack StickerPackId{%s} (UnexpectedErrorException) `` | the sticker file could not be read | spec |
| StickerPackInvalid | user | 1 | -1 |  | `` Invalid sticker pack: %s `` | the sticker pack manifest or files are invalid | spec |
| StickerUploadFailed | io | 3 | -3 |  | `` Upload error (maybe image size too large):%s `` | uploading the sticker pack failed | spec |
| SyncRequestFailed | io | 3 | -3 |  | `` Request sync data error: %s `` | the sync request could not be sent | spec |
| SendContactsFailed | io | 3 | -3 |  | `` SendContacts error: %s `` | the contacts sync could not be sent | spec |
| NoCallID | user | 1 | -1 |  | `` No call ID given `` | a call method without callId | spec |
| CallNotFound | io | 3 | -3 |  | `` Failed to accept call: No active call with id %s `` | no active call with that id | spec |
| CallNotRinging | io | 3 | -3 |  | `` Failed to accept call: Call %d is not in RINGING_INCOMING state (current: %s) `` | the call is not ringing | spec |
| HangupCallNotFound | io | 3 | -3 |  | `` Failed to hang up call: No active call with id %s `` | no active call with that id | spec |
| RejectCallNotFound | io | 3 | -3 |  | `` Failed to reject call: No active call with id %s `` | no active call with that id | spec |
| StartCallFailed | io | 3 | -3 |  | `` Failed to start call: %s `` | the call could not be started | spec |
| ChangeNumberInvalid | user | 1 | -1 |  | `` Failed to change number: %s `` | the new number is not a normalized E.164 number | spec |
| ChangeNumberFailed | io | 3 | -3 |  | `` Failed to change number: %s (%s) `` | the change-number request failed | spec |
| SubmitChallengeFailed | io | 3 | -3 |  | `` Submit challenge error: %s `` | the rate-limit challenge could not be submitted | spec |
| CallsUnsupported | user | 1 | -1 |  | `` Voice calls are not supported by signal-cli-go yet `` | voice calls are deferred (PLAN.md milestone 9b) | deviation |
| IllegalArgumentCrash | crash | 2 | -32603 |  | `` Exception in thread "main" java.lang.IllegalArgumentException: %s ``<br>rpc: `` %s (IllegalArgumentException) `` | a value failed Java's argument checks (bad base64 in a MobileCoin address or group link, an odd-length sticker pack id) | spec |
| Internal | unexpected | 2 | -32603 |  | `` %s ``<br>rpc: `` %s (UnexpectedErrorException) `` | an error without a code reached the output; it is printed as is | deviation |
