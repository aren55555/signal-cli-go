package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// runSendStory is `sendStory` (spec/surface/cli/sendStory.md,
// stories.md §2).
func runSendStory(ctx context.Context, e *env, g *globals, path string, noReplies bool, group string) error {
	return withAccount(g, func(a *store.Account) error {
		if path == "" {
			return userError("An attachment is required for sending a story.")
		}
		var gid []byte
		if group != "" {
			var err error
			if gid, err = parseGroupArg(group); err != nil {
				return err
			}
		}
		src, err := attachmentSource(path, g)
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
		var ts uint64
		var results []manager.SendResult
		if gid != nil {
			ts, results, err = s.groups.SendStory(ctx, gid, src, !noReplies)
			if errors.Is(err, manager.ErrGroupNotFound) || errors.Is(err, manager.ErrNotAMember) {
				return groupSendError(s, gid, err)
			}
		} else {
			ts, results, err = s.sender.SendStory(ctx, src, !noReplies)
		}
		switch {
		case errors.Is(err, manager.ErrStoryMedia):
			return &cliError{msg: fmt.Sprintf("Failed to send story: %s: %v (AttachmentInvalidException)", path, err), code: 2}
		case err != nil:
			return &cliError{msg: fmt.Sprintf("Failed to send story: %v (IOException)", err), code: 2}
		}
		var out []sendResult
		for _, r := range results {
			key := a.Recipients[r.Dest].Number
			if key == "" {
				key = r.Dest
			}
			out = append(out, resultFor(key, r.Dest, r.Err))
		}
		return finishSend(e, g, ts, out)
	})
}
