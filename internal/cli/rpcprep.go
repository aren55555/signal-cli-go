// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/jackson"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// JSON-RPC runs commands without the argument parser, so values the
// CLI parser requires may be absent and reach code that dereferences
// them. rpcPrepare reproduces what the reference reports then (a Java
// NullPointerException, mostly), in its validation order, and applies
// the few JSON-RPC-only conversions (methods/*.md; the exact texts were
// observed against the reference with a registered account).

func npe(invoke, variable string) error {
	return errcode.RPCNullPointer.New(invoke, `"`+variable+`"`)
}

// npeRecipient is a null single recipient reaching the identifier parser.
var npeRecipient = npe("String.startsWith(String)", "identifier")

func npeLong(variable string) error { return npe("java.lang.Long.longValue()", variable) }
func npeInt(variable string) error  { return npe("java.lang.Integer.intValue()", variable) }

// rpcPrepare checks and adjusts the parsed values of method name; it
// returns nil when the command should run (and report any other error
// itself).
func rpcPrepare(name string, a *store.Account, p *jackson.Node, r *argparse.Result) error {
	has := func(k string) bool { return r.Has(k) }
	switch name {
	case "sendReaction", "sendPinMessage", "sendUnpinMessage", "remoteDelete", "sendPollCreate", "sendPollTerminate", "sendPollVote", "send":
		single, ok := rpcTargets(a, r)
		if !ok {
			return nil // the command reports the target problem
		}
		authorDefault := func() error {
			if has("target-author") {
				return nil
			}
			if single == "" {
				return npeRecipient
			}
			r.Values["target-author"] = single
			return nil
		}
		switch name {
		case "send":
			if has("story-timestamp") && !has("story-author") {
				return npeRecipient
			}
		case "sendReaction", "sendUnpinMessage":
			if !has("target-timestamp") {
				return npeLong("targetTimestamp")
			}
			return authorDefault()
		case "sendPinMessage":
			if !has("pin-duration") {
				return npeInt("pinDuration")
			}
			if !has("target-timestamp") {
				return npeLong("targetTimestamp")
			}
			return authorDefault()
		case "remoteDelete":
			if !has("target-timestamp") {
				return errcode.RPCNullPointer.New("java.lang.Long.longValue()", `the return value of "net.sourceforge.argparse4j.inf.Namespace.getLong(String)"`)
			}
		case "sendPollCreate":
			if !has("option") {
				return npe("java.util.List.size()", "options")
			}
		case "sendPollTerminate":
			if !has("poll-timestamp") {
				return npeLong("pollTimestamp")
			}
		case "sendPollVote":
			switch {
			case !has("poll-author"):
				return npeRecipient
			case !has("poll-timestamp"):
				return npeLong("pollTimestamp")
			case !has("vote-count"):
				return npeInt("voteCount")
			}
		}
	case "sendAdminDelete":
		gids := r.Strings("group-id")
		for _, g := range gids {
			if _, err := groups.ParseGroupID(g); err != nil {
				return nil
			}
		}
		switch {
		case len(gids) == 0:
			return nil
		case !has("target-author"):
			return npeRecipient
		case !has("target-timestamp"):
			return npeLong("targetTimestamp")
		}
	case "sendReceipt":
		if !has("recipient") {
			return npeRecipient
		}
		if _, err := rpcRecipientSyntax(a, r.String("recipient")); err != nil {
			return nil
		}
		if t := r.String("type"); has("type") && t != "read" && t != "viewed" {
			return nil
		}
		if !has("target-timestamp") {
			return errcode.RPCNullPointer.New("java.util.List.stream()", `the return value of "org.whispersystems.signalservice.api.messages.SignalServiceReceiptMessage.getTimestamps()"`)
		}
	case "sendPaymentNotification":
		if !has("recipient") {
			return npeRecipient
		}
		if _, err := rpcRecipientSyntax(a, r.String("recipient")); err != nil {
			return nil
		}
		if !has("receipt") {
			return npe("String.getBytes(java.nio.charset.Charset)", "src")
		}
		if msg := javaBase64Error(r.String("receipt")); msg != "" {
			return errcode.IllegalArgumentCrash.New(msg)
		}
	case "sendMessageRequestResponse":
		v := lookup(p, "type")
		switch {
		case v == nil:
			return npe("String.toUpperCase()", "typeObj")
		case v.Kind != jackson.String:
			return classCast(v.JavaClass(), "java.lang.String")
		}
		t := strings.ToUpper(v.S)
		if t != "ACCEPT" && t != "DELETE" {
			return errcode.IllegalArgumentCrash.New("No enum constant org.asamk.signal.commands.MessageRequestResponseType." + t)
		}
		r.Values["type"] = strings.ToLower(t)
	case "trust", "removeContact", "updateContact":
		if !has("recipient") {
			return npeRecipient
		}
	case "getSticker":
		switch id := r.String("pack-id"); {
		case !has("pack-id"):
			return npe("String.length()", "s")
		case len(id)%2 != 0:
			return errcode.RPCIndexOutOfBounds.New(len(id), len(id))
		case !has("sticker-id"):
			return npeInt("stickerId")
		}
	case "removeDevice", "updateDevice":
		if !has("device-id") {
			return npeInt("deviceId")
		}
	case "addDevice", "joinGroup":
		if !has("uri") {
			return npe("String.length()", "this.input")
		}
	case "addStickerPack":
		if !has("uri") {
			return npe("java.util.List.iterator()", "uris")
		}
	case "getAvatar":
		switch {
		case has("contact"):
			delete(r.Values, "profile")
			delete(r.Values, "group-id")
		case has("profile"):
			delete(r.Values, "group-id")
		case !has("group-id"):
			return npe("org.asamk.signal.manager.api.GroupId.toBase64()", "groupId")
		}
	case "setPin", "uploadStickerPack", "quitGroup":
		key := map[string]string{"setPin": "pin", "uploadStickerPack": "path", "quitGroup": "group-id"}[name]
		if !has(key) {
			return errcode.RPCNullPointerPlain
		}
	case "startChangeNumber", "finishChangeNumber":
		if !has("number") {
			return npe("String.equals(Object)", "forNumber")
		}
	}
	return nil
}

// rpcTargets checks the recipients, groups and note-to-self of a send
// command the way the reference parses them before anything else. ok is
// false when the command would fail on them (no targets, a bad number
// or group id); single is the one individual recipient, if exactly one
// was given (the default author of reactions and pins).
func rpcTargets(a *store.Account, r *argparse.Result) (single string, ok bool) {
	var individuals []string
	for _, s := range r.Strings("recipient") {
		if _, err := rpcRecipientSyntax(a, s); err != nil {
			return "", false
		}
		individuals = append(individuals, s)
	}
	for _, u := range r.Strings("username") {
		individuals = append(individuals, "u:"+u)
	}
	gids := r.Strings("group-id")
	for _, g := range gids {
		if _, err := groups.ParseGroupID(g); err != nil {
			return "", false
		}
	}
	if len(individuals)+len(gids) == 0 && !r.Bool("note_to_self") {
		return "", false
	}
	if len(individuals) == 1 && len(gids) == 0 && !r.Bool("note_to_self") {
		single = individuals[0]
	}
	return single, true
}

// rpcRecipientSyntax parses a recipient without contacting the server.
func rpcRecipientSyntax(a *store.Account, s string) (string, error) {
	switch {
	case uuidRe.MatchString(s):
		return s, nil
	case strings.HasPrefix(s, "PNI:"):
		if !uuidRe.MatchString(s[4:]) {
			return "", errcode.InvalidPhoneNumber.New(s, "Invalid PNI")
		}
		return s, nil
	case strings.HasPrefix(s, "u:"):
		return s, nil
	case strings.Contains(s, "@"):
		return "", errcode.InvalidPhoneNumber.New(s, "Possible attempt to use email address.")
	}
	return canonicalNumber(a, s)
}

// javaBase64Error is java.util.Base64's decoder message for s, or "".
func javaBase64Error(s string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for _, c := range []byte(strings.TrimRight(s, "=")) {
		if !strings.ContainsRune(alphabet, rune(c)) {
			return fmt.Sprintf("Illegal base64 character %x", c)
		}
	}
	if _, err := base64.StdEncoding.DecodeString(s); err == nil {
		return ""
	}
	if _, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return ""
	}
	if len(strings.TrimRight(s, "="))%4 == 1 {
		return "Last unit does not have enough valid bits"
	}
	return "Input byte array has wrong 4-byte ending unit"
}
