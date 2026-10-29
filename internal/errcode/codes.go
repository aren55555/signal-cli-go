// SPDX-License-Identifier: GPL-3.0-or-later

package errcode

// Fixed parts of Jackson's messages (spec/surface/jsonrpc/errors.md,
// observations.md; parser messages also from black-box probing of the
// reference with a zero-account jsonRpc session).
const (
	jsonLoc  = "\n at [Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d]"
	jsonSrc  = "[Source: REDACTED (`StreamReadFeature.INCLUDE_SOURCE_IN_LOCATION` disabled); line: %d, column: %d]"
	bindLoc  = "\n at [Source: UNKNOWN; byte offset: #UNKNOWN]"
	reqClass = "org.asamk.signal.jsonrpc.JsonRpcRequest"
	reqChain = " (through reference chain: " + reqClass + "[\"%s\"])"
	jsonVals = "(JSON String, Number, Array, Object or token 'null', 'true' or 'false')"
)

const (
	_ Code = iota

	// JSON parser errors (-32700).
	RPCEmptyLine
	JSONUnrecognizedToken
	JSONNonStandardToken
	JSONExpectedValue
	JSONExpectedFieldName
	JSONExpectedColon
	JSONExpectedObjectComma
	JSONExpectedArrayComma
	JSONEOFInObject
	JSONEOFInArray
	JSONEOFCloseMarker
	JSONEOFInString
	JSONEOFInFieldName
	JSONEOFInEscape
	JSONMismatchedClose
	JSONUnmatchedClose
	JSONLeadingZeroes
	JSONNoDigitAfterSign
	JSONMinusNotDigit
	JSONDecimalPoint
	JSONExponentEOF
	JSONExponentNotDigit
	JSONBadEscape
	JSONBadUnicodeEscape
	JSONControlChar
	JSONIllegalWhitespace
	JSONComment
	JSONPlusSign
	JSONRootSpace
	JSONEOFInNumber
	JSONNestingDepth

	// Request validation (-32600 unless noted).
	RPCInvalidRequest
	RPCUnexpectedType
	RPCVersion
	RPCMethodMissing
	RPCUnrecognizedField
	RPCPropertyType
	RPCBatchElementScalar
	RPCDeserializeArray
	RPCDeserializeMember
	RPCDeserializeString
	RPCNumberRange
	RPCMethodNotImplemented
	RPCAccountRequired
	RPCAccountNotFound
	RPCAccountParamMissing
	RPCAccountLoadFailed
	RPCInternal
	RPCClassCast
	RPCSubscriptionUnknown
	RPCSubscriptionMissing
	RPCNullPointer
	RPCIllegalArgument
	RPCNullPointerPlain
	RPCIndexOutOfBounds

	// HTTP server responses.
	HTTPMissingHost
	HTTPNoContext
	HTTPNotFound
	HTTPMethodNotAllowed
	HTTPUnsupportedMediaType
	HTTPMisdirected
	HTTPUnknownAccount
	HTTPServerError

	// Command-line parsing (argparse4j).
	ArgUnrecognized
	ArgTooFew
	ArgRequired
	ArgOneOfRequired
	ArgNotAllowedWith
	ArgExpectedOne
	ArgExpectedN
	ArgNotInteger
	ArgNotDouble
	ArgConvertChoice
	ArgInvalidChoice
	ArgAmbiguousOption
	ArgAmbiguousCommand
	ArgInvalidCommand
	ArgIgnoredValue

	// Start-up and configuration.
	ConfigLoad
	ConfigUnknownField
	ConfigInvalidValue
	OutputTypeUnsupported
	MultiAccountOnly
	DBusUnavailable
	LinkWithAccount
	LiveUnsupported
	InvalidProxy
	NotImplemented
	DaemonNoChannel
	DaemonBadAddress
	DaemonBadPort
	DaemonBadHost
	DaemonBindFailed
	DaemonHTTPInit

	// Account selection.
	AccountsFileLoad
	AccountsListRead
	NoLocalUsers
	MultipleUsers
	InvalidAccountNumber
	UserNotRegistered
	StateFileLocked
	StateFileIO
	AccountCheckFailed
	NotPrimaryDevice

	// Recipients and groups.
	InvalidPhoneNumber
	RecipientNotRegistered
	PNIUnsupported
	UsernameSendUnsupported
	NoRecipients
	NoRecipient
	InvalidGroupID
	GroupNotFound
	GroupNotMember
	GroupSendNotAllowed
	GroupSendFailed
	GroupLastAdmin

	// Sending.
	SendFailed
	SendUntrusted
	SendRateLimited
	SendIOFailedUser
	SendIOFailed
	SendIOFailedIO
	SendAttachmentInvalid
	SendGroupCreateFailed
	SendGroupError
	EmptyMessage
	StdinReadFailed
	InvalidMention
	InvalidTextStyle
	InvalidStyle
	InvalidSticker
	StickerSendFailed
	QuoteAuthorMissing
	InvalidQuoteAttachment
	NumberParseFailed
	PollTooFewOptions
	PollTooManyOptions
	PollEmptyOption
	PollOptionTooLong
	PollAuthorRequired
	AdminDeleteNoGroups
	ReceiptBase64
	PaymentNoteRequired
	ReceiptTypeUnknown
	MessageRequestFailed
	StoryAttachmentRequired
	StoryAttachmentInvalid
	StorySendFailed
	BlockSyncFailed

	// Receiving.
	ReceiveFailed
	ReceiveBusy

	// Identities.
	TrustNeedsSafetyNumber
	SafetyNumberFormat
	TrustAllFailed
	TrustVerifiedFailed

	// Registration, linking and PINs.
	CaptchaRequired
	CaptchaInvalid
	RegisterRateLimited
	RegisterMethodUnavailable
	RegisterFailed
	RegisterAlreadyRegistered
	VerifyNoSession
	VerifyFailed
	VerifyRegLockFailed
	PinLocked
	PinMismatch
	PinDataDeleted
	PinUnsupported
	SetPinFailed
	RemovePinFailed
	LinkUserExists
	LinkRequestFailed
	FinishLinkUserExists
	InvalidDeviceLinkURI
	LinkTimedOut
	DeviceLinkTimedOut
	UnknownDeviceLinkURI
	MissingDeviceLinkURI
	NotDeletingRegistered
	DeletionFailed

	// Account settings, profiles, usernames, contacts.
	UpdateAccountFailed
	UsernameInvalid
	UsernameTaken
	UsernameSetFailed
	UsernameDeleteFailed
	UsernameLookupCrash
	UserStatusRateLimited
	UserStatusFailed
	UpdateProfileFailed
	ProfileInputTooLong
	UpdateContactFailed
	DeleteSelfRecipient
	AvatarNotFound
	AvatarReadFailed

	// Devices.
	ListDevicesFailed
	InvalidDeviceLink
	TooManyDevices
	AddDeviceFailed
	RemoveDeviceFailed
	UpdateDeviceFailed
	UnregisterFailed
	DeleteAccountUnsupported

	// Groups.
	GroupLinkNotALink
	GroupLinkIncompatible
	GroupLinkInvalid
	GroupLinkNotValid
	GroupPendingApproval
	InvalidGroupLinkState
	InvalidGroupPermission
	GroupAvatarInvalid

	// Attachments and stickers.
	AttachmentNotFound
	AttachmentReadFailed
	MissingAttachmentID
	StickerURIInvalid
	StickerPackLinkInvalid
	StickerInstallFailed
	StickerNotFound
	StickerReadFailed
	StickerPackInvalid
	StickerUploadFailed

	// Sync, calls, change number, rate-limit challenges.
	SyncRequestFailed
	SendContactsFailed
	NoCallID
	CallNotFound
	CallNotRinging
	HangupCallNotFound
	RejectCallNotFound
	StartCallFailed
	ChangeNumberInvalid
	ChangeNumberFailed
	SubmitChallengeFailed
	CallsUnsupported

	// Internal is an error that reached an output edge without a code.
	IllegalArgumentCrash

	Internal

	numCodes
)

func protocol(name string, rpc int, msg, cause string, src Source) info {
	return info{name: name, kind: KindProtocol, rpc: rpc, msg: msg, cause: cause, source: src}
}

func httpErr(name string, status int, body, cause string, src Source) info {
	return info{name: name, kind: KindHTTP, http: status, msg: body, cause: cause, source: src}
}

func argp(name, msg, cause string, src Source) info {
	return info{name: name, kind: KindArgparse, exit: 1, msg: msg, cause: cause, source: src}
}

func startup(name string, exit int, msg, cause string, src Source) info {
	return info{name: name, kind: KindStartup, exit: exit, msg: msg, cause: cause, source: src}
}

func user(name, msg, cause string, src Source) info {
	return info{name: name, kind: KindUser, exit: 1, rpc: -1, msg: msg, cause: cause, source: src}
}

func unexpected(name, msg, cause string, src Source) info {
	return info{name: name, kind: KindUnexpected, exit: 2, rpc: -32603, msg: msg, cause: cause, source: src}
}

func ioErr(name, msg, cause string, src Source) info {
	return info{name: name, kind: KindIO, exit: 3, rpc: -3, msg: msg, cause: cause, source: src}
}

// crash is an exception signal-cli does not catch: the JVM prints it
// and exits 2; the JSON-RPC handler reports "<message> (<SimpleName>)".
func crash(name, class, msg, cause string, src Source) info {
	simple := class[lastDot(class)+1:]
	return info{name: name, kind: KindCrash, exit: 2, rpc: -32603,
		msg: "Exception in thread \"main\" " + class + ": " + msg, rpcMsg: msg + " (" + simple + ")", cause: cause, source: src}
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

var table = [numCodes]info{
	RPCEmptyLine:            protocol("RPCEmptyLine", -32700, "Empty input line", "a stream line was empty or only whitespace", Observed),
	JSONUnrecognizedToken:   protocol("JSONUnrecognizedToken", -32700, "Unrecognized token '%s': was expecting "+jsonVals+jsonLoc, "input started with a bare word that is not a JSON literal", Observed),
	JSONNonStandardToken:    protocol("JSONNonStandardToken", -32700, "Non-standard token '%s': enable `JsonReadFeature.ALLOW_NON_NUMERIC_NUMBERS` to allow"+jsonLoc, "NaN or Infinity used as a JSON number", Spec),
	JSONExpectedValue:       protocol("JSONExpectedValue", -32700, "Unexpected character (%s): expected a valid value "+jsonVals+jsonLoc, "a character that cannot start a JSON value", Spec),
	JSONExpectedFieldName:   protocol("JSONExpectedFieldName", -32700, "Unexpected character (%s): was expecting double-quote to start field name"+jsonLoc, "an object member name was not a quoted string", Spec),
	JSONExpectedColon:       protocol("JSONExpectedColon", -32700, "Unexpected character (%s): was expecting a colon to separate field name and value"+jsonLoc, "an object member name was not followed by a colon", Spec),
	JSONExpectedObjectComma: protocol("JSONExpectedObjectComma", -32700, "Unexpected character (%s): was expecting comma to separate Object entries"+jsonLoc, "object members were not separated by a comma", Spec),
	JSONExpectedArrayComma:  protocol("JSONExpectedArrayComma", -32700, "Unexpected character (%s): was expecting comma to separate Array entries"+jsonLoc, "array elements were not separated by a comma", Spec),
	JSONEOFInObject:         protocol("JSONEOFInObject", -32700, "Unexpected end-of-input within/between Object entries"+jsonLoc, "input ended inside an object", Observed),
	JSONEOFInArray:          protocol("JSONEOFInArray", -32700, "Unexpected end-of-input within/between Array entries"+jsonLoc, "input ended inside an array after a comma", Spec),
	JSONEOFCloseMarker:      protocol("JSONEOFCloseMarker", -32700, "Unexpected end-of-input: expected close marker for %s (start marker at "+jsonSrc+")"+jsonLoc, "input ended before an object or array was closed", Spec),
	JSONEOFInString:         protocol("JSONEOFInString", -32700, "Unexpected end-of-input: was expecting closing quote for a string value"+jsonLoc, "input ended inside a string", Spec),
	JSONEOFInFieldName:      protocol("JSONEOFInFieldName", -32700, "Unexpected end-of-input in field name"+jsonLoc, "input ended inside an object member name", Spec),
	JSONEOFInEscape:         protocol("JSONEOFInEscape", -32700, "Unexpected end-of-input in character escape sequence"+jsonLoc, "input ended inside a backslash escape", Spec),
	JSONMismatchedClose:     protocol("JSONMismatchedClose", -32700, "Unexpected close marker '%s': expected '%s' (for %s starting at "+jsonSrc+")"+jsonLoc, "an object was closed with ] or an array with }", Spec),
	JSONUnmatchedClose:      protocol("JSONUnmatchedClose", -32700, "Unexpected close marker '%s': no open %s to close"+jsonLoc, "input started with a closing bracket", Spec),
	JSONLeadingZeroes:       protocol("JSONLeadingZeroes", -32700, "Invalid numeric value: Leading zeroes not allowed"+jsonLoc, "a number had a leading zero", Spec),
	JSONNoDigitAfterSign:    protocol("JSONNoDigitAfterSign", -32700, "Unexpected end-of-inputNo digit following sign"+jsonLoc, "input ended right after a minus sign", Spec),
	JSONMinusNotDigit:       protocol("JSONMinusNotDigit", -32700, "Unexpected character (%s) in numeric value: expected digit (0-9) to follow minus sign, for valid numeric value"+jsonLoc, "a minus sign was not followed by a digit", Spec),
	JSONDecimalPoint:        protocol("JSONDecimalPoint", -32700, "Unexpected character (%s) in numeric value: Decimal point not followed by a digit"+jsonLoc, "a decimal point was not followed by a digit", Spec),
	JSONExponentEOF:         protocol("JSONExponentEOF", -32700, "Unexpected end-of-inputexpected a digit for number exponent"+jsonLoc, "input ended inside a number's exponent", Spec),
	JSONExponentNotDigit:    protocol("JSONExponentNotDigit", -32700, "Unexpected character (%s) in numeric value: Exponent indicator not followed by a digit"+jsonLoc, "an exponent marker was not followed by a digit", Spec),
	JSONBadEscape:           protocol("JSONBadEscape", -32700, "Unrecognized character escape %s"+jsonLoc, "a backslash escape that JSON does not define", Spec),
	JSONBadUnicodeEscape:    protocol("JSONBadUnicodeEscape", -32700, "Unexpected character (%s): expected a hex-digit for character escape sequence"+jsonLoc, "a \\u escape with a non-hex digit", Spec),
	JSONControlChar:         protocol("JSONControlChar", -32700, "Illegal unquoted character (%s): has to be escaped using backslash to be included in %s"+jsonLoc, "a raw control character inside a string", Spec),
	JSONIllegalWhitespace:   protocol("JSONIllegalWhitespace", -32700, "Illegal character (%s): only regular white space (\\r, \\n, \\t) is allowed between tokens"+jsonLoc, "a control character between tokens", Spec),
	JSONComment:             protocol("JSONComment", -32700, "Unexpected character ('/' (code 47)): maybe a (non-standard) comment? (not recognized as one since Feature 'ALLOW_COMMENTS' not enabled for parser)"+jsonLoc, "a slash between tokens", Spec),
	JSONPlusSign:            protocol("JSONPlusSign", -32700, "Unexpected character ('+' (code 43)) in numeric value: JSON spec does not allow numbers to have plus signs: enable `JsonReadFeature.ALLOW_LEADING_PLUS_SIGN_FOR_NUMBERS` to allow"+jsonLoc, "a number with a leading plus sign", Spec),
	JSONRootSpace:           protocol("JSONRootSpace", -32700, "Unexpected character (%s): Expected space separating root-level values"+jsonLoc, "a top-level number directly followed by another character", Spec),
	JSONEOFInNumber:         protocol("JSONEOFInNumber", -32700, "Unexpected end-of-input in a Number value"+jsonLoc, "input ended inside a number", Spec),
	JSONNestingDepth:        info{name: "JSONNestingDepth", kind: KindCrash, exit: 2, rpc: -32603, msg: "java.lang.AssertionError: com.fasterxml.jackson.core.exc.StreamConstraintsException: Document nesting depth (%d) exceeds the maximum allowed (%d, from `StreamReadConstraints.getMaxNestingDepth()`)", rpcMsg: "Document nesting depth (%d) exceeds the maximum allowed (%d, from `StreamReadConstraints.getMaxNestingDepth()`) (StreamConstraintsException)", cause: "input nested deeper than 1000 levels; the reference's reader thread dies", source: Spec},

	RPCInvalidRequest:       protocol("RPCInvalidRequest", -32600, "invalid request", "the request was an empty batch", Observed),
	RPCUnexpectedType:       protocol("RPCUnexpectedType", -32600, "unexpected type: %s", "the request was a JSON scalar (or nothing) instead of an object or array", Observed),
	RPCVersion:              protocol("RPCVersion", -32600, "only jsonrpc version 2.0 is supported", "jsonrpc member missing or not \"2.0\"", Observed),
	RPCMethodMissing:        protocol("RPCMethodMissing", -32600, "method field must be set", "method member missing or null", Observed),
	RPCUnrecognizedField:    protocol("RPCUnrecognizedField", -32600, "Unrecognized field \"%s\" (class %s), not marked as ignorable (%d known properties: %s)"+bindLoc+" (through reference chain: %s[\"%s\"])", "the request object, or a typed-params method's params, had an unknown member", Observed),
	RPCPropertyType:         protocol("RPCPropertyType", -32600, "Problem deserializing property '%s' (expected type: [simple type, class %s]; actual type: `%s`), problem: Can not set %s field "+reqClass+".%s to %s"+bindLoc+reqChain, "id was an array or object, or params was a scalar", Observed),
	RPCBatchElementScalar:   protocol("RPCBatchElementScalar", -32600, "Cannot construct instance of `"+reqClass+"` (although at least one Creator exists): no %s-argument constructor/factory method to deserialize from %s value (%s)"+bindLoc, "a batch element was a string, number or boolean", Observed),
	RPCDeserializeArray:     protocol("RPCDeserializeArray", -32600, "Cannot deserialize value of type `%s` from Array value (token `JsonToken.START_ARRAY`)"+bindLoc, "an array where an object was expected: params of a method that takes an object, or a batch element", Observed),
	RPCDeserializeMember:    protocol("RPCDeserializeMember", -32600, "Cannot deserialize value of type `%s` from %s value (token `JsonToken.%s`)"+bindLoc+" (through reference chain: %s[\"%s\"])", "a request or typed-params member had the wrong JSON type", Spec),
	RPCDeserializeString:    protocol("RPCDeserializeString", -32600, "Cannot deserialize value of type `%s` from String %s: %s"+bindLoc+" (through reference chain: %s[\"%s\"])", "a typed-params member was a string that does not convert to its type", Spec),
	RPCNumberRange:          protocol("RPCNumberRange", -32600, "Numeric value (%s) out of range of int (-2147483648 - 2147483647)"+bindLoc+" (through reference chain: %s[\"%s\"])", "a typed-params integer member was outside the int range", Spec),
	RPCMethodNotImplemented: protocol("RPCMethodNotImplemented", -32601, "Method not implemented", "no such JSON-RPC method in this mode or over this transport", Observed),
	RPCAccountRequired:      protocol("RPCAccountRequired", -32602, "Method requires valid account parameter", "no account given and the daemon does not have exactly one", Observed),
	RPCAccountNotFound:      protocol("RPCAccountNotFound", -32602, "Specified account does not exist", "the account parameter names no loaded account", Observed),
	RPCAccountParamMissing:  protocol("RPCAccountParamMissing", -32602, "Method requires account parameter", "a registration method was called without an account key", Observed),
	RPCAccountLoadFailed:    protocol("RPCAccountLoadFailed", -32602, "Failed to load the given account", "the registration method's account could not be opened or locked", Spec),
	RPCInternal:             protocol("RPCInternal", -32603, "An internal server error has occurred.", "an exception escaped HTTP request processing", Spec),
	RPCClassCast:            protocol("RPCClassCast", -32603, "class %s cannot be cast to class %s (%s and %s are in module java.base of loader 'bootstrap') (ClassCastException)", "a parameter had the wrong JSON type", Spec),
	RPCSubscriptionUnknown:  protocol("RPCSubscriptionUnknown", -1, "Unknown subscription id", "no subscription with that id on this connection", Observed),
	RPCSubscriptionMissing:  protocol("RPCSubscriptionMissing", -1, "Missing subscription parameter with subscription id", "unsubscribe without params in multi-account mode", Observed),
	RPCNullPointer:          protocol("RPCNullPointer", -32603, "Cannot invoke \"%s\" because %s is null (NullPointerException)", "a required parameter was missing (a Java NullPointerException in the reference)", Observed),
	RPCIllegalArgument:      protocol("RPCIllegalArgument", -32603, "%s (IllegalArgumentException)", "a parameter value was outside its allowed set", Spec),
	RPCNullPointerPlain:     protocol("RPCNullPointerPlain", -32603, "null (NullPointerException)", "a required parameter was missing (a NullPointerException without a message)", Spec),
	RPCIndexOutOfBounds:     protocol("RPCIndexOutOfBounds", -32603, "Index %d out of bounds for length %d (StringIndexOutOfBoundsException)", "an odd-length hex value was decoded", Spec),

	HTTPMissingHost:          httpErr("HTTPMissingHost", 400, "<h1>400 Bad Request</h1>Missing Host header", "an HTTP/1.1 request without a Host header", Observed),
	HTTPNoContext:            httpErr("HTTPNoContext", 404, "<h1>404 Not Found</h1>No context found for request", "the path is outside /api/v1/rpc, /api/v1/events and /api/v1/check", Observed),
	HTTPNotFound:             httpErr("HTTPNotFound", 404, "", "the path extends one of the endpoint paths", Observed),
	HTTPMethodNotAllowed:     httpErr("HTTPMethodNotAllowed", 405, "", "wrong HTTP method for the endpoint", Observed),
	HTTPUnsupportedMediaType: httpErr("HTTPUnsupportedMediaType", 415, "", "Content-Type missing or not application/json", Observed),
	HTTPMisdirected:          httpErr("HTTPMisdirected", 421, "", "the Host header names neither the bind address nor loopback", Observed),
	HTTPUnknownAccount:       httpErr("HTTPUnknownAccount", 400, "", "the events stream's account parameter names no loaded account", Observed),
	HTTPServerError:          httpErr("HTTPServerError", 500, "", "an unexpected failure before the events stream started", Spec),

	ArgUnrecognized:     argp("ArgUnrecognized", "unrecognized arguments: '%s'", "an argument the command does not take", Observed),
	ArgTooFew:           argp("ArgTooFew", "too few arguments", "a required positional argument is missing", Observed),
	ArgRequired:         argp("ArgRequired", "argument %s is required", "a required option is missing", Observed),
	ArgOneOfRequired:    argp("ArgOneOfRequired", "one of the arguments %s is required", "none of a required group's options was given", Observed),
	ArgNotAllowedWith:   argp("ArgNotAllowedWith", "argument %s: not allowed with argument %s", "two mutually exclusive options were given", Observed),
	ArgExpectedOne:      argp("ArgExpectedOne", "argument %s: expected one argument", "an option that takes a value was given none", Observed),
	ArgExpectedN:        argp("ArgExpectedN", "argument %s: expected %d argument(s)", "a list option was given too few values", Observed),
	ArgNotInteger:       argp("ArgNotInteger", "argument %s: could not convert '%s' to integer (%d bits)", "an integer option got a non-integer or out-of-range value", Observed),
	ArgNotDouble:        argp("ArgNotDouble", "argument %s: could not convert '%s' to number (double precision)", "a numeric option got a non-number", Observed),
	ArgConvertChoice:    argp("ArgConvertChoice", "argument %s: could not convert '%s' (choose from %s)", "an enum option got a value outside its choices", Observed),
	ArgInvalidChoice:    argp("ArgInvalidChoice", "argument %s: invalid choice: '%s' (choose from %s)", "a choice option got a value outside its choices", Observed),
	ArgAmbiguousOption:  argp("ArgAmbiguousOption", "ambiguous option: %s could match %s", "an abbreviated option matches several options", Observed),
	ArgAmbiguousCommand: argp("ArgAmbiguousCommand", "ambiguous command: %s could match %s", "an abbreviated command matches several commands", Spec),
	ArgInvalidCommand:   argp("ArgInvalidCommand", "invalid choice: '%s' (choose from %s)", "no such command", Observed),
	ArgIgnoredValue:     argp("ArgIgnoredValue", "argument %s: ignore implicit argument '%s'", "a flag was given a value with =", Spec),

	ConfigLoad:            startup("ConfigLoad", 1, "Failed to load config from %s: %s", "a configuration file is not valid JSON", Observed),
	ConfigUnknownField:    startup("ConfigUnknownField", 1, "Failed to parse configuration file(s): Unrecognized field \"%s\" (class org.asamk.signal.GlobalConfig), not marked as ignorable (%d known properties: %s)"+bindLoc+" (through reference chain: org.asamk.signal.GlobalConfig[\"%s\"])", "a configuration file has an unknown key", Observed),
	ConfigInvalidValue:    startup("ConfigInvalidValue", 1, "Failed to parse configuration file(s): Cannot construct instance of `%s`, problem: %s"+bindLoc+" (through reference chain: org.asamk.signal.GlobalConfig[\"%s\"])", "a configuration value has the wrong type or an invalid value", Observed),
	OutputTypeUnsupported: startup("OutputTypeUnsupported", 1, "Command doesn't support output type %s", "the command has only plain-text or only JSON output", Observed),
	MultiAccountOnly:      startup("MultiAccountOnly", 1, "Command only works in multi-account mode", "a multi-account command was given -a", Observed),
	DBusUnavailable:       startup("DBusUnavailable", 2, "Dbus client failed: Cannot Resolve Session Bus Address: %s", "--dbus or --dbus-system without a reachable bus", Observed),
	LinkWithAccount:       startup("LinkWithAccount", 1, "You cannot specify a account (phone number) when linking", "link was given -a", Observed),
	LiveUnsupported:       startup("LiveUnsupported", 1, "The live service environment is not supported by signal-cli-go yet; use --service-environment staging", "only the staging environment is configured", Deviation),
	InvalidProxy:          startup("InvalidProxy", 1, "Invalid proxy setting: %s", "the proxy environment variable could not be parsed", Deviation),
	NotImplemented:        info{name: "NotImplemented", kind: KindUnexpected, exit: 2, rpc: -32603, msg: "Command %s is not implemented in signal-cli-go yet", rpcMsg: "Command %s is not implemented in signal-cli-go yet", cause: "the command has no implementation", source: Deviation},
	DaemonNoChannel:       startup("DaemonNoChannel", 1, "At least one channel parameter is required, e.g. --socket or --dbus.", "daemon was started without a transport", Spec),
	DaemonBadAddress:      startup("DaemonBadAddress", 1, "Invalid tcp bind address (expected host:port): %s", "a bind address without a colon", Spec),
	DaemonBadPort:         startup("DaemonBadPort", 1, "Invalid tcp port: %s", "a bind address whose port is not an integer", Spec),
	DaemonBadHost:         startup("DaemonBadHost", 1, "Invalid tcp bind address, invalid host: %s", "a bind address whose host does not resolve", Spec),
	DaemonBindFailed:      startup("DaemonBindFailed", 3, "Failed to bind socket %s: %s", "the TCP port or socket path could not be bound", Spec),
	DaemonHTTPInit:        startup("DaemonHTTPInit", 3, "Failed to initialize HTTP Server", "the HTTP port could not be bound", Spec),

	AccountsFileLoad:     info{name: "AccountsFileLoad", kind: KindIO, exit: 3, rpc: -3, msg: "Failed to load local accounts file", cause: "the accounts.json index could not be read", source: Spec},
	AccountsListRead:     ioErr("AccountsListRead", "Failed to read local accounts list", "the data directory could not be created or read", Observed),
	NoLocalUsers:         user("NoLocalUsers", "No local users found, you first need to register or link an account", "no -a and no local accounts", Observed),
	MultipleUsers:        user("MultipleUsers", "Multiple users found, you need to specify an account (phone number) with -a", "no -a and several local accounts", Observed),
	InvalidAccountNumber: user("InvalidAccountNumber", "Invalid account (phone number), make sure you include the country code.", "-a is neither an E.164 number nor a UUID", Observed),
	UserNotRegistered:    user("UserNotRegistered", "User %s is not registered.", "the account does not exist locally or is not registered on the server", Observed),
	StateFileLocked:      unexpected("StateFileLocked", "Error loading state file for user %s: %s (OverlappingFileLockException)", "another process holds the account's lock", Spec),
	StateFileIO:          unexpected("StateFileIO", "Error loading state file for user %s: %s (IOException)", "the account file could not be read", Spec),
	AccountCheckFailed:   ioErr("AccountCheckFailed", "Error while checking account %s: %s", "the start-up account check against the server failed", Spec),
	NotPrimaryDevice:     user("NotPrimaryDevice", "This command doesn't work on linked devices.", "the command needs the primary device", Spec),

	InvalidPhoneNumber:      user("InvalidPhoneNumber", "Invalid phone number '%s': %s", "a recipient could not be parsed as a number, UUID, PNI or username", Spec),
	RecipientNotRegistered:  user("RecipientNotRegistered", "The user %s is not registered.", "a recipient is not a Signal user", Spec),
	PNIUnsupported:          user("PNIUnsupported", "Sending to a PNI is not supported by signal-cli-go yet", "PNI recipients are not implemented", Deviation),
	UsernameSendUnsupported: user("UsernameSendUnsupported", "Sending to a username is not supported by signal-cli-go yet", "username recipients are not implemented for this command", Deviation),
	NoRecipients:            user("NoRecipients", "No recipients given", "no recipient, group or note-to-self target", Spec),
	NoRecipient:             user("NoRecipient", "No recipient given", "the command needs exactly one recipient", Spec),
	InvalidGroupID:          user("InvalidGroupID", "Invalid group id: %s", "a group id is not base64 of 16 or 32 bytes", Spec),
	GroupNotFound:           user("GroupNotFound", "Group not found: %s", "no local group with that id", Spec),
	GroupNotMember:          user("GroupNotMember", "User is not a member in group: %s (%s)", "the account is not a member of the group", Spec),
	GroupSendNotAllowed:     user("GroupSendNotAllowed", "User is not allowed to send message to group: %s (%s)", "the group only lets admins send", Spec),
	GroupSendFailed:         user("GroupSendFailed", "Failed to send to group: %s", "sending to a group failed on a group problem", Spec),
	GroupLastAdmin:          user("GroupLastAdmin", "You need to specify a new admin with --admin: User is last admin in group: %s (%s)", "the last admin tried to leave without naming a successor", Spec),

	SendFailed:              user("SendFailed", "Failed to send message", "every recipient failed, for mixed reasons", Spec),
	SendUntrusted:           info{name: "SendUntrusted", kind: KindUntrusted, exit: 4, rpc: -4, msg: "Failed to send message due to untrusted identities", cause: "every recipient failed on an untrusted identity", source: Spec},
	SendRateLimited:         info{name: "SendRateLimited", kind: KindRateLimit, exit: 5, rpc: -5, msg: "Failed to send message due to rate limiting", cause: "every recipient failed on a rate limit", source: Spec},
	SendIOFailedUser:        user("SendIOFailedUser", "Failed to send message: %s (IOException)", "network failure while sending to individual recipients", Spec),
	SendIOFailed:            unexpected("SendIOFailed", "Failed to send message: %s (IOException)", "network failure while sending", Spec),
	SendIOFailedIO:          ioErr("SendIOFailedIO", "Failed to send message: %s (%s)", "network failure while sending a group change", Spec),
	SendAttachmentInvalid:   unexpected("SendAttachmentInvalid", "Failed to send message: %s (AttachmentInvalidException)", "an attachment file could not be read", Spec),
	SendGroupCreateFailed:   unexpected("SendGroupCreateFailed", "Failed to send message: Failed to create group (IOException)", "the server refused to create the group", Spec),
	SendGroupError:          unexpected("SendGroupError", "Failed to send message: %s (%s)", "a group operation failed", Spec),
	EmptyMessage:            user("EmptyMessage", "Sending empty message is not allowed, either a message, attachment or sticker must be given.", "nothing to send", Spec),
	StdinReadFailed:         user("StdinReadFailed", "Failed to read message from stdin: %s", "the message could not be read from standard input", Spec),
	InvalidMention:          user("InvalidMention", "Invalid mention syntax (%s) expected 'start:length:recipientNumber'", "a mention is not start:length:recipient", Spec),
	InvalidTextStyle:        user("InvalidTextStyle", "Invalid textStyle syntax (%s) expected 'start:length:STYLE'", "a text style is not start:length:STYLE", Spec),
	InvalidStyle:            user("InvalidStyle", "Invalid style: %s", "an unknown text style name", Spec),
	InvalidSticker:          user("InvalidSticker", "Invalid sticker syntax (%s) expected 'stickerPackId:stickerId'", "a sticker is not packId:stickerId", Spec),
	StickerSendFailed:       user("StickerSendFailed", "Failed to send sticker: %s", "the sticker is not installed or invalid", Spec),
	QuoteAuthorMissing:      user("QuoteAuthorMissing", "Quote author parameter is missing", "a quote timestamp without a quote author", Spec),
	InvalidQuoteAttachment:  user("InvalidQuoteAttachment", "Invalid attachment syntax (%s) expected 'contentType[:filename[:previewFile]]'", "a quote attachment is malformed", Spec),
	NumberParseFailed:       unexpected("NumberParseFailed", "Failed to parse number: %s", "a numeric field of a structured argument is not a number", Spec),
	PollTooFewOptions:       user("PollTooFewOptions", "Poll needs at least two options", "fewer than two poll options", Spec),
	PollTooManyOptions:      user("PollTooManyOptions", "Poll cannot have more than 10 options", "more than ten poll options", Spec),
	PollEmptyOption:         user("PollEmptyOption", "Poll options must not be empty", "an empty poll option", Spec),
	PollOptionTooLong:       user("PollOptionTooLong", "Poll option \"%s\" exceeds the maximum length of 100 characters", "a poll option longer than 100 characters", Spec),
	PollAuthorRequired:      unexpected("PollAuthorRequired", "Poll author is required", "a poll vote without the poll's author", Spec),
	AdminDeleteNoGroups:     user("AdminDeleteNoGroups", "Admin delete requires group IDs", "admin delete without a group", Spec),
	ReceiptBase64:           unexpected("ReceiptBase64", "Illegal base64 receipt: %s", "a payment receipt is not base64", Spec),
	PaymentNoteRequired:     unexpected("PaymentNoteRequired", "Payment note is required", "a payment notification without a note", Spec),
	ReceiptTypeUnknown:      user("ReceiptTypeUnknown", "Unknown receipt type: %s", "a receipt type other than read or viewed", Spec),
	MessageRequestFailed:    ioErr("MessageRequestFailed", "Failed to send message request response: %s", "the message request response could not be sent", Spec),
	StoryAttachmentRequired: user("StoryAttachmentRequired", "An attachment is required for sending a story.", "a story without an attachment", Spec),
	StoryAttachmentInvalid:  unexpected("StoryAttachmentInvalid", "Failed to send story: %s: %s (AttachmentInvalidException)", "the story attachment could not be read", Spec),
	StorySendFailed:         unexpected("StorySendFailed", "Failed to send story: %s (IOException)", "network failure while sending a story", Spec),
	BlockSyncFailed:         unexpected("BlockSyncFailed", "Failed to sync block to linked devices: %s", "the blocked-list sync message could not be sent", Spec),

	ReceiveFailed: ioErr("ReceiveFailed", "Error while receiving messages: %s", "network failure while receiving", Spec),
	ReceiveBusy:   user("ReceiveBusy", "Receive command cannot be used if messages are already being received.", "the daemon is already receiving for this account", Spec),

	TrustNeedsSafetyNumber: user("TrustNeedsSafetyNumber", "You need to specify the fingerprint/safety number you have verified with -v SAFETY_NUMBER", "trust without -a or -v", Spec),
	SafetyNumberFormat:     user("SafetyNumberFormat", "Safety number has invalid format, either specify the old hex fingerprint or the new safety number", "the safety number is neither hex, 60 digits nor a scannable code", Spec),
	TrustAllFailed:         user("TrustAllFailed", "Failed to set the trust for this number, make sure the number is correct.", "no identity key is known for the recipient", Spec),
	TrustVerifiedFailed:    user("TrustVerifiedFailed", "Failed to set the trust for this number, make sure the number and the fingerprint/safety number are correct.", "the safety number does not match the recipient's key", Spec),

	CaptchaRequired:           user("CaptchaRequired", "Captcha required for verification, use --captcha CAPTCHA\nTo get the token, go to https://signalcaptchas.org/registration/generate.html\nAfter solving the captcha, right-click on the \"Open Signal\" link and copy the link.%s", "the server requires a captcha before sending a code", Spec),
	CaptchaInvalid:            user("CaptchaInvalid", "Invalid captcha given.%s", "the server rejected the captcha", Spec),
	RegisterRateLimited:       info{name: "RegisterRateLimited", kind: KindRateLimit, exit: 5, rpc: -5, msg: "Rate limit reached%s", cause: "the server rate-limited the request", source: Spec},
	RegisterMethodUnavailable: user("RegisterMethodUnavailable", "Failed to register: Invalid verification method%s", "the server cannot send a code by the chosen method", Spec),
	RegisterFailed:            ioErr("RegisterFailed", "Failed to register: %s (%s)", "the registration request failed", Spec),
	RegisterAlreadyRegistered: ioErr("RegisterAlreadyRegistered", "Failed to register: Account is already registered (IOException)", "register without --reregister on a registered account", Spec),
	VerifyNoSession:           ioErr("VerifyNoSession", "Verify error: No registration verification session active", "verify without a prior register", Spec),
	VerifyFailed:              ioErr("VerifyFailed", "Verify error: %s", "the verification code was rejected or the request failed", Spec),
	VerifyRegLockFailed:       crash("VerifyRegLockFailed", "java.lang.AssertionError", "KBS Pin appeared to matched but reg lock still failed!", "the restored PIN's registration lock token was refused", Spec),
	PinLocked:                 user("PinLocked", "Verification failed! This number is locked with a pin. Hours remaining until reset: %d\nUse '--pin PIN_CODE' to specify the registration lock PIN", "the number has a registration lock and no PIN was given", Spec),
	PinMismatch:               user("PinMismatch", "Verification failed! Invalid pin, tries remaining: %d", "the PIN does not match the SVR backup", Spec),
	PinDataDeleted:            user("PinDataDeleted", "Account is pin locked, but pin data has been deleted on the server.", "the number is locked but the SVR backup is gone", Spec),
	PinUnsupported:            user("PinUnsupported", "PIN contains characters signal-cli-go cannot normalize; use digits or ASCII", "the PIN needs Unicode normalization signal-cli-go does not implement", Deviation),
	SetPinFailed:              ioErr("SetPinFailed", "Set pin error: %s", "the PIN backup to SVR failed", Spec),
	RemovePinFailed:           ioErr("RemovePinFailed", "Remove pin error: %s", "deleting the SVR backup failed", Spec),
	LinkUserExists:            user("LinkUserExists", "User already exists", "the linked account already exists locally", Spec),
	LinkRequestFailed:         ioErr("LinkRequestFailed", "Link request error: %s", "the provisioning exchange failed", Spec),
	FinishLinkUserExists:      user("FinishLinkUserExists", "The user %s already exists\nDelete \"%s\" before trying again.", "finishLink for a number that already has a local account", Spec),
	InvalidDeviceLinkURI:      user("InvalidDeviceLinkURI", "Invalid device link uri.", "finishLink's deviceLinkUri is not a valid URI", Spec),
	LinkTimedOut:              user("LinkTimedOut", "Link request timed out, please try again.", "the primary device did not answer in time", Spec),
	DeviceLinkTimedOut:        user("DeviceLinkTimedOut", "Device link creation timed out, please try again.", "no provisioning address arrived in time", Spec),
	UnknownDeviceLinkURI:      user("UnknownDeviceLinkURI", "Unknown device link uri.", "finishLink for a URI no startLink produced", Spec),
	MissingDeviceLinkURI:      user("MissingDeviceLinkURI", "Missing deviceLinkUri.", "finishLink without deviceLinkUri", Spec),
	NotDeletingRegistered:     user("NotDeletingRegistered", "Not deleting account, it is still registered. Use --ignore-registered to delete it anyway.", "deleteLocalAccountData on a registered account", Spec),
	DeletionFailed:            ioErr("DeletionFailed", "Deletion error: %s", "the account files could not be deleted", Spec),

	UpdateAccountFailed:   ioErr("UpdateAccountFailed", "UpdateAccount error: %s", "the account attribute update failed", Spec),
	UsernameInvalid:       user("UsernameInvalid", "Invalid username: %s (%s)", "the nickname is not a valid username", Spec),
	UsernameTaken:         user("UsernameTaken", "Invalid username: Username is malformed or already taken", "the server refused to reserve the username", Spec),
	UsernameSetFailed:     ioErr("UsernameSetFailed", "Failed to set username: %s", "network failure while setting the username", Spec),
	UsernameDeleteFailed:  ioErr("UsernameDeleteFailed", "Failed to delete username: %s", "network failure while deleting the username", Spec),
	UsernameLookupCrash:   crash("UsernameLookupCrash", "java.lang.AssertionError", "org.signal.libsignal.usernames.%s: %s", "a looked-up username is not valid", Spec),
	UserStatusRateLimited: info{name: "UserStatusRateLimited", kind: KindRateLimit, exit: 5, rpc: -5, msg: "Rate limit reached\nNext attempt may be tried at %s", cause: "the contact discovery service rate-limited the lookup", source: Spec},
	UserStatusFailed:      ioErr("UserStatusFailed", "Unable to check if users are registered: %s (%s)", "the number or username lookup failed", Spec),
	UpdateProfileFailed:   ioErr("UpdateProfileFailed", "Update profile error: %s", "the profile upload failed or the avatar could not be read", Spec),
	ProfileInputTooLong:   crash("ProfileInputTooLong", "java.lang.AssertionError", "org.signal.libsignal.zkgroup.InvalidInputException: Input is too long", "a profile field exceeds the largest padding bucket", Spec),
	UpdateContactFailed:   ioErr("UpdateContactFailed", "Update contact error: %s", "sending the contact update failed", Spec),
	DeleteSelfRecipient:   crash("DeleteSelfRecipient", "java.lang.RuntimeException", "Can't delete self recipient", "removeContact --forget on the account itself", Spec),
	AvatarNotFound:        user("AvatarNotFound", "Could not find avatar", "no avatar is stored", Spec),
	AvatarReadFailed:      unexpected("AvatarReadFailed", "An error occurred reading avatar", "the avatar file could not be read", Spec),

	ListDevicesFailed:        ioErr("ListDevicesFailed", "Failed to get linked devices: %s", "the device list request failed", Spec),
	InvalidDeviceLink:        user("InvalidDeviceLink", "Invalid device link", "the device link URI lacks a valid uuid or key", Spec),
	TooManyDevices:           user("TooManyDevices", "Account has too many linked devices already", "the server's device limit was reached", Spec),
	AddDeviceFailed:          ioErr("AddDeviceFailed", "Add device link failed", "provisioning the new device failed", Spec),
	RemoveDeviceFailed:       ioErr("RemoveDeviceFailed", "Error while removing device: %s", "the device removal request failed", Spec),
	UpdateDeviceFailed:       ioErr("UpdateDeviceFailed", "Error while updating device: %s", "the device rename request failed", Spec),
	UnregisterFailed:         ioErr("UnregisterFailed", "Unregister error: %s", "the unregister request failed", Spec),
	DeleteAccountUnsupported: user("DeleteAccountUnsupported", "Deleting the account is not supported by signal-cli-go yet", "account deletion is not implemented", Deviation),

	GroupLinkNotALink:      user("GroupLinkNotALink", "Link is not a signal group invitation link", "the URI is not a signal.group link", Spec),
	GroupLinkIncompatible:  user("GroupLinkIncompatible", "Group link was created with an incompatible version: %s", "the link has no known content version", Spec),
	GroupLinkInvalid:       user("GroupLinkInvalid", "Group link is invalid: %s", "the link's fragment could not be decoded", Spec),
	GroupLinkNotValid:      user("GroupLinkNotValid", "Group link is not valid: %s", "the server says the link is inactive", Spec),
	GroupPendingApproval:   user("GroupPendingApproval", "Pending admin approval: %s", "a join request is already pending", Spec),
	InvalidGroupLinkState:  user("InvalidGroupLinkState", "Invalid group link state: %s", "an unknown group link mode", Spec),
	InvalidGroupPermission: user("InvalidGroupPermission", "Invalid group permission: %s", "an unknown group permission value", Spec),
	GroupAvatarInvalid:     user("GroupAvatarInvalid", "Failed to add avatar attachment for group\": %s", "the group avatar file could not be read", Spec),

	AttachmentNotFound:     user("AttachmentNotFound", "Could not find attachment with ID: %s", "no stored attachment with that id", Spec),
	AttachmentReadFailed:   unexpected("AttachmentReadFailed", "An error occurred reading attachment: %s", "the attachment file could not be read", Spec),
	MissingAttachmentID:    user("MissingAttachmentID", "Missing attachment id parameter", "getAttachment without id", Spec),
	StickerURIInvalid:      user("StickerURIInvalid", "Sticker pack uri has invalid format: %s", "the sticker pack URI is not a valid URI", Spec),
	StickerPackLinkInvalid: user("StickerPackLinkInvalid", "Invalid sticker pack link", "the sticker pack link lacks a valid pack id or key", Spec),
	StickerInstallFailed:   ioErr("StickerInstallFailed", "Install sticker pack failed", "downloading the sticker pack failed", Spec),
	StickerNotFound:        user("StickerNotFound", "Could not find sticker with ID: %d in pack StickerPackId{%s}", "the sticker is not stored locally", Spec),
	StickerReadFailed:      unexpected("StickerReadFailed", "An error occurred reading sticker with ID: %d in pack StickerPackId{%s}", "the sticker file could not be read", Spec),
	StickerPackInvalid:     user("StickerPackInvalid", "Invalid sticker pack: %s", "the sticker pack manifest or files are invalid", Spec),
	StickerUploadFailed:    ioErr("StickerUploadFailed", "Upload error (maybe image size too large):%s", "uploading the sticker pack failed", Spec),

	SyncRequestFailed:     ioErr("SyncRequestFailed", "Request sync data error: %s", "the sync request could not be sent", Spec),
	SendContactsFailed:    ioErr("SendContactsFailed", "SendContacts error: %s", "the contacts sync could not be sent", Spec),
	NoCallID:              user("NoCallID", "No call ID given", "a call method without callId", Spec),
	CallNotFound:          ioErr("CallNotFound", "Failed to accept call: No active call with id %s", "no active call with that id", Spec),
	CallNotRinging:        ioErr("CallNotRinging", "Failed to accept call: Call %d is not in RINGING_INCOMING state (current: %s)", "the call is not ringing", Spec),
	HangupCallNotFound:    ioErr("HangupCallNotFound", "Failed to hang up call: No active call with id %s", "no active call with that id", Spec),
	RejectCallNotFound:    ioErr("RejectCallNotFound", "Failed to reject call: No active call with id %s", "no active call with that id", Spec),
	StartCallFailed:       ioErr("StartCallFailed", "Failed to start call: %s", "the call could not be started", Spec),
	ChangeNumberInvalid:   user("ChangeNumberInvalid", "Failed to change number: %s", "the new number is not a normalized E.164 number", Spec),
	ChangeNumberFailed:    ioErr("ChangeNumberFailed", "Failed to change number: %s (%s)", "the change-number request failed", Spec),
	SubmitChallengeFailed: ioErr("SubmitChallengeFailed", "Submit challenge error: %s", "the rate-limit challenge could not be submitted", Spec),
	CallsUnsupported:      user("CallsUnsupported", "Voice calls are not supported by signal-cli-go yet", "voice calls are deferred (PLAN.md milestone 9b)", Deviation),

	IllegalArgumentCrash: crash("IllegalArgumentCrash", "java.lang.IllegalArgumentException", "%s", "a value failed Java's argument checks (bad base64 in a MobileCoin address or group link, an odd-length sticker pack id)", Spec),

	Internal: unexpected("Internal", "%s", "an error without a code reached the output; it is printed as is", Deviation),
}
