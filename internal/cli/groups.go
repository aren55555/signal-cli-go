package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/groups"
	"github.com/aren55555/signal-cli-go/internal/javahash"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/rest"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// groupTitle is a group's title for messages, "null" without state.
func groupTitle(rec *store.GroupRecord) string {
	if st := manager.StateOf(rec); st != nil {
		return st.Title
	}
	return "null"
}

// groupSendError maps the group checks of a send (groups.md §12.1).
func groupSendError(s *session, id []byte, err error) error {
	gid := groups.FormatGroupID(id)
	rec := s.acct.Group(id)
	switch {
	case errors.Is(err, manager.ErrGroupNotFound):
		return errcode.GroupNotFound.New(gid)
	case errors.Is(err, manager.ErrNotAMember):
		return errcode.GroupNotMember.New(groupTitle(rec), gid)
	case errors.Is(err, manager.ErrNotAllowed):
		return errcode.GroupSendNotAllowed.New(groupTitle(rec), gid)
	}
	return errcode.SendIOFailed.WrapText(err)
}

// groupResults turns a group send's per-member results into send
// results under the group's key.
func groupResults(a *store.Account, key string, res []manager.SendResult) []sendResult {
	var out []sendResult
	for _, r := range res {
		sr := resultFor(key, r.Dest, transcriptOK(r.Err))
		sr.number = a.Recipients[r.Dest].Number
		sr.groupID = key
		out = append(out, sr)
	}
	return out
}

// groupsManager builds the group manager for a session.
func (n *network) groupsManager(g *globals, a *store.Account, c manager.Requester, s *manager.Sender) *manager.Groups {
	svc := &manager.GroupsService{Acct: a, Chat: c, CDN: n.cdn(),
		REST: rest.New(rest.Config{BaseURL: "https://storage-staging.signal.org", Proxy: n.proxy, UserAgentOverride: n.userAgent})}
	return &manager.Groups{Acct: a, Svc: svc, Sender: s, AvatarsDir: filepath.Join(g.dataDir, "avatars")}
}

// groupCommandError maps errors of group commands (groups.md §14.3).
func groupCommandError(err error) error {
	var ce *errcode.Error
	var ge *manager.GroupError
	switch {
	case errors.As(err, &ce):
		return err
	case errors.Is(err, manager.ErrCreateFailed):
		return errcode.SendGroupCreateFailed.Wrap(err)
	case errors.As(err, &ge):
		return errcode.SendGroupError.Wrap(err, ge.Msg, ge.Class)
	}
	return errcode.SendIOFailed.WrapText(err)
}

// memberResults turns the results of a group update message into send
// results keyed by member.
func memberResults(a *store.Account, res []manager.SendResult) []sendResult {
	var out []sendResult
	for _, r := range res {
		key := a.Recipients[r.Dest].Number
		if key == "" {
			key = r.Dest
		}
		sr := resultFor(key, r.Dest, transcriptOK(r.Err))
		sr.number = a.Recipients[r.Dest].Number
		out = append(out, sr)
	}
	return out
}

// updateGroupOpts are updateGroup's options as given.
type updateGroupOpts struct {
	groupID                           string
	name, description                 *string
	avatar                            string
	members, remove, admins, unadmin  []string
	ban, unban                        []string
	resetLink                         bool
	link, permAdd, permEdit, permSend string
	expiration                        *uint32
	labelEmoji, label                 *string
}

func resolveACIs(a *store.Account, list []string) ([]string, error) {
	var out []string
	for _, r := range list {
		t, err := resolveRecipient(a, r)
		if err != nil {
			return nil, err
		}
		out = append(out, t.aci)
	}
	return out, nil
}

// runUpdateGroup is `updateGroup` (groups.md §7-8, §14.3).
func runUpdateGroup(ctx context.Context, e *env, g *globals, o updateGroupOpts) error {
	return withAccount(g, func(a *store.Account) error {
		var id []byte
		if o.groupID != "" {
			var err error
			if id, err = parseGroupArg(o.groupID); err != nil {
				return err
			}
		}
		lists := [][]string{o.members, o.remove, o.admins, o.unadmin, o.ban, o.unban}
		resolvedLists := make([][]string, len(lists))
		for i, l := range lists {
			acis, err := resolveACIs(a, l)
			if err != nil {
				return err
			}
			resolvedLists[i] = acis
		}
		if o.link != "" {
			v, ok := map[string]string{"enabled": "enabled", "enabled-with-approval": "enabled-with-approval", "enabledWithApproval": "enabled-with-approval", "disabled": "disabled"}[o.link]
			if !ok {
				return errcode.InvalidGroupLinkState.New(o.link)
			}
			o.link = v
		}
		for _, perm := range []*string{&o.permAdd, &o.permEdit, &o.permSend} {
			if *perm == "" {
				continue
			}
			v, ok := map[string]string{"every-member": "every-member", "everyMember": "every-member", "only-admins": "only-admins", "onlyAdmins": "only-admins"}[*perm]
			if !ok {
				return errcode.InvalidGroupPermission.New(*perm)
			}
			*perm = v
		}
		var avatar []byte
		if o.avatar != "" {
			src, err := attachment.FromArgument(o.avatar, filepath.Join(g.dataDir, "data"))
			if err != nil {
				return errcode.SendIOFailed.WrapText(err)
			}
			avatar = src.Data
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		defer s.syncStorage(ctx, e)
		uo := manager.UpdateOptions{Description: o.description, Remove: resolvedLists[1], Admins: resolvedLists[2],
			NoAdmins: resolvedLists[3], Ban: resolvedLists[4], Unban: resolvedLists[5], ResetLink: o.resetLink, Link: o.link,
			AddMemberPerm: o.permAdd, EditDetailsPerm: o.permEdit, SendMessagesPerm: o.permSend, Expiration: o.expiration,
			LabelEmoji: o.labelEmoji, Label: o.label}
		var results []sendResult
		var ts uint64
		var createdID string
		var rec *store.GroupRecord
		if id == nil {
			name := ""
			if o.name != nil {
				name = *o.name
			}
			res, err := s.groups.Create(ctx, name, resolvedLists[0], avatar)
			if err != nil {
				return groupCommandError(err)
			}
			rec, createdID = res.Record, groups.FormatGroupID(res.Record.GroupID)
			results, ts = memberResults(a, res.Results), res.Timestamp
			if !g.json() {
				fmt.Fprintf(e.stdout, "Created new group: \"%s\"\n", createdID)
			}
		} else {
			rec = s.groups.Group(ctx, id, false)
			if rec == nil {
				return errcode.GroupNotFound.New(groups.FormatGroupID(id))
			}
			st := manager.StateOf(rec)
			if st == nil || (st.Member(a.Data.ACI) == nil && st.PendingIndex(a.Data.ACI) < 0 && st.PendingIndex("PNI:"+a.Data.PNI) < 0) {
				return errcode.GroupNotMember.New(groupTitle(rec), groups.FormatGroupID(id))
			}
			uo.Name, uo.Avatar, uo.Add = o.name, avatar, resolvedLists[0]
		}
		s.groups.Refresh(ctx, rec, true)
		rec = a.Group(rec.GroupID)
		res, err := s.groups.Update(ctx, rec, uo)
		if err != nil {
			return groupCommandError(err)
		}
		if res != nil {
			results, ts = append(results, memberResults(a, res.Results)...), res.Timestamp
		}
		if g.json() {
			top := &jsonObject{}
			if createdID != "" {
				top.add("groupId", createdID)
			}
			if ts != 0 {
				top.add("results", resultsJSON(results))
				top.add("timestamp", ts)
			}
			fmt.Fprintln(e.stdout, top.String())
			return nil
		}
		if ts == 0 {
			return nil
		}
		return finishSendPlainOnly(e, ts, results)
	})
}

// finishSendPlainOnly prints plain results and the timestamp; the exit
// code follows the shared send rules.
func finishSendPlainOnly(e *env, ts uint64, results []sendResult) error {
	out, code, msg := renderSendResults(false, ts, results)
	io.WriteString(e.stdout, out)
	if code != 0 {
		return sendFailure(code, msg)
	}
	return nil
}

// runJoinGroup is `joinGroup` (groups.md §9).
func runJoinGroup(ctx context.Context, e *env, g *globals, uri string) error {
	return withAccount(g, func(a *store.Account) error {
		if _, _, err := groups.ParseInviteLink(uri); err != nil {
			return joinLinkError(err)
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		defer s.syncStorage(ctx, e)
		res, requested, err := s.groups.Join(ctx, uri)
		var inactive *manager.GroupLinkInactiveError
		var ge *manager.GroupError
		switch {
		case errors.As(err, &inactive):
			return errcode.GroupLinkNotValid.WrapText(inactive)
		case errors.Is(err, manager.ErrAlreadyRequested):
			return errcode.GroupPendingApproval.WrapText(err)
		case errors.As(err, &ge):
			return errcode.SendIOFailedIO.Wrap(err, ge.Msg, ge.Class)
		case err != nil:
			return errcode.SendIOFailedIO.Wrap(err, err, "IOException")
		}
		gid := groups.FormatGroupID(res.Record.GroupID)
		results := memberResults(a, res.Results)
		if g.json() {
			top := &jsonObject{}
			top.add("timestamp", res.Timestamp)
			top.add("results", resultsJSON(results))
			top.add("groupId", gid)
			if requested {
				top.add("onlyRequested", true)
			}
			fmt.Fprintln(e.stdout, top.String())
			return nil
		}
		if requested {
			fmt.Fprintf(e.stdout, "Requested to join group \"%s\"\n", gid)
		} else {
			fmt.Fprintf(e.stdout, "Joined group \"%s\"\n", gid)
		}
		return finishSendPlainOnly(e, res.Timestamp, results)
	})
}

func joinLinkError(err error) error {
	var le *groups.InviteLinkError
	var ue *groups.ErrUncaught
	switch {
	case errors.Is(err, groups.ErrNotALink):
		return errcode.GroupLinkNotALink.Wrap(err)
	case errors.As(err, &le) && le.UnknownVersion:
		return errcode.GroupLinkIncompatible.Wrap(err, le.Message)
	case errors.As(err, &le):
		return errcode.GroupLinkInvalid.Wrap(err, le.Message)
	case errors.As(err, &ue):
		return errcode.IllegalArgumentCrash.Wrap(err, strings.TrimPrefix(ue.Message, "java.lang.IllegalArgumentException: "))
	}
	return err
}

// runQuitGroup is `quitGroup` (groups.md §10).
func runQuitGroup(ctx context.Context, e *env, g *globals, groupID string, del bool, admins []string) error {
	return withAccount(g, func(a *store.Account) error {
		id, err := parseGroupArg(groupID)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		defer s.syncStorage(ctx, e)
		rec := s.groups.Group(ctx, id, true)
		gid := groups.FormatGroupID(id)
		if rec == nil {
			return errcode.GroupSendFailed.New(errcode.GroupNotFound.New(gid))
		}
		st := manager.StateOf(rec)
		member := st != nil && (st.Member(a.Data.ACI) != nil || st.PendingIndex(a.Data.ACI) >= 0 || st.PendingIndex("PNI:"+a.Data.PNI) >= 0)
		if member {
			acis, err := resolveACIs(a, admins)
			if err != nil {
				return err
			}
			res, err := s.groups.Quit(ctx, rec, acis)
			if errors.Is(err, manager.ErrLastAdmin) {
				return errcode.GroupLastAdmin.New(groupTitle(rec), gid)
			}
			if err != nil {
				return groupCommandError(err)
			}
			results := memberResults(a, res.Results)
			if g.json() {
				top := &jsonObject{}
				top.add("timestamp", res.Timestamp)
				top.add("results", resultsJSON(results))
				fmt.Fprintln(e.stdout, top.String())
			} else if err := finishSendPlainOnly(e, res.Timestamp, results); err != nil {
				return err
			}
			rec = a.Group(id)
		}
		if del {
			if err := s.groups.Delete(rec); err != nil {
				return errcode.SendIOFailedIO.Wrap(err, err, "IOException")
			}
		}
		return nil
	})
}

// runListGroups is `listGroups` (groups.md §14.1, listGroups.md).
func runListGroups(ctx context.Context, e *env, g *globals, detailed bool, ids []string) error {
	return withAccount(g, func(a *store.Account) error {
		var want [][]byte
		for _, s := range ids {
			id, err := parseGroupArg(s)
			if err != nil {
				return err
			}
			want = append(want, id)
		}
		// Lookups contact the groups service: missing state, or -g given.
		needNet := len(want) > 0
		for _, rec := range a.Groups {
			if len(rec.State) == 0 && !rec.PermissionDenied {
				needNet = true
			}
		}
		var gm *manager.Groups
		if needNet {
			ctx2, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			if s, err := openSession(ctx2, e, g, a); err == nil {
				defer s.close()
				gm, ctx = s.groups, ctx2
			}
		}
		var recs []*store.GroupRecord
		if len(want) > 0 {
			for _, id := range want {
				rec := a.Group(id)
				if rec == nil {
					continue
				}
				if gm != nil {
					gm.Refresh(ctx, rec, true)
				}
				recs = append(recs, a.Group(id))
			}
		} else {
			for i := range a.Groups {
				if gm != nil {
					gm.Refresh(ctx, &a.Groups[i], false)
				}
			}
			for i := range a.Groups {
				recs = append(recs, &a.Groups[i])
			}
		}
		var items []string
		for _, rec := range recs {
			if g.json() {
				items = append(items, groupJSON(a, rec).String())
			} else {
				fmt.Fprintln(e.stdout, groupLine(a, rec, detailed))
			}
		}
		if g.json() {
			fmt.Fprintln(e.stdout, "["+strings.Join(items, ",")+"]")
		}
		return nil
	})
}

func ident(a *store.Account, id string) string {
	if n := a.Recipients[id].Number; n != "" {
		return n
	}
	if id == a.Data.ACI && a.Data.Number != "" {
		return a.Data.Number
	}
	return id
}

func permission(st *groups.State, f func(*groups.Access) int32) string {
	if st != nil && st.Access != nil && f(st.Access) == groups.AccessAdmin {
		return "ONLY_ADMINS"
	}
	return "EVERY_MEMBER"
}

func groupLine(a *store.Account, rec *store.GroupRecord, detailed bool) string {
	st := manager.StateOf(rec)
	name, desc := "null", "null"
	member := false
	if st != nil {
		name, desc, member = st.Title, st.Description, st.Member(a.Data.ACI) != nil
	}
	gid := groups.FormatGroupID(rec.GroupID)
	if !detailed {
		return fmt.Sprintf("Id: %s Name: %s  Active: %t Blocked: %t", gid, name, member, rec.Blocked)
	}
	set := func(xs []string) string { return "[" + strings.Join(xs, ", ") + "]" }
	var members, pending, requesting, banned []string
	exp := "disabled"
	if st != nil {
		for _, m := range orderedMembers(a, st) {
			s := legacyIdent(m.addr)
			if m.admin {
				s += "{ADMIN}"
			}
			if m.emoji != "" || m.label != "" {
				s += "(" + m.emoji + m.label + ")"
			}
			members = append(members, s)
		}
		idents := func(as []groupAddr) []string {
			var xs []string
			for _, g := range as {
				xs = append(xs, legacyIdent(g))
			}
			return orderedStrings(xs)
		}
		members = orderedStrings(members)
		pending, requesting, banned = idents(orderedAddrs(a, pendingIDs(st))), idents(orderedAddrs(a, requestingIDs(st))), idents(orderedAddrs(a, bannedIDs(st)))
		if st.Timer > 0 {
			exp = fmt.Sprintf("%ds", st.Timer)
		}
	}
	link := manager.InviteLink(rec)
	if link == "" {
		link = "-"
	}
	return fmt.Sprintf("Id: %s Name: %s Description: %s Active: %t Blocked: %t Members: %s Pending members: %s Requesting members: %s Banned: %s Message expiration: %s Link: %s",
		gid, name, desc, member, rec.Blocked, set(members), set(pending), set(requesting), set(banned), exp, link)
}

func addrJSON(a *store.Account, id string) *jsonObject {
	o := &jsonObject{}
	num := a.Recipients[id].Number
	if id == a.Data.ACI {
		num = a.Data.Number
	}
	o.add("number", strOrNull(num))
	if strings.HasPrefix(id, "PNI:") {
		o.add("uuid", nil)
	} else {
		o.add("uuid", id)
	}
	return o
}

func groupJSON(a *store.Account, rec *store.GroupRecord) *jsonObject {
	st := manager.StateOf(rec)
	o := &jsonObject{}
	o.add("id", groups.FormatGroupID(rec.GroupID))
	if st != nil {
		o.add("name", st.Title)
		o.add("description", st.Description)
		o.add("isMember", st.Member(a.Data.ACI) != nil)
	} else {
		o.add("name", nil)
		o.add("description", nil)
		o.add("isMember", false)
	}
	o.add("isBlocked", rec.Blocked)
	var timer uint32
	var members, pending, requesting, admins, banned []string
	if st != nil {
		timer = st.Timer
		// Every list is a hash set of output records (serialization.md §1.2).
		ms := orderedMembers(a, st)
		jm := javahash.Order(ms, func(m groupMember) int32 {
			return javahash.Record(javahash.Nullable(m.addr.number), javahash.Nullable(m.addr.aci), javahash.Bool(m.admin), javahash.Nullable(m.emoji), javahash.Nullable(m.label))
		}, func(m groupMember) string {
			return fmt.Sprint(m.addr.number, "|", m.addr.aci, m.admin, "|", m.emoji, "|", m.label)
		})
		for _, m := range jm {
			mo := addrOutJSON(m.addr)
			mo.add("isAdmin", m.admin)
			if m.emoji != "" {
				mo.add("labelEmoji", m.emoji)
			}
			if m.label != "" {
				mo.add("label", m.label)
			}
			members = append(members, mo.String())
		}
		var adm []groupAddr
		for _, m := range ms {
			if m.admin {
				adm = append(adm, m.addr)
			}
		}
		out := func(as []groupAddr) []string {
			as = javahash.Order(as, groupAddr.jsonAddrHash, func(g groupAddr) string { return g.number + "|" + g.aci })
			var xs []string
			for _, g := range as {
				xs = append(xs, addrOutJSON(g).String())
			}
			return xs
		}
		admins = out(javahash.Order(adm, groupAddr.hash, groupAddr.key))
		pending, requesting, banned = out(orderedAddrs(a, pendingIDs(st))), out(orderedAddrs(a, requestingIDs(st))), out(orderedAddrs(a, bannedIDs(st)))
	}
	arr := func(xs []string) rawJSON { return rawJSON("[" + strings.Join(xs, ",") + "]") }
	o.add("messageExpirationTime", timer)
	o.add("members", arr(members))
	o.add("pendingMembers", arr(pending))
	o.add("requestingMembers", arr(requesting))
	o.add("admins", arr(admins))
	o.add("banned", arr(banned))
	o.add("permissionAddMember", permission(st, func(ac *groups.Access) int32 { return ac.Members }))
	o.add("permissionEditDetails", permission(st, func(ac *groups.Access) int32 { return ac.Attributes }))
	send := "EVERY_MEMBER"
	if st != nil && st.Announcements == groups.Enabled {
		send = "ONLY_ADMINS"
	}
	o.add("permissionSendMessage", send)
	o.add("groupInviteLink", strOrNull(manager.InviteLink(rec)))
	return o
}

// groupAddr is a group member's address as the reference's api
// RecipientAddress record (serialization.md §1.2).
type groupAddr struct {
	id                         string // our recipient key
	aci, pni, number, username string
	rowID                      int64
}

func memberAddr(a *store.Account, id string) groupAddr {
	g := groupAddr{id: id}
	rc := a.Recipients[id]
	g.rowID, g.number, g.username = rc.ID, rc.Number, rc.Username
	switch {
	case strings.HasPrefix(id, "PNI:"):
		g.pni = id
	default:
		g.aci = id
		if rc.PNI != "" {
			g.pni = "PNI:" + rc.PNI
		}
	}
	if id == a.Data.ACI {
		g.number, g.username = a.Data.Number, a.Data.Username
		if a.Data.PNI != "" {
			g.pni = "PNI:" + a.Data.PNI
		}
	}
	return g
}

func (g groupAddr) hash() int32 {
	return javahash.Record(javahash.Optional(g.aci), javahash.Optional(g.pni), javahash.Optional(g.number), javahash.Optional(g.username))
}

func (g groupAddr) key() string { return g.aci + "|" + g.pni + "|" + g.number + "|" + g.username }

// jsonAddrHash is the JsonGroupMemberAddress record (number, uuid).
func (g groupAddr) jsonAddrHash() int32 {
	return javahash.Record(javahash.Nullable(g.number), javahash.Nullable(g.aci))
}

// groupMember is a GroupMember record.
type groupMember struct {
	addr         groupAddr
	admin        bool
	emoji, label string
}

func (m groupMember) hash() int32 {
	return javahash.Record(m.addr.hash(), javahash.Bool(m.admin), javahash.Nullable(m.emoji), javahash.Nullable(m.label))
}

func (m groupMember) key() string {
	return fmt.Sprint(m.addr.key(), m.admin, "|", m.emoji, "|", m.label)
}

// orderedMembers is Group.members' iteration order.
func orderedMembers(a *store.Account, st *groups.State) []groupMember {
	var ms []groupMember
	for _, m := range st.Members {
		ms = append(ms, groupMember{addr: memberAddr(a, m.ACI), admin: m.Role == groups.RoleAdmin, emoji: m.LabelEmoji, label: m.LabelString})
	}
	return javahash.Order(ms, groupMember.hash, groupMember.key)
}

// orderedAddrs is the iteration order of a Group address set built from
// recipient ids (pending, requesting, banned).
func orderedAddrs(a *store.Account, ids []string) []groupAddr {
	var as []groupAddr
	for _, id := range ids {
		as = append(as, memberAddr(a, id))
	}
	as = javahash.Order(as, func(g groupAddr) int32 { return 31 + javahash.Long(g.rowID) }, func(g groupAddr) string { return g.id })
	return javahash.Order(as, groupAddr.hash, groupAddr.key)
}

// orderedStrings is a HashSet<String>'s order.
func orderedStrings(xs []string) []string {
	return javahash.Order(xs, javahash.String, func(s string) string { return s })
}

func addrOutJSON(g groupAddr) *jsonObject {
	o := &jsonObject{}
	o.add("number", strOrNull(g.number))
	o.add("uuid", strOrNull(g.aci))
	return o
}

func pendingIDs(st *groups.State) []string {
	var ids []string
	for _, p := range st.Pending {
		ids = append(ids, p.ServiceID)
	}
	return ids
}

func requestingIDs(st *groups.State) []string {
	var ids []string
	for _, r := range st.Requesting {
		ids = append(ids, r.ACI)
	}
	return ids
}

func bannedIDs(st *groups.State) []string {
	var ids []string
	for _, b := range st.Banned {
		ids = append(ids, b.ServiceID)
	}
	return ids
}

// legacyIdent is the reference's legacy identifier: number, else ACI,
// else PNI, else username.
func legacyIdent(g groupAddr) string {
	for _, s := range []string{g.number, g.aci, g.pni, g.username} {
		if s != "" {
			return s
		}
	}
	return ""
}
