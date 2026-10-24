package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/service/provision"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// Linked-device commands (spec/surface/cli/{listDevices,addDevice,
// removeDevice,updateDevice,unregister}.md, sync.md §8).

func withSession(ctx context.Context, e *env, g *globals, f func(*session) error) error {
	return withAccount(g, func(a *store.Account) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		return f(s)
	})
}

func runListDevices(ctx context.Context, e *env, g *globals) error {
	return withSession(ctx, e, g, func(s *session) error {
		devs, err := manager.ListDevices(ctx, s.acct, s.chat, func(msg string) { fmt.Fprintln(e.stderr, "WARN  ManagerImpl - "+msg) })
		if err != nil {
			return ioError("Failed to get linked devices: %v", err)
		}
		if g.json() {
			items := make([]string, 0, len(devs))
			for _, d := range devs {
				o := &jsonObject{}
				o.add("id", d.ID)
				if d.Name != nil {
					o.add("name", *d.Name)
				} else {
					o.add("name", nil)
				}
				o.add("createdTimestamp", d.Created)
				o.add("lastSeenTimestamp", d.LastSeen)
				items = append(items, o.String())
			}
			fmt.Fprintln(e.stdout, "["+strings.Join(items, ",")+"]")
			return nil
		}
		for _, d := range devs {
			this := ""
			if d.This {
				this = " (this device)"
			}
			name := "null"
			if d.Name != nil {
				name = *d.Name
			}
			fmt.Fprintf(e.stdout, "- Device %d%s:\n  Name: %s\n  Created: %s\n  Last seen: %s\n", d.ID, this, name, ts(uint64(d.Created)), ts(uint64(d.LastSeen)))
		}
		return nil
	})
}

func runAddDevice(ctx context.Context, e *env, g *globals, uri string) error {
	return withAccount(g, func(a *store.Account) error {
		if a.Data.DeviceID != 1 {
			return userError(manager.ErrNotPrimary.Error())
		}
		if _, _, err := provision.ParseLinkURL(uri); err != nil {
			return userError("Invalid device link")
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		err = s.sender.AddDevice(ctx, uri)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, manager.ErrTooManyDevices):
			return userError(err.Error())
		}
		fmt.Fprintf(e.stderr, "ERROR AddDeviceCommand - Add device link failed: %v\n", err)
		return ioError("Add device link failed")
	})
}

func runRemoveDevice(ctx context.Context, e *env, g *globals, id int) error {
	return withAccount(g, func(a *store.Account) error {
		if a.Data.DeviceID != 1 {
			return userError(manager.ErrNotPrimary.Error())
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		if err := manager.RemoveDevice(ctx, a, s.chat, uint32(id)); err != nil {
			return ioError("Error while removing device: %v", err)
		}
		return nil
	})
}

func runUpdateDevice(ctx context.Context, e *env, g *globals, id int, name string) error {
	return withAccount(g, func(a *store.Account) error {
		if id != a.Data.DeviceID && a.Data.DeviceID != 1 {
			return userError(manager.ErrNotPrimary.Error())
		}
		if id == a.Data.DeviceID {
			// Stored only; no request (sync.md §8.4).
			s := &manager.Sender{Acct: a}
			if err := s.UpdateDeviceName(ctx, uint32(id), name); err != nil {
				return ioError("Error while updating device: %v", err)
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		s, err := openSession(ctx, e, g, a)
		if err != nil {
			return err
		}
		defer s.close()
		if err := s.sender.UpdateDeviceName(ctx, uint32(id), name); err != nil {
			return ioError("Error while updating device: %v", err)
		}
		return nil
	})
}

func runUnregister(ctx context.Context, e *env, g *globals, deleteAccount bool) error {
	if deleteAccount {
		return userError("Deleting the account is not supported by signal-cli-go yet")
	}
	return withSession(ctx, e, g, func(s *session) error {
		if err := manager.ClearPushToken(ctx, s.chat); err != nil {
			return ioError("Unregister error: %v", err)
		}
		s.acct.Data.Registered = false
		if err := s.acct.Save(); err != nil {
			return ioError("Unregister error: %v", err)
		}
		return nil
	})
}
