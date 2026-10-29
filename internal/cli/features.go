// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/cli/argparse"
	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/protocol"
	"github.com/aren55555/signal-cli-go/internal/store"
	"google.golang.org/protobuf/proto"
)

// Data-message feature commands (spec/service/content-features.md,
// spec/surface/cli/<command>.md).

// recipientSpec is the recipient options most send-family commands share.
type recipientSpec struct {
	recipients []string
	groups     []string
	usernames  []string
	noteToSelf bool
}

func recipientsOf(s *argparse.Result) recipientSpec {
	return recipientSpec{recipients: s.Strings("recipient"), groups: s.Strings("group-id"), usernames: s.Strings("username"), noteToSelf: s.Bool("note_to_self")}
}

// targets resolves recipients; note-to-self is the own ACI under the key
// "Note-To-Self" (_send-results.md).
func (rs recipientSpec) targets(a *store.Account) ([]resolved, error) {
	if len(rs.recipients) == 0 && len(rs.groups) == 0 && len(rs.usernames) == 0 && !rs.noteToSelf {
		return nil, errcode.NoRecipients
	}
	var groupTargets []resolved
	for _, g := range rs.groups {
		id, err := parseGroupArg(g)
		if err != nil {
			return nil, err
		}
		groupTargets = append(groupTargets, resolved{key: groups.FormatGroupID(id), group: id})
	}
	if len(rs.usernames) > 0 {
		return nil, errcode.UsernameSendUnsupported
	}
	out, err := resolveAll(a, rs.recipients)
	if err != nil {
		return nil, err
	}
	if rs.noteToSelf {
		out = append(out, resolved{key: "Note-To-Self", aci: a.Data.ACI})
	}
	return append(out, groupTargets...), nil
}

// checkGroupID decodes a group ID (16 or 32 bytes, standard base64).
func checkGroupID(g string) error {
	_, err := parseGroupArg(g)
	return err
}

// authorBinary resolves an author to its binary service ID (§1.2).
func authorBinary(a *store.Account, s string) ([]byte, error) {
	r, err := resolveRecipient(a, s)
	if err != nil {
		return nil, err
	}
	b, ok := uuidBytes16(r.aci)
	if !ok {
		return make([]byte, 16), nil
	}
	return b, nil
}

func uuidBytes16(s string) ([]byte, bool) {
	b := uuidRaw(s)
	return b, len(b) == 16
}

// sendDataCommand runs the common path for one DataMessage command and
// prints the results.
func sendDataCommand(ctx context.Context, e *env, g *globals, a *store.Account, targets []resolved, d manager.DataSend) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	s, err := openSession(ctx, e, g, a)
	if err != nil {
		return err
	}
	defer s.close()
	if d.Prepare != nil {
		if err := d.Prepare(ctx, s.sender); err != nil {
			var ce *errcode.Error
			if errors.As(err, &ce) {
				return err
			}
			return errcode.SendIOFailed.WrapText(err)
		}
	}
	var people, groupTargets []resolved
	for _, t := range targets {
		if t.group != nil {
			groupTargets = append(groupTargets, t)
		} else {
			people = append(people, t)
		}
	}
	var results []sendResult
	var ts uint64
	if len(people) > 0 {
		acis := make([]string, len(people))
		for i, t := range people {
			acis[i] = t.aci
		}
		var res []manager.SendResult
		ts, res = s.sender.SendData(ctx, acis, d)
		for i, r := range res {
			results = append(results, resultFor(people[i].key, people[i].aci, transcriptOK(r.Err)))
		}
		d.Timestamp = ts
	}
	for _, t := range groupTargets {
		gts, res, err := s.groups.SendData(ctx, t.group, d)
		if err != nil {
			return groupSendError(s, t.group, err)
		}
		ts, d.Timestamp = gts, gts
		results = append(results, groupResults(a, t.key, res)...)
	}
	return finishSend(e, g, ts, results)
}

// transcriptOK treats a failed transcript as a successful send.
func transcriptOK(err error) error {
	if err != nil && strings.HasPrefix(err.Error(), "sent, but the sync transcript failed") {
		return nil
	}
	return err
}

func withAccount(g *globals, f func(*store.Account) error) error {
	a, release, err := useAccount(g)
	if err != nil {
		return err
	}
	defer release()
	return f(a)
}

// useAccount opens the command's account: inside the daemon the shared
// open account, held for the command's run; else a fresh load.
func useAccount(g *globals) (*store.Account, func(), error) {
	if g.daemon != nil {
		la := g.daemon.account(g.account)
		if la == nil {
			return nil, nil, errcode.UserNotRegistered.New(g.account)
		}
		la.mu.Lock()
		return la.acct, la.mu.Unlock, nil
	}
	a, err := selectAccount(g.root(), g.account)
	if err != nil {
		return nil, nil, err
	}
	return a, func() { a.Close() }, nil
}

var (
	mentionRe = regexp.MustCompile(`^(\d+):(\d+):(.+)$`)
	styleRe   = regexp.MustCompile(`^(\d+):(\d+):(.+)$`)
	stickerRe = regexp.MustCompile(`^([0-9a-f]*):(\d+)$`)
)

func parseUint32(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, errcode.NumberParseFailed.Wrap(err, s)
	}
	return uint32(n), nil
}

func mentionRanges(a *store.Account, vals []string) ([]*signalpb.AppBodyRange, error) {
	var out []*signalpb.AppBodyRange
	for _, v := range vals {
		m := mentionRe.FindStringSubmatch(v)
		if m == nil {
			return nil, errcode.InvalidMention.New(v)
		}
		start, err := parseUint32(m[1])
		if err != nil {
			return nil, err
		}
		length, err := parseUint32(m[2])
		if err != nil {
			return nil, err
		}
		bin, err := authorBinary(a, m[3])
		if err != nil {
			return nil, err
		}
		out = append(out, &signalpb.AppBodyRange{Start: proto.Uint32(start), Length: proto.Uint32(length),
			AssociatedValue: &signalpb.AppBodyRange_MentionAciBinary{MentionAciBinary: bin}})
	}
	return out, nil
}

func styleRanges(vals []string) ([]*signalpb.AppBodyRange, error) {
	var out []*signalpb.AppBodyRange
	for _, v := range vals {
		m := styleRe.FindStringSubmatch(v)
		if m == nil {
			return nil, errcode.InvalidTextStyle.New(v)
		}
		st, ok := signalpb.AppBodyRange_Style_value[m[3]]
		if !ok {
			return nil, errcode.InvalidStyle.New(m[3])
		}
		start, err := parseUint32(m[1])
		if err != nil {
			return nil, err
		}
		length, err := parseUint32(m[2])
		if err != nil {
			return nil, err
		}
		out = append(out, &signalpb.AppBodyRange{Start: proto.Uint32(start), Length: proto.Uint32(length),
			AssociatedValue: &signalpb.AppBodyRange_Style_{Style: signalpb.AppBodyRange_Style(st)}})
	}
	return out, nil
}

// fullSendOpts are send's options beyond text.
type fullSendOpts struct {
	sendOpts
	recipients                   recipientSpec
	notifySelf, viewOnce, endSes bool
	mentions, styles             []string
	quoteTS                      int64
	hasQuote                     bool
	quoteAuthor, quoteMessage    string
	hasQuoteMessage              bool
	quoteMentions, quoteStyles   []string
	quoteAttachments             []string
	sticker                      string
	previewURL, previewTitle     string
	previewDesc                  string
	hasPreview                   bool
	storyTS                      int64
	hasStory                     bool
	storyAuthor                  string
	editTS                       int64
	hasEdit                      bool
	attachments                  []string
	previewImage                 string
	voiceNote                    bool
}

var quoteAttachmentRe = regexp.MustCompile(`^([^:]+)(?::([^:]*)(?::(.*))?)?$`)

// runSendFull is `send` with every option signal-cli-go supports
// (spec/surface/cli/send.md, content-features.md §3).
func runSendFull(ctx context.Context, e *env, g *globals, o fullSendOpts) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := o.recipients.targets(a)
		if err != nil {
			return err
		}
		if o.endSes {
			return endSession(a, targets)
		}
		if o.sticker != "" && !stickerRe.MatchString(o.sticker) || (o.sticker != "" && len(strings.Split(o.sticker, ":")[0])%2 != 0) {
			return errcode.InvalidSticker.New(o.sticker)
		}
		text := o.message
		if o.fromStdin {
			b, err := readAll(e)
			if err != nil {
				return errcode.StdinReadFailed.WrapText(err)
			}
			text = b
		}
		type thumb struct {
			att *signalpb.AppQuotedAttachment
			src attachment.Source
		}
		var thumbs []thumb
		dm := &signalpb.AppDataMessage{}
		if text != "" {
			dm.Body = proto.String(text)
		}
		ms, err := mentionRanges(a, o.mentions)
		if err != nil {
			return err
		}
		ss, err := styleRanges(o.styles)
		if err != nil {
			return err
		}
		dm.BodyRanges = append(ms, ss...)
		if o.hasQuote {
			if o.quoteAuthor == "" {
				return errcode.QuoteAuthorMissing
			}
			qm, err := mentionRanges(a, o.quoteMentions)
			if err != nil {
				return err
			}
			qs, err := styleRanges(o.quoteStyles)
			if err != nil {
				return err
			}
			var qas []*signalpb.AppQuotedAttachment
			for _, qa := range o.quoteAttachments {
				m := quoteAttachmentRe.FindStringSubmatch(qa)
				if m == nil {
					return errcode.InvalidQuoteAttachment.New(qa)
				}
				att := &signalpb.AppQuotedAttachment{ContentType: proto.String(m[1])}
				if m[2] != "" {
					att.FileName = proto.String(m[2])
				}
				if m[3] != "" {
					src, err := attachmentSource(m[3], g)
					if err != nil {
						return err
					}
					thumbs = append(thumbs, thumb{att, src})
				}
				qas = append(qas, att)
			}
			author, err := authorBinary(a, o.quoteAuthor)
			if err != nil {
				return err
			}
			dm.Quote = &signalpb.AppQuote{Id: proto.Uint64(uint64(o.quoteTS)), Text: proto.String(o.quoteMessage), Attachments: qas,
				BodyRanges: append(qm, qs...), Type: signalpb.AppQuote_NORMAL.Enum(), AuthorAciBinary: author}
		}
		if o.hasPreview {
			dm.Preview = []*signalpb.AppPreview{{Url: proto.String(o.previewURL), Title: proto.String(o.previewTitle),
				Description: proto.String(o.previewDesc), Date: proto.Uint64(0)}}
		}
		if o.hasStory {
			author, err := authorBinary(a, o.storyAuthor)
			if err != nil {
				return err
			}
			dm.StoryContext = &signalpb.AppStoryContext{SentTimestamp: proto.Uint64(uint64(o.storyTS)), AuthorAciBinary: author}
		}
		if o.viewOnce {
			dm.IsViewOnce = proto.Bool(true)
		}
		var srcs []attachment.Source
		for _, arg := range o.attachments {
			src, err := attachmentSource(arg, g)
			if err != nil {
				return err
			}
			srcs = append(srcs, src)
		}
		var previewSrc *attachment.Source
		if o.hasPreview && o.previewImage != "" {
			src, err := attachmentSource(o.previewImage, g)
			if err != nil {
				return err
			}
			previewSrc = &src
		}
		if text == "" && len(srcs) == 0 && !o.hasQuote && o.sticker == "" {
			return errcode.EmptyMessage
		}
		d := manager.DataSend{Message: dm, NotifySelf: o.notifySelf, Urgent: !o.noUrgent}
		d.Prepare = func(ctx context.Context, s *manager.Sender) error {
			if o.sticker != "" {
				pack, num, _ := strings.Cut(o.sticker, ":")
				n, _ := strconv.ParseUint(num, 10, 32)
				net, _ := e.network(g)
				st, err := s.StickerMessage(ctx, net.stickerStore(g), strings.ToLower(pack), uint32(n))
				if err != nil {
					return errcode.StickerSendFailed.WrapText(err)
				}
				dm.Sticker = st
			}
			if err := s.Attach(ctx, dm, srcs, o.voiceNote); err != nil {
				return err
			}
			for _, t := range thumbs {
				ps, err := s.UploadAttachments(ctx, []attachment.Source{t.src}, 0)
				if err != nil {
					return err
				}
				t.att.Thumbnail = ps[0]
			}
			if previewSrc != nil {
				ps, err := s.UploadAttachments(ctx, []attachment.Source{*previewSrc}, 0)
				if err != nil {
					return err
				}
				dm.Preview[0].Image = ps[0]
			}
			return nil
		}
		if o.hasEdit {
			d.EditTarget = uint64(o.editTS)
		}
		return sendDataCommand(ctx, e, g, a, targets, d)
	})
}

// endSession archives the sessions with each individual recipient and
// sends nothing (content-features.md §3.16).
func endSession(a *store.Account, targets []resolved) error {
	for _, t := range targets {
		if t.aci == a.Data.ACI {
			continue
		}
		for k, b := range a.Sessions {
			_, rest, _ := strings.Cut(k, ":")
			if !strings.HasPrefix(rest, t.aci+".") {
				continue
			}
			if rec, err := protocol.LoadSessionRecord(b); err == nil {
				if nb, err := rec.ArchiveCurrent().Serialize(); err == nil {
					a.Sessions[k] = nb
				}
			}
		}
	}
	return a.SaveSessions()
}

// runSendReaction is `sendReaction` (§3.11).
func runSendReaction(ctx context.Context, e *env, g *globals, rs recipientSpec, notifySelf bool, emoji, author string, target int64, remove, story bool) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := rs.targets(a)
		if err != nil {
			return err
		}
		bin, err := authorBinary(a, author)
		if err != nil {
			return err
		}
		dm := &signalpb.AppDataMessage{Reaction: &signalpb.AppReaction{Emoji: proto.String(emoji), Remove: proto.Bool(remove),
			TargetSentTimestamp: proto.Uint64(uint64(target)), TargetAuthorAciBinary: bin}}
		if story {
			dm.StoryContext = &signalpb.AppStoryContext{SentTimestamp: proto.Uint64(uint64(target)), AuthorAciBinary: bin}
		}
		return sendDataCommand(ctx, e, g, a, targets, manager.DataSend{Message: dm, NotifySelf: notifySelf, Urgent: true})
	})
}

// runRemoteDelete is `remoteDelete` (§3.12).
func runRemoteDelete(ctx context.Context, e *env, g *globals, rs recipientSpec, target int64) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := rs.targets(a)
		if err != nil {
			return err
		}
		dm := &signalpb.AppDataMessage{Delete: &signalpb.AppDelete{TargetSentTimestamp: proto.Uint64(uint64(target))}}
		return sendDataCommand(ctx, e, g, a, targets, manager.DataSend{Message: dm, Urgent: true})
	})
}

// runPollCreate is `sendPollCreate` (§3.13).
func runPollCreate(ctx context.Context, e *env, g *globals, rs recipientSpec, notifySelf bool, question string, noMulti bool, options []string) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := rs.targets(a)
		if err != nil {
			return err
		}
		switch {
		case len(options) < 2:
			return errcode.PollTooFewOptions
		case len(options) > 10:
			return errcode.PollTooManyOptions
		}
		for _, o := range options {
			if o == "" {
				return errcode.PollEmptyOption
			}
			if len(utf16.Encode([]rune(o))) > 100 {
				return errcode.PollOptionTooLong.New(o)
			}
		}
		dm := &signalpb.AppDataMessage{PollCreate: &signalpb.AppPollCreate{Question: proto.String(question), AllowMultiple: proto.Bool(!noMulti), Options: options}}
		return sendDataCommand(ctx, e, g, a, targets, manager.DataSend{Message: dm, NotifySelf: notifySelf, Urgent: true})
	})
}

// runPollVote is `sendPollVote` (§3.13).
func runPollVote(ctx context.Context, e *env, g *globals, rs recipientSpec, notifySelf bool, author string, pollTS int64, options []string, count int) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := rs.targets(a)
		if err != nil {
			return err
		}
		if author == "" {
			return errcode.PollAuthorRequired
		}
		bin, err := authorBinary(a, author)
		if err != nil {
			return err
		}
		var idx []uint32
		for _, o := range options {
			n, err := parseUint32(o)
			if err != nil {
				return err
			}
			idx = append(idx, n)
		}
		dm := &signalpb.AppDataMessage{PollVote: &signalpb.AppPollVote{TargetAuthorAciBinary: bin, TargetSentTimestamp: proto.Uint64(uint64(pollTS)),
			OptionIndexes: idx, VoteCount: proto.Uint32(uint32(count))}}
		return sendDataCommand(ctx, e, g, a, targets, manager.DataSend{Message: dm, NotifySelf: notifySelf, Urgent: true})
	})
}

// runPollTerminate is `sendPollTerminate`.
func runPollTerminate(ctx context.Context, e *env, g *globals, rs recipientSpec, notifySelf bool, pollTS int64) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := rs.targets(a)
		if err != nil {
			return err
		}
		dm := &signalpb.AppDataMessage{PollTerminate: &signalpb.AppPollTerminate{TargetSentTimestamp: proto.Uint64(uint64(pollTS))}}
		return sendDataCommand(ctx, e, g, a, targets, manager.DataSend{Message: dm, NotifySelf: notifySelf, Urgent: true})
	})
}

// runPin is `sendPinMessage` and `sendUnpinMessage` (§3.14).
func runPin(ctx context.Context, e *env, g *globals, rs recipientSpec, notifySelf bool, author string, target int64, story, unpin bool, duration int) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := rs.targets(a)
		if err != nil {
			return err
		}
		bin, err := authorBinary(a, author)
		if err != nil {
			return err
		}
		dm := &signalpb.AppDataMessage{}
		if unpin {
			dm.UnpinMessage = &signalpb.AppUnpinMessage{TargetAuthorAciBinary: bin, TargetSentTimestamp: proto.Uint64(uint64(target))}
		} else {
			pm := &signalpb.AppPinMessage{TargetAuthorAciBinary: bin, TargetSentTimestamp: proto.Uint64(uint64(target))}
			if duration >= 0 {
				pm.PinDuration = &signalpb.AppPinMessage_PinDurationSeconds{PinDurationSeconds: uint32(duration)}
			} else {
				pm.PinDuration = &signalpb.AppPinMessage_PinDurationForever{PinDurationForever: true}
			}
			dm.PinMessage = pm
		}
		if story {
			dm.StoryContext = &signalpb.AppStoryContext{SentTimestamp: proto.Uint64(uint64(target)), AuthorAciBinary: bin}
		}
		return sendDataCommand(ctx, e, g, a, targets, manager.DataSend{Message: dm, NotifySelf: notifySelf, Urgent: true})
	})
}

// runAdminDelete is `sendAdminDelete`: groups only (§3.15).
func runAdminDelete(ctx context.Context, e *env, g *globals, groups []string, notifySelf bool, author string, target int64, story bool) error {
	return withAccount(g, func(a *store.Account) error {
		for _, id := range groups {
			if err := checkGroupID(id); err != nil {
				return err
			}
		}
		if len(groups) == 0 {
			return errcode.AdminDeleteNoGroups
		}
		targets, err := recipientSpec{groups: groups}.targets(a)
		if err != nil {
			return err
		}
		bin, err := authorBinary(a, author)
		if err != nil {
			return err
		}
		dm := &signalpb.AppDataMessage{AdminDelete: &signalpb.AppAdminDelete{TargetAuthorAciBinary: bin, TargetSentTimestamp: proto.Uint64(uint64(target))}}
		if story {
			dm.StoryContext = &signalpb.AppStoryContext{SentTimestamp: proto.Uint64(uint64(target)), AuthorAciBinary: bin}
		}
		return sendDataCommand(ctx, e, g, a, targets, manager.DataSend{Message: dm, NotifySelf: notifySelf, Urgent: true})
	})
}

// runPaymentNotification is `sendPaymentNotification` (§3.17).
func runPaymentNotification(ctx context.Context, e *env, g *globals, recipient, receipt, note string, hasNote bool) error {
	return withAccount(g, func(a *store.Account) error {
		t, err := resolveRecipient(a, recipient)
		if err != nil {
			return err
		}
		raw, err := base64.StdEncoding.DecodeString(receipt)
		if err != nil {
			return errcode.ReceiptBase64.Wrap(err, err)
		}
		if !hasNote {
			return errcode.PaymentNoteRequired
		}
		dm := &signalpb.AppDataMessage{Payment: &signalpb.AppPayment{Item: &signalpb.AppPayment_Notification_{Notification: &signalpb.AppPayment_Notification{
			Note: proto.String(note), Transaction: &signalpb.AppPayment_Notification_MobileCoin_{MobileCoin: &signalpb.AppPayment_Notification_MobileCoin{Receipt: raw}}}}}}
		return sendDataCommand(ctx, e, g, a, []resolved{t}, manager.DataSend{Message: dm, Urgent: true})
	})
}

func readAll(e *env) (string, error) {
	b, err := io.ReadAll(e.stdin)
	return string(b), err
}

// runMessageRequestResponse is `sendMessageRequestResponse`: a sync to our
// own devices only; prints nothing (content-features.md §6).
func runMessageRequestResponse(ctx context.Context, e *env, g *globals, rs recipientSpec, kind string) error {
	return withAccount(g, func(a *store.Account) error {
		targets, err := rs.targets(a)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		for _, t := range targets {
			if err := s.sender.SendMessageRequestResponse(ctx, t.aci, kind == "accept"); err != nil {
				return errcode.MessageRequestFailed.WrapText(err)
			}
		}
		return nil
	})
}

// attachmentSource reads an -a value, a preview image or a quote preview
// file (attachments.md §7).
func attachmentSource(arg string, g *globals) (attachment.Source, error) {
	src, err := attachment.FromArgument(arg, filepath.Join(g.dataDir, "data"))
	if err != nil {
		return src, errcode.SendAttachmentInvalid.Wrap(err, attachmentErrorText(arg, err))
	}
	return src, nil
}

func attachmentErrorText(arg string, err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(err, fs.ErrNotExist) {
		return arg + ": " + arg + " (No such file or directory)"
	}
	return arg + ": " + err.Error()
}

// parseGroupArg parses a -g value (groups.md §1).
func parseGroupArg(s string) ([]byte, error) {
	id, err := groups.ParseGroupID(s)
	if err != nil {
		return nil, errcode.InvalidGroupID.WrapText(err)
	}
	return id, nil
}
