// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/base64"

	"github.com/aren55555/signal-cli-go/internal/crypto/zkgroup"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// GROUPINFO and groupInfo (spec/surface/cli/_received-message-output.md,
// groups.md §14.2).

// groupIDOf derives the group ID of a v2 context's master key; nil for a
// malformed key.
func groupIDOf(gc *signalpb.AppGroupContextV2) []byte {
	if len(gc.GetMasterKey()) != 32 {
		return nil
	}
	var mk [32]byte
	copy(mk[:], gc.GetMasterKey())
	id := zkgroup.DeriveGroupSecretParams(mk).GroupID
	return id[:]
}

// groupName is the stored title of a group: ok false when the group is
// not stored, "" with null true when it has no state.
func (c *RenderContext) groupName(id []byte) (name string, null, ok bool) {
	if c.Group == nil {
		return "", false, false
	}
	return c.Group(id)
}

func (c *RenderContext) plainGroupInfo(p *plain, l int, id []byte) {
	p.line(l, "Id: %s", base64.StdEncoding.EncodeToString(id))
	switch name, null, ok := c.groupName(id); {
	case !ok:
		p.line(l, "Name: <Unknown group>")
	case null:
		p.line(l, "Name: null")
	default:
		p.line(l, "Name: %s", name)
	}
}

// groupType is UPDATE for a context carrying a group change.
func groupType(gc *signalpb.AppGroupContextV2) string {
	if len(gc.GetGroupChange()) > 0 {
		return "UPDATE"
	}
	return "DELIVER"
}

func (c *RenderContext) plainDataGroup(p *plain, l int, gc *signalpb.AppGroupContextV2) {
	id := groupIDOf(gc)
	if id == nil {
		return
	}
	p.line(l, "Group info:")
	c.plainGroupInfo(p, l+1, id)
	p.line(l+1, "Revision: %d", gc.GetRevision())
	p.line(l+1, "Type: %s", groupType(gc))
}

func (c *RenderContext) jsonGroupInfo(gc *signalpb.AppGroupContextV2) *jsonObject {
	id := groupIDOf(gc)
	if id == nil {
		return nil
	}
	o := &jsonObject{}
	o.add("groupId", base64.StdEncoding.EncodeToString(id))
	if name, null, ok := c.groupName(id); ok && !null {
		o.add("groupName", name)
	} else {
		o.add("groupName", nil)
	}
	o.add("revision", gc.GetRevision())
	o.add("type", groupType(gc))
	return o
}
