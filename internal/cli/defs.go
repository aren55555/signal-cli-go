package cli

import (
	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
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
	verbose        int
	scrubLog       bool
	dataDir        string
	busName        string
	dbus           bool
	dbusSystem     bool
	output         string // "" when unset
	serviceEnv     string
	trustNew       string
	disableSendLog bool
	logFile        string
}

func newParser(d globalDefaults) *argparse.Parser {
	dbus := &argparse.Arg{Names: []string{"--dbus"}, Action: argparse.StoreTrue, Help: "Make request via user dbus.", Default: d.dbus}
	dbusSystem := &argparse.Arg{Names: []string{"--dbus-system"}, Action: argparse.StoreTrue, Help: "Make request via system dbus.", Default: d.dbusSystem}
	var output any
	if d.output != "" {
		output = d.output
	}
	var logFile any
	if d.logFile != "" {
		logFile = d.logFile
	}
	p := &argparse.Parser{
		Prog:        "signal-cli",
		Description: description,
		Epilog:      epilog,
		CommandDest: "command",
		Args: []*argparse.Arg{
			helpArg(),
			{Names: []string{"--version"}, Action: argparse.Version, Help: "Show package version."},
			{Names: []string{"-v", "--verbose"}, Action: argparse.Count, Help: "Raise log level and include lib signal logs. Specify multiple times for even more logs.", Default: d.verbose},
			{Names: []string{"--log-file"}, Dest: "log-file", Metavar: "LOG_FILE", Help: "Write log output to the given file. If --verbose is also given, the detailed logs will only be written to the log file.", Default: logFile},
			{Names: []string{"--scrub-log"}, Action: argparse.StoreTrue, Help: "Scrub possibly sensitive information from the log, like phone numbers and UUIDs.", Default: d.scrubLog},
			{Names: []string{"-d", "--data-dir", "-c", "--config"}, Dest: "data-dir", Metavar: "DATA_DIR", Help: "Set the path where to store data (Default: $XDG_DATA_HOME/signal-cli , $HOME/.local/share/signal-cli)."},
			{Names: []string{"-a", "--account", "-u", "--username"}, Dest: "account", Metavar: "ACCOUNT", Help: "Specify your phone number, that will be your identifier."},
			dbus,
			dbusSystem,
			{Names: []string{"--bus-name"}, Dest: "bus-name", Metavar: "GLOBAL-BUS-NAME", Help: "Specify the D-Bus bus name to connect to.", Default: d.busName},
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
	return p
}

// commandParser returns a subcommand's parser. Commands signal-cli-go
// does not implement yet get only -h.
func commandParser(name string) *argparse.Parser {
	p := &argparse.Parser{Prog: "signal-cli " + name, Args: []*argparse.Arg{helpArg()}}
	switch name {
	case "link":
		p.Args = append(p.Args, &argparse.Arg{Names: []string{"-n", "--name"}, Dest: "name", Metavar: "NAME", Help: "Specify a name to describe this new device."})
	case "receive":
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"-t", "--timeout"}, Dest: "timeout", Metavar: "TIMEOUT", Kind: argparse.Double, Help: "Number of seconds to wait for new messages (negative values disable timeout)", Default: 3.0},
			&argparse.Arg{Names: []string{"--max-messages"}, Dest: "max-messages", Metavar: "MAX_MESSAGES", Kind: argparse.Int32, Help: "Maximum number of messages to receive, before returning.", Default: -1},
			&argparse.Arg{Names: []string{"--ignore-attachments"}, Action: argparse.StoreTrue, Help: "Don’t download attachments of received messages."},
			&argparse.Arg{Names: []string{"--ignore-stories"}, Action: argparse.StoreTrue, Help: "Don’t receive story messages from the server."},
			&argparse.Arg{Names: []string{"--ignore-avatars"}, Action: argparse.StoreTrue, Help: "Don't download avatars of received messages."},
			&argparse.Arg{Names: []string{"--ignore-stickers"}, Action: argparse.StoreTrue, Help: "Don't download sticker packs of received messages."},
			&argparse.Arg{Names: []string{"--send-read-receipts"}, Action: argparse.StoreTrue, Help: "Send read receipts for all incoming data messages (in addition to the default delivery receipts)"},
		)
	case "send":
		many := func(names []string, dest, metavar, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Dest: dest, Metavar: metavar, Nargs: argparse.ZeroOrMore, Help: help}
		}
		one := func(names []string, dest, metavar, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Dest: dest, Metavar: metavar, Help: help}
		}
		int64Arg := func(names []string, dest, metavar, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Dest: dest, Metavar: metavar, Kind: argparse.Int64, Help: help}
		}
		flag := func(names []string, help string) *argparse.Arg {
			return &argparse.Arg{Names: names, Action: argparse.StoreTrue, Help: help}
		}
		message := one([]string{"-m", "--message"}, "message", "MESSAGE", "Specify the message to be sent.")
		fromStdin := flag([]string{"--message-from-stdin"}, "Read the message from standard input.")
		p.Args = append(p.Args,
			&argparse.Arg{Names: []string{"recipient"}, Dest: "recipient", Metavar: "recipient", Nargs: argparse.ZeroOrMore, Help: "Specify the recipients' phone number."},
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
