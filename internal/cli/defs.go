package cli

import (
	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
	"strings"
)

// The parser definitions transcribe spec/surface/cli/index.md and the
// per-command files; help strings are verbatim.

const (
	description = "Commandline interface for Signal."
	epilog      = "The global arguments are shown with 'signal-cli -h' and need to come before the subcommand, while the subcommand-specific arguments (shown with 'signal-cli SUBCOMMAND -h') need to be given after the subcommand."
)

func helpArg() *argparse.Arg {
	return &argparse.Arg{Names: []string{"-h", "--help"}, Action: argparse.Help, Help: "show this help message and exit"}
}

// commandHelp is index.md §9's table: name and help text, in the
// parser's order. version has no help text.
var commandHelp = [][2]string{
	{"acceptCall", "Accept an incoming voice call."},
	{"addDevice", "Link another device to this device. Only works, if this is the primary device."},
	{"addStickerPack", "Install a sticker pack for this account."},
	{"block", "Block the given contacts or groups (no messages will be received)"},
	{"daemon", "Run in daemon mode and provide a JSON-RPC or an experimental dbus interface."},
	{"deleteLocalAccountData", "Delete all local data for this account. Data should only be deleted if the account is unregistered. CAUTION: This cannot be undone."},
	{"finishChangeNumber", "Verify the new number using the code received via SMS or voice."},
	{"getAttachment", "Retrieve an already downloaded attachment base64 encoded."},
	{"getAvatar", "Retrieve the avatar of a contact, contact's profile or group base64 encoded."},
	{"getSticker", "Retrieve the sticker of a sticker pack base64 encoded."},
	{"getUserStatus", "Check if the specified phone number/s have been registered"},
	{"hangupCall", "Hang up an active voice call."},
	{"joinGroup", "Join a group via an invitation link."},
	{"jsonRpc", "Take commands from standard input as line-delimited JSON RPC while receiving messages."},
	{"link", "Link to an existing device, instead of registering a new number."},
	{"listAccounts", "Show a list of registered accounts."},
	{"listCalls", "List active voice calls."},
	{"listContacts", "Show a list of known contacts with names and profiles."},
	{"listDevices", "Show a list of linked devices."},
	{"listGroups", "List group information including names, ids, active status, blocked status and members"},
	{"listIdentities", "List all known identity keys and their trust status, fingerprint and safety number."},
	{"listStickerPacks", "Show a list of known sticker packs."},
	{"quitGroup", "Send a quit group message to all group members and remove self from member list."},
	{"receive", "Query the server for new messages."},
	{"register", "Register a phone number with SMS or voice verification."},
	{"rejectCall", "Reject an incoming voice call."},
	{"remoteDelete", "Remotely delete a previously sent message."},
	{"removeContact", "Remove the details of a given contact"},
	{"removeDevice", "Remove a linked device."},
	{"removePin", "Remove the registration lock pin."},
	{"send", "Send a message to another user or group."},
	{"sendAdminDelete", "Send admin delete message for a previously received or sent message."},
	{"sendContacts", "Send a synchronization message with the local contacts list to all linked devices."},
	{"sendMessageRequestResponse", "Send response to a message request to linked devices."},
	{"sendPaymentNotification", "Send a payment notification."},
	{"sendPinMessage", "Send pin message for a previously received or sent message."},
	{"sendPollCreate", "Create a poll and send it to another user or group."},
	{"sendPollTerminate", "Terminate a poll and send it to another user or group."},
	{"sendPollVote", "Vote on a poll and send it to another user or group."},
	{"sendReaction", "Send reaction to a previously received or sent message."},
	{"sendReceipt", "Send a read or viewed receipt to a previously received message."},
	{"sendStory", "Post a story to your Story."},
	{"sendSyncRequest", "Send a synchronization request message to primary device (for group, contacts, ...)."},
	{"sendTyping", "Send typing message to trigger a typing indicator for the recipient. Indicator will be shown for 15seconds unless a typing STOP message is sent first."},
	{"sendUnpinMessage", "Send unpin message for a previously received or sent message."},
	{"setPin", "Set a registration lock pin, to prevent others from registering this number."},
	{"startCall", "Start an outgoing voice call."},
	{"startChangeNumber", "Change account to a new phone number with SMS or voice verification."},
	{"submitRateLimitChallenge", "Submit a captcha challenge to lift the rate limit. This command should only be necessary when sending fails with a proof required error."},
	{"trust", "Set the trust level of a given number."},
	{"unblock", "Unblock the given contacts or groups (messages will be received again)"},
	{"unregister", "Unregister the current device from the signal server."},
	{"updateAccount", "Update the account attributes on the signal server."},
	{"updateConfiguration", "Update signal configs and sync them to linked devices."},
	{"updateContact", "Update the details of a given contact"},
	{"updateDevice", "Update a linked device."},
	{"updateGroup", "Create or update a group."},
	{"updateProfile", "Set a name, about and avatar image for the user profile"},
	{"uploadStickerPack", "Upload a new sticker pack, consisting of a manifest file and the stickers images."},
	{"verify", "Verify the number using the code received via SMS or voice."},
	{"version", ""},
}

// globalDefaults are the option defaults, possibly overridden by config
// files (index.md §7).
type globalDefaults struct {
	verbose           int
	scrubLog          bool
	dataDir           string
	dataDirConfigured bool
	busName           string
	dbus              bool
	dbusSystem        bool
	output            string // "" when unset
	serviceEnv        string
	trustNew          string
	disableSendLog    bool
	logFile           string
}

func newParser(d globalDefaults) *argparse.Parser {
	dbus := &argparse.Arg{Names: []string{"--dbus"}, Action: argparse.StoreTrue, Help: "Make request via user dbus.", Default: d.dbus}
	dbusSystem := &argparse.Arg{Names: []string{"--dbus-system"}, Action: argparse.StoreTrue, Help: "Make request via system dbus.", Default: d.dbusSystem}
	var output any
	if d.output != "" {
		output = d.output
	}
	var dataDirDefault any
	if d.dataDirConfigured {
		dataDirDefault = d.dataDir
	}
	var logFile any
	if d.logFile != "" {
		logFile = d.logFile
	}
	p := &argparse.Parser{
		Prog:          "signal-cli",
		ShowDefaults:  true,
		CommandsTitle: "subcommands",
		Description:   description,
		Epilog:        epilog,
		CommandDest:   "command",
		Args: []*argparse.Arg{
			helpArg(),
			{Names: []string{"--version"}, Action: argparse.Version, Help: "Show package version."},
			{Names: []string{"-v", "--verbose"}, Action: argparse.Count, Help: "Raise log level and include lib signal logs. Specify multiple times for even more logs.", Default: d.verbose},
			{Names: []string{"--log-file"}, Dest: "log-file", Metavars: []string{"LOG_FILE"}, Help: "Write log output to the given file. If --verbose is also given, the detailed logs will only be written to the log file.", Default: logFile},
			{Names: []string{"--scrub-log"}, Action: argparse.StoreTrue, Help: "Scrub possibly sensitive information from the log, like phone numbers and UUIDs.", Default: d.scrubLog},
			{Names: []string{"-d", "--data-dir", "-c", "--config"}, Dest: "data-dir", Metavars: []string{"DATA_DIR"}, Help: "Set the path where to store data (Default: $XDG_DATA_HOME/signal-cli , $HOME/.local/share/signal-cli).", Default: dataDirDefault},
			{Names: []string{"-a", "--account", "-u", "--username"}, Dest: "account", Metavars: []string{"ACCOUNT"}, Help: "Specify your phone number, that will be your identifier."},
			dbus,
			dbusSystem,
			{Names: []string{"--bus-name"}, Dest: "bus-name", Metavars: []string{"GLOBAL-BUS-NAME"}, Help: "Specify the D-Bus bus name to connect to.", Default: d.busName},
			{Names: []string{"-o", "--output"}, Dest: "output", Kind: argparse.Enum, Choices: []string{"plain-text", "json"}, Help: "Choose to output in plain text or JSON", Default: output},
			{Names: []string{"--service-environment"}, Dest: "service-environment", Kind: argparse.Enum, Choices: []string{"live", "staging"}, Help: "Choose the server environment to use.", Default: d.serviceEnv},
			{Names: []string{"--trust-new-identities"}, Dest: "trust-new-identities", Kind: argparse.Enum, Choices: []string{"always", "on-first-use", "never"}, Help: "Choose when to trust new identities.", Default: d.trustNew},
			{Names: []string{"--disable-send-log"}, Action: argparse.StoreTrue, Help: "Disable message send log (for resending messages that recipient couldn't decrypt)", Default: d.disableSendLog},
		},
		Groups: []*argparse.Group{{Args: []*argparse.Arg{dbus, dbusSystem}}},
	}
	for _, c := range commandHelp {
		p.Commands = append(p.Commands, &argparse.Command{Name: c[0], Help: c[1], Parser: commandParser(c[0])})
	}
	p.Link()
	return p
}

// commandParser returns a subcommand's parser. Commands signal-cli-go
// does not implement yet get only -h.
func commandParser(name string) *argparse.Parser {
	p := &argparse.Parser{Prog: "signal-cli", Args: []*argparse.Arg{helpArg()}}
	switch name {
	case "link":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"-n", "--name"}, Dest: "name", Metavars: []string{"NAME"}, Help: "Specify a name to describe this new device."})
	case "register":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-v", "--voice"}, Action: argparse.StoreTrue, Help: "The verification should be done over voice, not SMS."},
			&argparse.Arg{Names: []string{"--captcha"}, Dest: "captcha", Metavars: []string{"CAPTCHA"}, Help: "The captcha token, required if registration failed with a captcha required error."},
			&argparse.Arg{Names: []string{"--reregister"}, Action: argparse.StoreTrue, Help: "Register even if account is already registered"},
		)
	case "verify":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"verification_code"}, Dest: "verification_code", Help: "The verification code you received via sms or voice call."},
			&argparse.Arg{Names: []string{"-p", "--pin"}, Dest: "pin", Metavars: []string{"PIN"}, Help: "The registration lock PIN, that was set by the user (Optional)"},
		)
	case "sendReaction", "remoteDelete", "sendPollCreate", "sendPollVote", "sendPollTerminate", "sendPinMessage", "sendUnpinMessage", "sendAdminDelete", "sendPaymentNotification", "sendMessageRequestResponse":
		p.Args = append(p.Args, featureArgs(name)...)
	case "sendReceipt":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Help: "Specify the sender's phone number."},
			&argparse.Arg{Names: []string{"-t", "--target-timestamp"}, Dest: "target-timestamp", Metavars: []string{"TARGET_TIMESTAMP"}, Kind: argparse.Int64, Nargs: argparse.OneOrMore, Required: true, Help: "Specify the timestamp of the messages for which a receipt should be sent."},
			&argparse.Arg{Names: []string{"--type"}, Dest: "type", Choices: []string{"read", "viewed"}, Help: "Specify the receipt type (default is read receipt)."},
		)
	case "sendTyping":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Nargs: argparse.ZeroOrMore, Help: "Specify the recipients' phone number."},
			&argparse.Arg{Names: []string{"-g", "--group-id", "--group"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Nargs: argparse.ZeroOrMore, Help: "Specify the recipient group ID."},
			&argparse.Arg{Names: []string{"-s", "--stop"}, Action: argparse.StoreTrue, Help: "Send a typing STOP message."},
		)
	case "trust":
		all := &argparse.Arg{Names: []string{"-a", "--trust-all-known-keys"}, Action: argparse.StoreTrue, Help: "Trust all known keys of this user, only use this for testing."}
		verified := &argparse.Arg{Names: []string{"-v", "--verified-safety-number", "--verified-fingerprint"}, Dest: "verified_safety_number", Metavars: []string{"VERIFIED_SAFETY_NUMBER"},
			Help: "Specify the safety number of the key, only use this option if you have verified the safety number."}
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Metavars: []string{"recipient"}, Help: "Specify the phone number, for which to set the trust."},
			all, verified)
		p.Groups = []*argparse.Group{{Args: []*argparse.Arg{all, verified}}}
	case "updateProfile":
		avatar := &argparse.Arg{Names: []string{"--avatar"}, Dest: "avatar", Metavars: []string{"AVATAR"}, Help: "Path to new profile avatar"}
		remove := &argparse.Arg{Names: []string{"--remove-avatar"}, Action: argparse.StoreTrue}
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"--given-name", "--name"}, Dest: "given-name", Metavars: []string{"GIVEN_NAME"}, Help: "New profile (given) name"},
			&argparse.Arg{Names: []string{"--family-name"}, Dest: "family-name", Metavars: []string{"FAMILY_NAME"}, Help: "New profile family name (optional)"},
			&argparse.Arg{Names: []string{"--about"}, Dest: "about", Metavars: []string{"ABOUT"}, Help: "New profile about text"},
			&argparse.Arg{Names: []string{"--about-emoji"}, Dest: "about-emoji", Metavars: []string{"ABOUT_EMOJI"}, Help: "New profile about emoji"},
			&argparse.Arg{Names: []string{"--mobile-coin-address", "--mobilecoin-address"}, Dest: "mobile-coin-address", Metavars: []string{"MOBILE_COIN_ADDRESS"}, Help: "New MobileCoin address (Base64 encoded public address)"},
			avatar, remove)
		p.Groups = []*argparse.Group{{Args: []*argparse.Arg{avatar, remove}}}
	case "getAvatar":
		c := &argparse.Arg{Names: []string{"-c", "--contact"}, Dest: "contact", Metavars: []string{"CONTACT"}, Help: "Get a contact avatar"}
		pr := &argparse.Arg{Names: []string{"-p", "--profile"}, Dest: "profile", Metavars: []string{"PROFILE"}, Help: "Get a profile avatar"}
		g := &argparse.Arg{Names: []string{"-g", "--group-id"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Help: "Get a group avatar"}
		p.Args = append(p.Args, c, pr, g)
		p.Groups = []*argparse.Group{{Args: []*argparse.Arg{c, pr, g}, Required: true}}
	case "listContacts":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Nargs: argparse.ZeroOrMore, Help: "Specify one ore more phone numbers to show."},
			&argparse.Arg{Names: []string{"-a", "--all-recipients"}, Action: argparse.StoreTrue, Help: "Include all known recipients, not only contacts."},
			&argparse.Arg{Names: []string{"--blocked"}, Dest: "blocked", Kind: argparse.Boolean, Help: "Specify if only blocked or unblocked contacts should be shown (default: all contacts)"},
			&argparse.Arg{Names: []string{"--name"}, Dest: "name", Metavars: []string{"NAME"}, Help: "Find contacts with the given contact or profile name."},
			&argparse.Arg{Names: []string{"--detailed"}, Action: argparse.StoreTrue, Help: "List the contacts with more details. If output=json, then this is always set"},
			&argparse.Arg{Names: []string{"--internal"}, Action: argparse.StoreTrue, Help: "Include internal information that's normally not user visible"})
	case "updateContact":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Help: "Contact number"},
			&argparse.Arg{Names: []string{"-n", "--name"}, Dest: "name", Metavars: []string{"NAME"}, Help: "New contact name"},
			&argparse.Arg{Names: []string{"--given-name"}, Dest: "given-name", Metavars: []string{"GIVEN_NAME"}, Help: "New system given name"},
			&argparse.Arg{Names: []string{"--family-name"}, Dest: "family-name", Metavars: []string{"FAMILY_NAME"}, Help: "New system family name"},
			&argparse.Arg{Names: []string{"--nick-given-name"}, Dest: "nick-given-name", Metavars: []string{"NICK_GIVEN_NAME"}, Help: "New nick given name"},
			&argparse.Arg{Names: []string{"--nick-family-name"}, Dest: "nick-family-name", Metavars: []string{"NICK_FAMILY_NAME"}, Help: "New nick family name"},
			&argparse.Arg{Names: []string{"--note"}, Dest: "note", Metavars: []string{"NOTE"}, Help: "New note"},
			&argparse.Arg{Names: []string{"-e", "--expiration"}, Dest: "expiration", Metavars: []string{"EXPIRATION"}, Kind: argparse.Int32, Help: "Set expiration time of messages (seconds)"})
	case "removeContact":
		hide := &argparse.Arg{Names: []string{"--hide"}, Action: argparse.StoreTrue, Help: "Hide the contact in the contact list, but keep the data."}
		forget := &argparse.Arg{Names: []string{"--forget"}, Action: argparse.StoreTrue, Help: "Delete all data associated with this contact, including identity keys and sessions."}
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Help: "Contact number"}, hide, forget)
		p.Groups = []*argparse.Group{{Args: []*argparse.Arg{hide, forget}}}
	case "block", "unblock":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Nargs: argparse.ZeroOrMore, Help: "Contact number"},
			&argparse.Arg{Names: []string{"-g", "--group-id", "--group"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Nargs: argparse.ZeroOrMore, Help: "Group ID"})
	case "getUserStatus":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Nargs: argparse.ZeroOrMore, Help: "Phone number"},
			&argparse.Arg{Names: []string{"--username"}, Dest: "username", Metavars: []string{"USERNAME"}, Nargs: argparse.ZeroOrMore, Help: "Specify the recipient username or username link."})
	case "listIdentities":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"-n", "--number"}, Dest: "number", Metavars: []string{"NUMBER"}, Help: "Only show identity keys for the given phone number."})
	case "updateAccount":
		user := &argparse.Arg{Names: []string{"-u", "--username"}, Dest: "username", Metavars: []string{"USERNAME"}, Help: "Specify a username that can then be used to contact this account."}
		del := &argparse.Arg{Names: []string{"--delete-username"}, Action: argparse.StoreTrue, Help: "Delete the username associated with this account."}
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-n", "--device-name"}, Dest: "device-name", Metavars: []string{"DEVICE_NAME"}, Help: "Specify a name to describe this device."},
			&argparse.Arg{Names: []string{"--unrestricted-unidentified-sender"}, Dest: "unrestricted-unidentified-sender", Kind: argparse.Boolean, Help: "Enable if anyone should be able to send you unidentified sender messages."},
			&argparse.Arg{Names: []string{"--discoverable-by-number"}, Dest: "discoverable-by-number", Kind: argparse.Boolean, Help: "Enable/disable if the account should be discoverable by phone number"},
			&argparse.Arg{Names: []string{"--number-sharing"}, Dest: "number-sharing", Kind: argparse.Boolean, Help: "Indicates if Signal should share its phone number when sending a message."},
			user, del)
		p.Groups = []*argparse.Group{{Args: []*argparse.Arg{user, del}}}
	case "setPin":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"pin"}, Dest: "pin", Help: "The registration lock PIN, that will be required for new registrations (resets after 7 days of inactivity)"})
	case "removePin":
	case "listDevices":
	case "addDevice":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"--uri"}, Dest: "uri", Metavars: []string{"URI"}, Required: true, Help: "Specify the uri contained in the QR code shown by the new device."})
	case "removeDevice":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"-d", "--device-id", "--deviceId"}, Dest: "device-id", Metavars: []string{"DEVICE_ID"}, Kind: argparse.Int32, Required: true,
			Help: "Specify the device you want to remove. Use listDevices to see the deviceIds."})
	case "updateDevice":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-d", "--device-id", "--deviceId"}, Dest: "device-id", Metavars: []string{"DEVICE_ID"}, Kind: argparse.Int32, Required: true,
				Help: "Specify the device you want to update. Use listDevices to see the deviceIds."},
			&argparse.Arg{Names: []string{"-n", "--device-name"}, Dest: "device-name", Metavars: []string{"DEVICE_NAME"}, Required: true, Help: "Specify a name to describe the given device."})
	case "unregister":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"--delete-account"}, Action: argparse.StoreTrue,
			Help: "Delete account completely from server. CAUTION: Only do this if you won't use this number again!"})
	case "sendStory":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-a", "--attachment"}, Dest: "attachment", Metavars: []string{"ATTACHMENT"}, Required: true, Help: "Specify the file path to the image or video to post as a story."},
			&argparse.Arg{Names: []string{"--no-replies"}, Action: argparse.StoreTrue, Help: "Disable replies on this story."},
			&argparse.Arg{Names: []string{"-g", "--group-id"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Help: "Specify a group to post the story to. Without this, posts to My Story."})
	case "addStickerPack":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"--uri"}, Dest: "uri", Metavars: []string{"URI"}, Nargs: argparse.OneOrMore, Required: true,
			Help: "Specify the uri of the sticker pack. (e.g. https://signal.art/addstickers/#pack_id=XXX&pack_key=XXX)"})
	case "getSticker":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"--pack-id"}, Dest: "pack-id", Metavars: []string{"PACK_ID"}, Required: true, Help: "The ID of the sticker pack."},
			&argparse.Arg{Names: []string{"--sticker-id"}, Dest: "sticker-id", Metavars: []string{"STICKER_ID"}, Kind: argparse.Int32, Required: true, Help: "The ID of the sticker."})
	case "listStickerPacks":
	case "uploadStickerPack":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"path"}, Dest: "path", Metavars: []string{"path"},
			Help: "The path of the manifest.json or a zip file containing the sticker pack you wish to upload."})
	case "updateConfiguration":
		boolArg := func(name, help string) *argparse.Arg {
			return &argparse.Arg{Names: []string{name}, Dest: strings.TrimPrefix(name, "--"), Kind: argparse.Boolean, Help: help}
		}
		p.Args = append(p.Args,
			boolArg("--read-receipts", "Indicates if Signal should send read receipts."),
			boolArg("--unidentified-delivery-indicators", "Indicates if Signal should show unidentified delivery indicators."),
			boolArg("--typing-indicators", "Indicates if Signal should send/show typing indicators."),
			boolArg("--link-previews", "Indicates if Signal should generate link previews."))
	case "updateGroup":
		many := func(names []string, dest, metavar, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Dest: dest, Metavars: []string{metavar}, Nargs: argparse.ZeroOrMore, Help: help}
		}
		perm := func(name, help string) *argparse.Arg {
			return &argparse.Arg{Names: []string{name}, Dest: strings.TrimPrefix(name, "--"), Choices: []string{"every-member", "only-admins"}, Help: help}
		}
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-g", "--group-id", "--group"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Help: "Specify the group ID."},
			&argparse.Arg{Names: []string{"-n", "--name"}, Dest: "name", Metavars: []string{"NAME"}, Help: "Specify the new group name."},
			&argparse.Arg{Names: []string{"-d", "--description"}, Dest: "description", Metavars: []string{"DESCRIPTION"}, Help: "Specify the new group description."},
			&argparse.Arg{Names: []string{"-a", "--avatar"}, Dest: "avatar", Metavars: []string{"AVATAR"}, Help: "Specify a new group avatar image file"},
			many([]string{"-m", "--member"}, "member", "MEMBER", "Specify one or more members to add to the group"),
			many([]string{"-r", "--remove-member"}, "remove-member", "REMOVE_MEMBER", "Specify one or more members to remove from the group"),
			many([]string{"--admin"}, "admin", "ADMIN", "Specify one or more members to make a group admin"),
			many([]string{"--remove-admin"}, "remove-admin", "REMOVE_ADMIN", "Specify one or more members to remove group admin privileges"),
			many([]string{"--ban"}, "ban", "BAN", "Specify one or more members to ban from joining the group"),
			many([]string{"--unban"}, "unban", "UNBAN", "Specify one or more members to remove from the ban list"),
			&argparse.Arg{Names: []string{"--reset-link"}, Action: argparse.StoreTrue, Help: "Reset group link and create new link password"},
			&argparse.Arg{Names: []string{"--link"}, Dest: "link", Choices: []string{"enabled", "enabled-with-approval", "disabled"}, Help: "Set group link state, with or without admin approval"},
			perm("--set-permission-add-member", "Set permission to add new group members"),
			perm("--set-permission-edit-details", "Set permission to edit group details"),
			perm("--set-permission-send-messages", "Set permission to send messages"),
			&argparse.Arg{Names: []string{"-e", "--expiration"}, Dest: "expiration", Metavars: []string{"EXPIRATION"}, Kind: argparse.Int32, Help: "Set expiration time of messages (seconds)"},
			&argparse.Arg{Names: []string{"--member-label-emoji"}, Dest: "member-label-emoji", Metavars: []string{"MEMBER_LABEL_EMOJI"}, Help: "Specify the emoji for the member label."},
			&argparse.Arg{Names: []string{"--member-label"}, Dest: "member-label", Metavars: []string{"MEMBER_LABEL"}, Help: "Specify the string for the member label."})
	case "joinGroup":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"--uri"}, Dest: "uri", Metavars: []string{"URI"}, Required: true, Help: "Specify the uri with the group invitation link."})
	case "quitGroup":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-g", "--group-id", "--group"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Required: true, Help: "Specify the recipient group ID."},
			&argparse.Arg{Names: []string{"--delete"}, Action: argparse.StoreTrue, Help: "Delete local group data completely after quitting group."},
			&argparse.Arg{Names: []string{"--admin"}, Dest: "admin", Metavars: []string{"ADMIN"}, Nargs: argparse.ZeroOrMore, Help: "Specify one or more members to make a group admin, required if you're currently the only admin."})
	case "listGroups":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-d", "--detailed"}, Action: argparse.StoreTrue, Help: "List the members and group invite links of each group. If output=json, then this is always set"},
			&argparse.Arg{Names: []string{"-g", "--group-id"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Nargs: argparse.ZeroOrMore, Help: "Specify one or more group IDs to show."})
	case "getAttachment":
		recipient := &argparse.Arg{Names: []string{"--recipient"}, Dest: "recipient", Metavars: []string{"RECIPIENT"}, Help: "Sender of the attachment"}
		group := &argparse.Arg{Names: []string{"-g", "--group-id"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Help: "Group in which the attachment was received"}
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"--id"}, Dest: "id", Metavars: []string{"ID"}, Required: true, Help: "The ID of the attachment file."},
			recipient, group)
		p.Groups = []*argparse.Group{{Args: []*argparse.Arg{recipient, group}, Required: true}}
	case "deleteLocalAccountData":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"--ignore-registered"}, Action: argparse.StoreTrue, Help: "Delete the account data even though the account is still registered on the Signal servers."})
	case "acceptCall", "hangupCall", "rejectCall":
		verb := map[string]string{"acceptCall": "accept", "hangupCall": "hang up", "rejectCall": "reject"}[name]
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"--call-id"}, Dest: "call-id", Metavars: []string{"CALL_ID"}, Kind: argparse.Int64, Required: true, Help: "The call ID to " + verb + "."})
	case "startCall":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Help: "Specify the recipient's phone number or UUID."})
	case "daemon":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"--dbus"}, Action: argparse.StoreTrue, Help: "Expose a DBus interface on the user bus."},
			&argparse.Arg{Names: []string{"--dbus-system", "--system"}, Action: argparse.StoreTrue, Help: "Expose a DBus interface on the system bus."},
			&argparse.Arg{Names: []string{"--bus-name"}, Dest: "bus-name", Metavars: []string{"BUS_NAME"}, Help: "Specify the D-Bus bus name to connect to."},
			&argparse.Arg{Names: []string{"--socket"}, Dest: "socket", Metavars: []string{"SOCKET"}, Nargs: argparse.Optional, Const: "", Help: "Expose a JSON-RPC interface on a UNIX socket (default $XDG_RUNTIME_DIR/signal-cli/socket)."},
			&argparse.Arg{Names: []string{"--tcp"}, Dest: "tcp", Metavars: []string{"TCP"}, Nargs: argparse.Optional, Const: "localhost:7583", Help: "Expose a JSON-RPC interface on a TCP socket (default localhost:7583)."},
			&argparse.Arg{Names: []string{"--http"}, Dest: "http", Metavars: []string{"HTTP"}, Nargs: argparse.Optional, Const: "localhost:8080", Help: "Expose a JSON-RPC interface as http endpoint (default localhost:8080)."},
			&argparse.Arg{Names: []string{"--no-receive-stdout"}, Action: argparse.StoreTrue, Help: "Don’t print received messages to stdout."},
			receiveModeArg(),
		)
		p.Args = append(p.Args, receiveFlags()...)
	case "jsonRpc":
		p.Args = append(p.Args, receiveFlags()...)
		p.Args = append(p.Args, receiveModeArg())
	case "receive":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-t", "--timeout"}, Dest: "timeout", Metavars: []string{"TIMEOUT"}, Kind: argparse.Double, Help: "Number of seconds to wait for new messages (negative values disable timeout)", Default: 3.0},
			&argparse.Arg{Names: []string{"--max-messages"}, Dest: "max-messages", Metavars: []string{"MAX_MESSAGES"}, Kind: argparse.Int32, Help: "Maximum number of messages to receive, before returning.", Default: -1},
			&argparse.Arg{Names: []string{"--ignore-attachments"}, Action: argparse.StoreTrue, Help: "Don’t download attachments of received messages."},
			&argparse.Arg{Names: []string{"--ignore-stories"}, Action: argparse.StoreTrue, Help: "Don’t receive story messages from the server."},
			&argparse.Arg{Names: []string{"--ignore-avatars"}, Action: argparse.StoreTrue, Help: "Don't download avatars of received messages."},
			&argparse.Arg{Names: []string{"--ignore-stickers"}, Action: argparse.StoreTrue, Help: "Don't download sticker packs of received messages."},
			&argparse.Arg{Names: []string{"--send-read-receipts"}, Action: argparse.StoreTrue, Help: "Send read receipts for all incoming data messages (in addition to the default delivery receipts)"},
		)
	case "send":
		many := func(names []string, dest, metavar, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Dest: dest, Metavars: []string{metavar}, Nargs: argparse.ZeroOrMore, Help: help}
		}
		one := func(names []string, dest, metavar, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Dest: dest, Metavars: []string{metavar}, Help: help}
		}
		int64Arg := func(names []string, dest, metavar, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Dest: dest, Metavars: []string{metavar}, Kind: argparse.Int64, Help: help}
		}
		flag := func(names []string, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Action: argparse.StoreTrue, Help: help}
		}
		message := one([]string{"-m", "--message"}, "message", "MESSAGE", "Specify the message to be sent.")
		fromStdin := flag([]string{"--message-from-stdin"}, "Read the message from standard input.")
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Metavars: []string{"recipient"}, Nargs: argparse.ZeroOrMore, Help: "Specify the recipients' phone number."},
			many([]string{"-g", "--group-id", "--group"}, "group-id", "GROUP_ID", "Specify the recipient group ID."),
			many([]string{"-u", "--username"}, "username", "USERNAME", "Specify the recipient username or username link."),
			flag([]string{"--note-to-self"}, "Send the message to self."),
			flag([]string{"--notify-self"}, "If self is part of recipients/groups send a normal message, not a sync message."),
			message,
			fromStdin,
			many([]string{"-a", "--attachment"}, "attachment", "ATTACHMENT", "Add an attachment. Can be either a file path or a data URI. Data URI encoded attachments must follow the RFC 2397. Additionally a file name can be added, e.g. data:<MIME-TYPE>;filename=<FILENAME>;base64,<BASE64 ENCODED DATA>."),
			flag([]string{"--view-once"}, "Send the message as a view once message"),
			flag([]string{"-e", "--end-session", "--endsession"}, "Clear session state and send end session message."),
			many([]string{"--mention"}, "mention", "MENTION", "Mention another group member (syntax: start:length:recipientNumber). Unit of start and length is UTF-16 code units, NOT Unicode code points."),
			many([]string{"--text-style"}, "text-style", "TEXT_STYLE", "Style parts of the message text (syntax: start:length:STYLE). Unit of start and length is UTF-16 code units, NOT Unicode code points."),
			int64Arg([]string{"--quote-timestamp"}, "quote-timestamp", "QUOTE_TIMESTAMP", "Specify the timestamp of a previous message with the recipient or group to add a quote to the new message."),
			one([]string{"--quote-author"}, "quote-author", "QUOTE_AUTHOR", "Specify the number of the author of the original message."),
			one([]string{"--quote-message"}, "quote-message", "QUOTE_MESSAGE", "Specify the message of the original message."),
			many([]string{"--quote-mention"}, "quote-mention", "QUOTE_MENTION", "Quote with mention of another group member (syntax: start:length:recipientNumber)"),
			many([]string{"--quote-attachment"}, "quote-attachment", "QUOTE_ATTACHMENT", "Specify the attachments of the original message (syntax: contentType[:filename[:previewFile]]), e.g. 'audio/aac' or 'image/png:test.png:/tmp/preview.jpg'."),
			many([]string{"--quote-text-style"}, "quote-text-style", "QUOTE_TEXT_STYLE", "Quote with style parts of the message text (syntax: start:length:STYLE)"),
			one([]string{"--sticker"}, "sticker", "STICKER", "Send a sticker (syntax: stickerPackId:stickerId)"),
			one([]string{"--preview-url"}, "preview-url", "PREVIEW_URL", "Specify the url for the link preview (the same url must also appear in the message body)."),
			one([]string{"--preview-title"}, "preview-title", "PREVIEW_TITLE", "Specify the title for the link preview (mandatory)."),
			one([]string{"--preview-description"}, "preview-description", "PREVIEW_DESCRIPTION", "Specify the description for the link preview (optional)."),
			one([]string{"--preview-image"}, "preview-image", "PREVIEW_IMAGE", "Specify the image file for the link preview (optional)."),
			int64Arg([]string{"--story-timestamp"}, "story-timestamp", "STORY_TIMESTAMP", "Specify the timestamp of a story to reply to."),
			one([]string{"--story-author"}, "story-author", "STORY_AUTHOR", "Specify the number of the author of the story."),
			int64Arg([]string{"--edit-timestamp"}, "edit-timestamp", "EDIT_TIMESTAMP", "Specify the timestamp of a previous message with the recipient or group to send an edited message."),
			flag([]string{"--no-urgent"}, "Send the message without the urgent flag, so no push notification is triggered for the recipient. The message will still be delivered in real-time if the recipient's app is active."),
			flag([]string{"--voice-note"}, "Mark audio attachments as voice notes. Voice notes are displayed inline in Signal clients."),
		)
		p.Groups = []*argparse.Group{{Args: []*argparse.Arg{message, fromStdin}}}
	}
	return p
}

// featureArgs are the arguments of the data-message feature commands,
// transcribed from their CLI specs and recorded help.
func featureArgs(name string) []*argparse.Arg {
	recipients := &argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Nargs: argparse.ZeroOrMore, Help: "Specify the recipients' phone number."}
	groups := &argparse.Arg{Names: []string{"-g", "--group-id", "--group"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Nargs: argparse.ZeroOrMore, Help: "Specify the recipient group ID."}
	usernames := &argparse.Arg{Names: []string{"-u", "--username"}, Dest: "username", Metavars: []string{"USERNAME"}, Nargs: argparse.ZeroOrMore, Help: "Specify the recipient username or username link."}
	noteToSelf := func(help string) *argparse.Arg {
		return &argparse.Arg{Names: []string{"--note-to-self"}, Action: argparse.StoreTrue, Help: help}
	}
	notifySelf := &argparse.Arg{Names: []string{"--notify-self"}, Action: argparse.StoreTrue, Help: "If self is part of recipients/groups send a normal message, not a sync message."}
	req := func(names []string, dest, metavar, help string, kind argparse.Kind) *argparse.Arg {
		return &argparse.Arg{Names: names, Dest: dest, Metavars: []string{metavar}, Kind: kind, Required: true, Help: help}
	}
	opt := func(names []string, dest, metavar, help string, kind argparse.Kind) *argparse.Arg {
		return &argparse.Arg{Names: names, Dest: dest, Metavars: []string{metavar}, Kind: kind, Help: help}
	}
	flag := func(names []string, help string) *argparse.Arg {
		return &argparse.Arg{Names: names, Action: argparse.StoreTrue, Help: help}
	}
	switch name {
	case "sendReaction":
		return []*argparse.Arg{recipients, groups, usernames, noteToSelf("Send the reaction to self."), notifySelf,
			req([]string{"-e", "--emoji"}, "emoji", "EMOJI", "Specify the emoji, should be a single unicode grapheme cluster.", argparse.String),
			req([]string{"-a", "--target-author"}, "target-author", "TARGET_AUTHOR", "Specify the number of the author of the message to which to react.", argparse.String),
			req([]string{"-t", "--target-timestamp"}, "target-timestamp", "TARGET_TIMESTAMP", "Specify the timestamp of the message to which to react.", argparse.Int64),
			flag([]string{"-r", "--remove"}, "Remove a reaction."),
			flag([]string{"--story"}, "React to a story instead of a normal message")}
	case "remoteDelete":
		return []*argparse.Arg{recipients,
			req([]string{"-t", "--target-timestamp"}, "target-timestamp", "TARGET_TIMESTAMP", "Specify the timestamp of the message to delete.", argparse.Int64),
			groups, usernames, noteToSelf("")}
	case "sendPollCreate":
		return []*argparse.Arg{recipients, groups, usernames, noteToSelf("Send the message to self"), notifySelf,
			req([]string{"-q", "--question"}, "question", "QUESTION", "Specify the poll question.", argparse.String),
			flag([]string{"--no-multi"}, "Allow only one option to be selected by each recipient."),
			{Names: []string{"-o", "--option"}, Dest: "option", Metavars: []string{"OPTION"}, Nargs: argparse.OneOrMore, Required: true, Help: "The options for the poll"}}
	case "sendPollVote":
		return []*argparse.Arg{recipients, groups, usernames, noteToSelf("Send the message to self"), notifySelf,
			opt([]string{"--poll-author"}, "poll-author", "POLL_AUTHOR", "Specify the number of the author of the poll message.", argparse.String),
			req([]string{"--poll-timestamp"}, "poll-timestamp", "POLL_TIMESTAMP", "Specify the timestamp of the original poll message.", argparse.Int64),
			{Names: []string{"-o", "--option"}, Dest: "option", Metavars: []string{"OPTION"}, Kind: argparse.Int32, Nargs: argparse.ZeroOrMore, Help: "The option indexes of the poll to vote for"},
			req([]string{"--vote-count"}, "vote-count", "VOTE_COUNT", "Specify the number of this vote (increase by one for every time you vote).", argparse.Int32)}
	case "sendPollTerminate":
		return []*argparse.Arg{recipients, groups, usernames, noteToSelf("Send the message to self"), notifySelf,
			req([]string{"--poll-timestamp"}, "poll-timestamp", "POLL_TIMESTAMP", "Specify the timestamp of the original poll message.", argparse.Int64)}
	case "sendPinMessage":
		return []*argparse.Arg{recipients, groups, usernames, noteToSelf("Send the pin message to self."), notifySelf,
			{Names: []string{"-d", "--pin-duration"}, Dest: "pin-duration", Metavars: []string{"PIN_DURATION"}, Kind: argparse.Int32, Default: -1, Help: "Specify the pin duration in seconds. Use -1 for forever."},
			req([]string{"-a", "--target-author"}, "target-author", "TARGET_AUTHOR", "Specify the number of the author of the message to pin.", argparse.String),
			req([]string{"-t", "--target-timestamp"}, "target-timestamp", "TARGET_TIMESTAMP", "Specify the timestamp of the message to pin.", argparse.Int64),
			flag([]string{"--story"}, "Pin a story instead of a normal message")}
	case "sendUnpinMessage":
		return []*argparse.Arg{recipients, groups, usernames, noteToSelf("Send the unpin message to self."), notifySelf,
			req([]string{"-a", "--target-author"}, "target-author", "TARGET_AUTHOR", "Specify the number of the author of the message to unpin.", argparse.String),
			req([]string{"-t", "--target-timestamp"}, "target-timestamp", "TARGET_TIMESTAMP", "Specify the timestamp of the message to unpin.", argparse.Int64),
			flag([]string{"--story"}, "Unpin a story instead of a normal message")}
	case "sendAdminDelete":
		return []*argparse.Arg{
			{Names: []string{"-g", "--group-id", "--group"}, Dest: "group-id", Metavars: []string{"GROUP_ID"}, Nargs: argparse.OneOrMore, Help: "Specify the recipient group ID."},
			notifySelf,
			req([]string{"-a", "--target-author"}, "target-author", "TARGET_AUTHOR", "Specify the number of the author of the message to admin delete.", argparse.String),
			req([]string{"-t", "--target-timestamp"}, "target-timestamp", "TARGET_TIMESTAMP", "Specify the timestamp of the message to admin delete.", argparse.Int64),
			flag([]string{"--story"}, "Admin delete a story instead of a normal message")}
	case "sendPaymentNotification":
		return []*argparse.Arg{
			{Names: []string{"recipient"}, Dest: "recipient", Help: "Specify the recipient's phone number."},
			req([]string{"--receipt"}, "receipt", "RECEIPT", "The base64 encoded receipt blob.", argparse.String),
			opt([]string{"--note"}, "note", "NOTE", "Specify a note for the payment notification.", argparse.String)}
	case "sendMessageRequestResponse":
		return []*argparse.Arg{recipients, groups, usernames,
			{Names: []string{"--type"}, Dest: "type", Kind: argparse.Enum, Choices: []string{"accept", "delete"}, Required: true, Help: "Type of message request response"}}
	}
	return nil
}

// receiveFlags are the receive options daemon and jsonRpc share with
// receive.
func receiveFlags() []*argparse.Arg {
	return []*argparse.Arg{
		{Names: []string{"--ignore-attachments"}, Action: argparse.StoreTrue, Help: "Don’t download attachments of received messages."},
		{Names: []string{"--ignore-stories"}, Action: argparse.StoreTrue, Help: "Don’t receive story messages from the server."},
		{Names: []string{"--ignore-avatars"}, Action: argparse.StoreTrue, Help: "Don't download avatars of received messages."},
		{Names: []string{"--ignore-stickers"}, Action: argparse.StoreTrue, Help: "Don't download sticker packs of received messages."},
		{Names: []string{"--send-read-receipts"}, Action: argparse.StoreTrue, Help: "Send read receipts for all incoming data messages (in addition to the default delivery receipts)"},
	}
}

func receiveModeArg() *argparse.Arg {
	return &argparse.Arg{Names: []string{"--receive-mode"}, Dest: "receive-mode", Kind: argparse.Enum, Choices: []string{"on-start", "on-connection", "manual"}, Default: "on-start", Help: "Specify when to start receiving messages."}
}
