// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/errcode"
	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// updateProfileOpts are updateProfile's options; nil means not given.
type updateProfileOpts struct {
	given, family, about, emoji, mobileCoin, avatar *string
	removeAvatar                                    bool
}

// runUpdateProfile is `updateProfile` (spec/service/profiles.md §4).
func runUpdateProfile(ctx context.Context, e *env, g *globals, o updateProfileOpts) error {
	return withAccount(g, func(a *store.Account) error {
		u := manager.ProfileUpdate{GivenName: o.given, FamilyName: o.family, About: o.about, AboutEmoji: o.emoji}
		if o.mobileCoin != nil {
			b, err := base64.StdEncoding.DecodeString(*o.mobileCoin)
			if err != nil {
				return errcode.IllegalArgumentCrash.Wrap(err, err)
			}
			if b == nil {
				b = []byte{}
			}
			u.MobileCoinAddress = b
		}
		switch {
		case o.avatar != nil:
			src, err := attachment.FromArgument(*o.avatar, "")
			if err != nil {
				return errcode.UpdateProfileFailed.Wrap(err, javaFileError(*o.avatar, err))
			}
			u.Avatar, u.AvatarSource = manager.AvatarNew, &src
		case o.removeAvatar:
			u.Avatar = manager.AvatarRemove
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		err = s.sender.WriteProfile(ctx, u)
		var pe *manager.ProfileError
		switch {
		case errors.Is(err, manager.ErrProfileInputTooLong):
			return errcode.ProfileInputTooLong.Wrap(err)
		case errors.As(err, &pe):
			return errcode.UpdateProfileFailed.Wrap(err, pe.Err)
		case err != nil:
			return errcode.UpdateProfileFailed.WrapText(err)
		}
		s.sender.SendFetchLatest(ctx, signalpb.AppSyncMessage_FetchLatest_LOCAL_PROFILE)
		return nil
	})
}

// javaFileError renders a file-open failure like the JDK's
// FileNotFoundException: "<path> (No such file or directory)".
func javaFileError(path string, err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		msg := pe.Err.Error()
		return fmt.Sprintf("%s (%s)", path, strings.ToUpper(msg[:1])+msg[1:])
	}
	return err.Error()
}
