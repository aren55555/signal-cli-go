package cli

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
)

// The STORY block and StoryMessage JSON
// (spec/surface/cli/_received-message-output.md).

func (c *RenderContext) plainPreview(p *plain, l int, pv *signalpb.AppPreview) {
	p.line(l, "Title: %s", pv.GetTitle())
	p.line(l, "Description: %s", pv.GetDescription())
	p.line(l, "Date: %s", ts(pv.GetDate()))
	p.line(l, "Url: %s", pv.GetUrl())
	if pv.Image != nil {
		p.line(l, "Image:")
		c.plainAttachment(p, l+1, pv.Image)
	}
}

func (c *RenderContext) plainStory(p *plain, l int, st *signalpb.AppStoryMessage) {
	p.line(l, "Story: with replies: %t", st.GetAllowsReplies())
	if id := groupIDOf(st.GetGroup()); id != nil {
		p.line(l, "Group info:")
		c.plainGroupInfo(p, l+1, id)
	}
	if ta := st.GetTextAttachment(); ta != nil {
		p.line(l, "Body: %s", ta.GetText())
		if ta.Preview != nil {
			p.line(l, "Preview:")
			c.plainPreview(p, l+1, ta.Preview)
		}
	}
	if f := st.GetFileAttachment(); f != nil {
		p.line(l, "Attachments:")
		c.plainAttachment(p, l+1, f)
	}
}

// color is a #aarrggbb color string.
func color(v uint32) string { return fmt.Sprintf("#%08x", v) }

func jsonPreview(pv *signalpb.AppPreview) *jsonObject {
	po := &jsonObject{}
	po.add("url", pv.GetUrl())
	po.add("title", pv.GetTitle())
	po.add("description", pv.GetDescription())
	if pv.Image != nil {
		po.add("image", jsonAttachment(pv.Image))
	} else {
		po.add("image", nil)
	}
	return po
}

func (c *RenderContext) jsonStory(st *signalpb.AppStoryMessage) *jsonObject {
	o := &jsonObject{}
	o.add("allowsReplies", st.GetAllowsReplies())
	if id := groupIDOf(st.GetGroup()); id != nil {
		o.add("groupId", base64.StdEncoding.EncodeToString(id))
	}
	if f := st.GetFileAttachment(); f != nil {
		o.add("fileAttachment", jsonAttachment(f))
	}
	if ta := st.GetTextAttachment(); ta != nil {
		to := &jsonObject{}
		if ta.Text != nil {
			to.add("text", ta.GetText())
		} else {
			to.add("text", nil)
		}
		if ta.TextStyle != nil {
			to.add("style", ta.GetTextStyle().String())
		}
		if ta.TextForegroundColor != nil {
			to.add("textForegroundColor", color(ta.GetTextForegroundColor()))
		}
		if ta.TextBackgroundColor != nil {
			to.add("textBackgroundColor", color(ta.GetTextBackgroundColor()))
		}
		if ta.Preview != nil {
			to.add("preview", jsonPreview(ta.Preview))
		}
		if g := ta.GetGradient(); g != nil {
			gradient := &jsonObject{}
			colors := g.GetColors()
			positions := g.GetPositions()
			legacy := len(colors) == 2 && len(positions) == 2 && positions[0] == 0 && positions[1] == 1
			if legacy {
				gradient.add("startColor", color(colors[0]))
				gradient.add("endColor", color(colors[1]))
			} else {
				gradient.add("startColor", nil)
				gradient.add("endColor", nil)
			}
			cs := make([]string, len(colors))
			for i, v := range colors {
				cs[i] = color(v)
			}
			gradient.add("colors", cs)
			ps := make([]string, len(positions))
			for i, v := range positions {
				ps[i] = jsonFloat(v)
			}
			gradient.add("positions", rawJSON("["+strings.Join(ps, ",")+"]"))
			if g.Angle != nil {
				gradient.add("angle", g.GetAngle())
			} else {
				gradient.add("angle", nil)
			}
			to.add("backgroundGradient", gradient)
		}
		if _, ok := ta.Background.(*signalpb.AppTextAttachment_Color); ok {
			to.add("backgroundColor", color(ta.GetColor()))
		}
		o.add("textAttachment", to)
	}
	return o
}

// jsonFloat renders a float as Java's Jackson does for a float value.
func jsonFloat(v float32) string {
	s := fmt.Sprintf("%v", v)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}
