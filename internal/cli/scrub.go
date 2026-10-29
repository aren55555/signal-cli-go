// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"regexp"
	"strings"
)

// --scrub-log censoring (spec/surface/cli/logging.md §6): seven rules,
// applied in order to the whole formatted text of a log event.

var (
	scrubUUID   = regexp.MustCompile(`(?i)(JOB::)?[0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12}`)
	scrubPhone  = regexp.MustCompile(`(\+|%2B|_)([0-9]{7,15})`)
	scrubEmail  = regexp.MustCompile(`\b[^\s/]+@\S+`)
	scrubGroup2 = regexp.MustCompile(`(/org/asamk/Signal/.*Groups/)?([A-Za-z0-9/_+-]{2})[A-Za-z0-9/_+-]{39}([A-Za-z0-9/_+-]{2})[=_]`)
	scrubGroup1 = regexp.MustCompile(`(/org/asamk/Signal/.*Groups/)?([A-Za-z0-9/_+-]{2})[A-Za-z0-9/_+-]{18}([A-Za-z0-9/_+-]{2})(==|__)`)
	scrubDomain = regexp.MustCompile(`(?i)(?:[a-z0-9]+\.)+([a-z0-9-]*[a-z-][a-z0-9-]*)`)
	scrubIPv4   = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)

	scrubTLDs = func() map[string]bool {
		m := map[string]bool{}
		for _, t := range strings.Fields(`com net org jp de uk fr br it ru es me gov pl ca au cn co in nl edu info eu ch id at kr cz mx be tv se tr tw al ua ir vn
			cl sk ly cc to no fi us pt dk ar hu tk gr il news ro my biz ie za nz sg ee th io xyz pe bg hk lt link ph club si site mobi by cat wiki la ga xxx cf hr ng jobs online kz ug gq
			ae is lv pro fm tips ms sa app`) {
			m[t] = true
		}
		return m
	}()
)

func scrubLog(s string) string {
	s = scrubUUID.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "JOB::") {
			return m
		}
		return "********-****-****-****-**********" + m[len(m)-2:]
	})
	s = scrubPhone.ReplaceAllStringFunc(s, func(m string) string {
		g := scrubPhone.FindStringSubmatch(m)
		d := g[2]
		return g[1] + strings.Repeat("*", len(d)-2) + d[len(d)-2:]
	})
	s = scrubEmail.ReplaceAllStringFunc(s, func(m string) string { return m[:1] + "...@..." })
	group := func(re *regexp.Regexp, stars int) {
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			g := re.FindStringSubmatch(m)
			return g[1] + g[2] + strings.Repeat("*", stars) + g[3]
		})
	}
	group(scrubGroup2, 39)
	group(scrubGroup1, 18)
	s = scrubDomain.ReplaceAllStringFunc(s, func(m string) string {
		g := scrubDomain.FindStringSubmatch(m)
		if !scrubTLDs[strings.ToLower(g[1])] || strings.HasSuffix(m, "whispersystems.org") || strings.HasSuffix(m, "signal.org") {
			return m
		}
		return "***." + g[1]
	})
	return scrubIPv4.ReplaceAllString(s, "...ipv4...")
}
