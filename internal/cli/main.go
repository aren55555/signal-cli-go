// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
	"github.com/aren55555/signal-cli-go/internal/errcode"
)

// Version is the signal-cli version being reproduced.
const Version = "0.14.7"

// loggingHelp is the early logging parser's help, printed before every
// help output (index.md §4).
const loggingHelp = "usage: signal-cli [-h] [-v] [--log-file LOG_FILE] [--scrub-log]\n\n" +
	"named arguments:\n" +
	"  -h, --help             show this help message and exit\n" +
	"  -v, --verbose\n" +
	"  --log-file LOG_FILE\n" +
	"  --scrub-log\n"

// plainOnly and jsonOnly are the commands with a single output mode
// (index.md §3).
var (
	plainOnly = map[string]bool{"link": true}
	jsonOnly  = map[string]bool{"jsonRpc": true}
)

// Main runs signal-cli-go with args (without the program name) and
// returns the exit code.
func Main(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	e := &env{stdout: stdout, stderr: stderr, stdin: stdin, getenv: getenv}
	width := formatWidth(getenv)
	d := globalDefaults{dataDir: defaultDataDir(getenv), busName: "org.asamk.Signal", serviceEnv: "live", trustNew: "on-first-use"}
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		d.busName = "org.asamk.SignalCli"
	}
	if err := applyConfig(&d, getenv); err != nil {
		return fail(stderr, err)
	}
	p := newParser(d)
	p.Width = width
	showLoggingHelp := loggingPreparse(args)
	r, err := p.Parse(args)
	if err != nil {
		var pe *argparse.Error
		if errors.As(err, &pe) {
			if showLoggingHelp {
				io.WriteString(stdout, loggingHelp)
			}
			io.WriteString(stderr, argparse.FormatError(pe))
			return 1
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	switch {
	case r.Help:
		if showLoggingHelp {
			io.WriteString(stdout, loggingHelp)
		}
		io.WriteString(stdout, argparse.FormatHelp(r.HelpParser))
		return 0
	case r.Version:
		fmt.Fprintf(stdout, "signal-cli %s\n", Version)
		return 0
	}
	g := &globals{
		verbose:        r.Int("verbose"),
		dataDir:        r.String("data-dir"),
		account:        r.String("account"),
		output:         r.String("output"),
		serviceEnv:     r.String("service-environment"),
		trustNew:       r.String("trust-new-identities"),
		disableSendLog: r.Bool("disable_send_log"),
	}
	if g.dataDir == "" {
		g.dataDir = d.dataDir
	}
	cmd := r.Command
	if g.output == "" {
		g.output = d.output
	}
	if g.output == "" {
		g.output = "plain-text"
		if jsonOnly[cmd] {
			g.output = "json"
		}
	}
	if (plainOnly[cmd] && g.output != "plain-text") || (jsonOnly[cmd] && g.output != "json") {
		return fail(stderr, errcode.OutputTypeUnsupported.New(g.output))
	}
	if r.Bool("dbus") || r.Bool("dbus_system") {
		// D-Bus is out of scope; behave as the reference does without a bus.
		detail := "DISPLAY variable not set"
		if r.Bool("dbus_system") {
			detail = "MachineId file can not be found"
		}
		return fail(stderr, errcode.DBusUnavailable.New(detail))
	}
	if cmd == "link" && g.account != "" {
		return fail(stderr, errcode.LinkWithAccount)
	}
	numberResolver = cdsiResolver(e, g)
	defer func() { numberResolver = nil }()
	if err := runCommand(ctx, e, g, cmd, r.Sub); err != nil {
		return fail(stderr, err)
	}
	return 0
}

// fail is the CLI's output edge for errors: the reference message on
// stderr and the exit code.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, errcode.Message(err))
	return errcode.ExitCode(err)
}

// runCommand runs a parsed command; its output goes to e.stdout and its
// diagnostics to e.stderr.
func runCommand(ctx context.Context, e *env, g *globals, cmd string, s *argparse.Result) error {
	stdout, stderr := e.stdout, e.stderr
	var runErr error
	switch cmd {
	case "daemon":
		runErr = runDaemon(ctx, e, g, s)
	case "jsonRpc":
		runErr = runJSONRPC(ctx, e, g, s)
	case "listAccounts":
		runErr = runListAccounts(e, g)
	case "deleteLocalAccountData":
		runErr = runDeleteLocalAccountData(e, g, s.Bool("ignore_registered"))
	case "startChangeNumber":
		runErr = runStartChangeNumber(ctx, e, g, s.String("number"), s.Bool("voice"), s.String("captcha"))
	case "finishChangeNumber":
		runErr = runFinishChangeNumber(ctx, e, g, s.String("number"), s.String("verification-code"))
	case "submitRateLimitChallenge":
		runErr = runSubmitChallenge(ctx, e, g, s.String("challenge"), s.String("captcha"))
	case "listCalls":
		runErr = runListCalls(e, g)
	case "acceptCall", "hangupCall", "rejectCall":
		id, _ := s.Int64("call-id")
		runErr = runCallAction(e, g, cmd, id, s.Has("call-id"))
	case "startCall":
		runErr = runStartCall(g)
	case "version":
		if g.account != "" {
			a, err := selectAccount(g.root(), g.account)
			if err != nil {
				return err
			}
			a.Close()
		}
		if g.json() {
			fmt.Fprintf(stdout, "{\"version\":%q}\n", Version)
		} else {
			fmt.Fprintf(stdout, "signal-cli %s\n", Version)
		}
	case "link":
		name := s.String("name")
		if name == "" {
			name = "cli"
		}
		runErr = runLink(ctx, e, g, name)
	case "send":
		o := fullSendOpts{
			sendOpts: sendOpts{recipients: s.Strings("recipient"), message: s.String("message"), hasMessage: s.Has("message"),
				fromStdin: s.Bool("message_from_stdin"), noteToSelf: s.Bool("note_to_self"), noUrgent: s.Bool("no_urgent")},
			recipients:       recipientsOf(s),
			notifySelf:       s.Bool("notify_self"),
			viewOnce:         s.Bool("view_once"),
			endSes:           s.Bool("end_session"),
			mentions:         s.Strings("mention"),
			styles:           s.Strings("text-style"),
			quoteAuthor:      s.String("quote-author"),
			quoteMessage:     s.String("quote-message"),
			hasQuoteMessage:  s.Has("quote-message"),
			quoteMentions:    s.Strings("quote-mention"),
			quoteStyles:      s.Strings("quote-text-style"),
			quoteAttachments: s.Strings("quote-attachment"),
			sticker:          s.String("sticker"),
			previewURL:       s.String("preview-url"),
			hasPreview:       s.Has("preview-url"),
			previewTitle:     s.String("preview-title"),
			previewDesc:      s.String("preview-description"),
			storyAuthor:      s.String("story-author"),
			attachments:      s.Strings("attachment"),
			previewImage:     s.String("preview-image"),
			voiceNote:        s.Bool("voice_note"),
		}
		o.quoteTS, o.hasQuote = s.Int64("quote-timestamp")
		o.storyTS, o.hasStory = s.Int64("story-timestamp")
		o.editTS, o.hasEdit = s.Int64("edit-timestamp")
		runErr = runSendFull(ctx, e, g, o)
	case "sendReaction":
		ts, _ := s.Int64("target-timestamp")
		runErr = runSendReaction(ctx, e, g, recipientsOf(s), s.Bool("notify_self"), s.String("emoji"), s.String("target-author"), ts, s.Bool("remove"), s.Bool("story"))
	case "remoteDelete":
		ts, _ := s.Int64("target-timestamp")
		runErr = runRemoteDelete(ctx, e, g, recipientsOf(s), ts)
	case "sendPollCreate":
		runErr = runPollCreate(ctx, e, g, recipientsOf(s), s.Bool("notify_self"), s.String("question"), s.Bool("no_multi"), s.Strings("option"))
	case "sendPollVote":
		ts, _ := s.Int64("poll-timestamp")
		runErr = runPollVote(ctx, e, g, recipientsOf(s), s.Bool("notify_self"), s.String("poll-author"), ts, s.Strings("option"), s.Int("vote-count"))
	case "sendPollTerminate":
		ts, _ := s.Int64("poll-timestamp")
		runErr = runPollTerminate(ctx, e, g, recipientsOf(s), s.Bool("notify_self"), ts)
	case "sendPinMessage", "sendUnpinMessage":
		ts, _ := s.Int64("target-timestamp")
		d := -1
		if s.Has("pin-duration") {
			d = s.Int("pin-duration")
		}
		runErr = runPin(ctx, e, g, recipientsOf(s), s.Bool("notify_self"), s.String("target-author"), ts, s.Bool("story"), cmd == "sendUnpinMessage", d)
	case "sendAdminDelete":
		ts, _ := s.Int64("target-timestamp")
		runErr = runAdminDelete(ctx, e, g, s.Strings("group-id"), s.Bool("notify_self"), s.String("target-author"), ts, s.Bool("story"))
	case "sendPaymentNotification":
		runErr = runPaymentNotification(ctx, e, g, s.String("recipient"), s.String("receipt"), s.String("note"), s.Has("note"))
	case "sendMessageRequestResponse":
		runErr = runMessageRequestResponse(ctx, e, g, recipientsOf(s), s.String("type"))
	case "sendTyping":
		runErr = runSendTyping(ctx, e, g, s.Strings("recipient"), s.Strings("group-id"), s.Bool("stop"))
	case "sendReceipt":
		var tss []uint64
		for _, x := range s.Strings("target-timestamp") {
			v, _ := strconv.ParseUint(x, 10, 64)
			tss = append(tss, v)
		}
		runErr = runSendReceipt(ctx, e, g, s.String("recipient"), tss, s.String("type"))
	case "register":
		runErr = runRegister(ctx, e, g, s.Bool("voice"), s.String("captcha"), s.Bool("reregister"))
	case "verify":
		runErr = runVerify(ctx, e, g, s.String("verification_code"), s.String("pin"))
	case "listDevices":
		runErr = runListDevices(ctx, e, g)
	case "addDevice":
		runErr = runAddDevice(ctx, e, g, s.String("uri"))
	case "removeDevice":
		runErr = runRemoveDevice(ctx, e, g, s.Int("device-id"))
	case "updateDevice":
		runErr = runUpdateDevice(ctx, e, g, s.Int("device-id"), s.String("device-name"))
	case "unregister":
		runErr = runUnregister(ctx, e, g, s.Bool("delete_account"))
	case "sendStory":
		runErr = runSendStory(ctx, e, g, s.String("attachment"), s.Bool("no_replies"), s.String("group-id"))
	case "addStickerPack":
		runErr = runAddStickerPack(ctx, e, g, s.Strings("uri"))
	case "getSticker":
		runErr = runGetSticker(e, g, s.String("pack-id"), s.Int("sticker-id"))
	case "listStickerPacks":
		runErr = runListStickerPacks(e, g)
	case "uploadStickerPack":
		runErr = runUploadStickerPack(ctx, e, g, s.String("path"))
	case "updateConfiguration":
		set := map[string]*bool{}
		for _, k := range []string{"read-receipts", "unidentified-delivery-indicators", "typing-indicators", "link-previews"} {
			if s.Has(k) {
				v := s.Bool(k)
				set[k] = &v
			}
		}
		runErr = runUpdateConfiguration(ctx, e, g, set)
	case "updateGroup":
		o := updateGroupOpts{groupID: s.String("group-id"), avatar: s.String("avatar"), members: s.Strings("member"),
			remove: s.Strings("remove-member"), admins: s.Strings("admin"), unadmin: s.Strings("remove-admin"), ban: s.Strings("ban"),
			unban: s.Strings("unban"), resetLink: s.Bool("reset_link"), link: s.String("link"), permAdd: s.String("set-permission-add-member"),
			permEdit: s.String("set-permission-edit-details"), permSend: s.String("set-permission-send-messages")}
		strp := func(k string) *string {
			if !s.Has(k) {
				return nil
			}
			v := s.String(k)
			return &v
		}
		o.name, o.description, o.labelEmoji, o.label = strp("name"), strp("description"), strp("member-label-emoji"), strp("member-label")
		if s.Has("expiration") {
			v := uint32(s.Int("expiration"))
			o.expiration = &v
		}
		runErr = runUpdateGroup(ctx, e, g, o)
	case "joinGroup":
		runErr = runJoinGroup(ctx, e, g, s.String("uri"))
	case "quitGroup":
		runErr = runQuitGroup(ctx, e, g, s.String("group-id"), s.Bool("delete"), s.Strings("admin"))
	case "listGroups":
		runErr = runListGroups(ctx, e, g, s.Bool("detailed"), s.Strings("group-id"))
	case "updateProfile":
		strp := func(k string) *string {
			if !s.Has(k) {
				return nil
			}
			v := s.String(k)
			return &v
		}
		runErr = runUpdateProfile(ctx, e, g, updateProfileOpts{given: strp("given-name"), family: strp("family-name"), about: strp("about"),
			emoji: strp("about-emoji"), mobileCoin: strp("mobile-coin-address"), avatar: strp("avatar"), removeAvatar: s.Bool("remove_avatar")})
	case "listContacts":
		o := listContactsOpts{recipients: s.Strings("recipient"), all: s.Bool("all_recipients"), detailed: s.Bool("detailed"), internal: s.Bool("internal")}
		if s.Has("blocked") {
			v := s.Bool("blocked")
			o.blocked = &v
		}
		if s.Has("name") {
			v := s.String("name")
			o.name = &v
		}
		runErr = runListContacts(ctx, e, g, o)
	case "updateContact":
		strp := func(k string) *string {
			if !s.Has(k) {
				return nil
			}
			v := s.String(k)
			return &v
		}
		u := manager.ContactUpdate{GivenName: strp("given-name"), FamilyName: strp("family-name"), NickGivenName: strp("nick-given-name"),
			NickFamilyName: strp("nick-family-name"), Note: strp("note")}
		if s.Has("expiration") {
			v := uint32(s.Int("expiration"))
			u.Expiration = &v
		}
		runErr = runUpdateContact(ctx, e, g, s.String("recipient"), strp("name"), u)
	case "removeContact":
		runErr = runRemoveContact(e, g, s.String("recipient"), s.Bool("hide"), s.Bool("forget"))
	case "block", "unblock":
		runErr = runBlock(ctx, e, g, s.Strings("recipient"), s.Strings("group-id"), cmd == "block")
	case "getAvatar":
		runErr = runGetAvatar(ctx, e, g, s.String("contact"), s.String("profile"), s.String("group-id"))
	case "listIdentities":
		runErr = runListIdentities(e, g, s.String("number"), s.Has("number"))
	case "updateAccount":
		o := updateAccountOpts{deleteUsername: s.Bool("delete_username")}
		boolp := func(k string) *bool {
			if !s.Has(k) {
				return nil
			}
			v := s.Bool(k)
			return &v
		}
		o.unrestricted, o.discoverable, o.numShare = boolp("unrestricted-unidentified-sender"), boolp("discoverable-by-number"), boolp("number-sharing")
		if s.Has("device-name") {
			v := s.String("device-name")
			o.deviceName = &v
		}
		if s.Has("username") {
			v := s.String("username")
			o.username = &v
		}
		runErr = runUpdateAccount(ctx, e, g, o)
	case "getUserStatus":
		runErr = runGetUserStatus(ctx, e, g, s.Strings("recipient"), s.Strings("username"))
	case "setPin", "removePin":
		runErr = runSetPin(ctx, e, g, s.String("pin"), cmd == "removePin")
	case "trust":
		sn := s.String("verified_safety_number")
		runErr = runTrust(ctx, e, g, s.String("recipient"), s.Bool("trust_all_known_keys"), sn, s.Has("verified_safety_number"))
	case "sendSyncRequest":
		runErr = runSyncCommand(ctx, e, g, false)
	case "sendContacts":
		runErr = runSyncCommand(ctx, e, g, true)
	case "getAttachment":
		runErr = runGetAttachment(e, g, s.String("id"))
	case "receive":
		t := s.Float("timeout")
		timeout := time.Duration(t * float64(time.Second)).Truncate(time.Millisecond)
		if t < 0 {
			timeout = 0
		}
		runErr = runReceive(ctx, e, g, receiveOpts{timeout: timeout, maxMessages: s.Int("max-messages"),
			sendReadReceipts: s.Bool("send_read_receipts"), ignoreStories: s.Bool("ignore_stories"), ignoreAttachments: s.Bool("ignore_attachments"), ignoreAvatars: s.Bool("ignore_avatars"), ignoreStickers: s.Bool("ignore_stickers")})
	default:
		runErr = errcode.NotImplemented.New(cmd)
	}
	_ = stderr
	return runErr
}

// defaultDataDir is $XDG_DATA_HOME/signal-cli, else
// $HOME/.local/share/signal-cli (index.md §2).
func defaultDataDir(getenv func(string) string) string {
	if x := getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "signal-cli")
	}
	return filepath.Join(getenv("HOME"), ".local", "share", "signal-cli")
}

// formatWidth is W (argparse.md §1): COLUMNS − 5, else the terminal's
// columns − 5, and 75 when that is ≤ 0 or unknown.
func formatWidth(getenv func(string) string) int {
	cols := -1
	if c, ok := os.LookupEnv("COLUMNS"); ok || getenv("COLUMNS") != "" {
		if getenv("COLUMNS") != "" {
			c = getenv("COLUMNS")
		}
		if n, err := strconv.Atoi(c); err == nil {
			cols = n
		}
	} else if n := ttyColumns(); n > 0 {
		cols = n
	}
	if w := cols - 5; w > 0 {
		return w
	}
	return 75
}

// loggingPreparse runs the early logging parser in known-arguments mode
// (argparse.md §7.7) and reports whether it reaches -h, which prints its
// 7-line help block.
func loggingPreparse(args []string) bool {
	flags := []string{"-h", "--help", "-v", "--verbose", "--log-file", "--scrub-log"}
	resolve := func(part string) string {
		for _, f := range flags {
			if f == part {
				return f
			}
		}
		var match string
		n := 0
		for _, f := range flags {
			if strings.HasPrefix(f, part) {
				match, n = f, n+1
			}
		}
		if n == 1 {
			return match
		}
		return ""
	}
	isFlag := func(s string) bool {
		return strings.HasPrefix(s, "-") && strings.Trim(s, "-") != "" && !negativeInteger(s)
	}
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			return false
		}
		if !isFlag(tok) {
			continue
		}
		part, _, attached := strings.Cut(tok, "=")
		f := resolve(part)
		if f == "" && !strings.HasPrefix(tok, "--") {
			// Concatenated short flags: -vvh.
		cluster:
			for _, c := range tok[1:] {
				switch c {
				case 'h':
					return true
				case 'v':
				default:
					break cluster
				}
			}
			continue
		}
		switch f {
		case "-h", "--help":
			return !attached
		case "--log-file":
			if !attached {
				if i+1 >= len(args) || isFlag(args[i+1]) || args[i+1] == "--" {
					return false // an error: signal-cli ignores this pass
				}
				i++
			}
		}
	}
	return false
}

func negativeInteger(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
