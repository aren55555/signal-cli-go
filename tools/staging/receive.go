// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aren55555/signal-cli-go/internal/manager"
	"github.com/aren55555/signal-cli-go/internal/proto/signalpb"
	"github.com/aren55555/signal-cli-go/internal/service/chat"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// receive connects as the stored account, decrypts what arrives, and acks
// each envelope only after its state is saved. Undecryptable envelopes are
// left unacked for a later attempt. It stops at queue-empty plus -linger.
func receive(args []string) {
	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	data := fs.String("data", ".data/go", "data directory")
	linger := fs.Duration("linger", 5*time.Second, "keep listening this long after queue-empty")
	fs.Parse(args)
	a := onlyAccount(*data)
	defer a.Close()
	d := &manager.Decryptor{Acct: a}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c, err := chat.Dial(ctx, chat.Endpoint{
		Proxy:          envProxy(chat.StagingHost).Proxy,
		Credentials:    &chat.Credentials{ACI: a.Data.ACI, DeviceID: a.Data.DeviceID, Password: a.Data.Password},
		ReceiveStories: true,
		KeepAlive:      chat.KeepAlive,
	})
	if err != nil {
		log.Fatalf("connect: %s", mask(err.Error()))
	}
	defer c.Close()
	fmt.Printf("connected as device %d\n", a.Data.DeviceID)
	var stop <-chan time.Time
	for {
		wctx, wcancel := context.WithCancel(ctx)
		if stop != nil {
			go func(s <-chan time.Time) {
				select {
				case <-s:
					wcancel()
				case <-wctx.Done():
				}
			}(stop)
		}
		r, err := c.Next(wctx)
		wcancel()
		if err != nil {
			if stop != nil {
				fmt.Println("done")
				return
			}
			log.Fatalf("next: %v", err)
		}
		if r.Kind == chat.KindQueueEmpty {
			fmt.Println("queue empty")
			stop = time.After(*linger)
			continue
		}
		var env signalpb.ServiceEnvelope
		if err := proto.Unmarshal(r.Body, &env); err != nil {
			fmt.Printf("unparsable envelope (%v), not acked\n", err)
			continue
		}
		m, err := d.Decrypt(&env)
		if err != nil {
			fmt.Printf("envelope type %d from device %d: %v (not acked)\n", env.GetType(), env.GetSourceDeviceId(), err)
			continue
		}
		if err := manager.ApplyContent(a, m, time.Now()); err != nil {
			log.Fatalf("store: %v", err)
		}
		if err := r.Ack(); err != nil {
			log.Fatalf("ack: %v", err)
		}
		if m.ServerReceipt {
			fmt.Printf("server delivery receipt for %d\n", m.ClientTimestamp)
			continue
		}
		self := m.Sender.ServiceID == a.Data.ACI
		fmt.Printf("envelope type %d, sealed=%v, from %s device %d, %d-byte content: %s, profile key stored: %v, to PNI: %v, inner type %d\n",
			env.GetType(), m.Sealed, map[bool]string{true: "self", false: "peer"}[self], m.Sender.DeviceID, len(m.Content), describe(m.Content), len(a.Recipients[m.Sender.ServiceID].ProfileKey) == 32, m.ToPNI, m.InnerType)
	}
}

// describe names the Content fields present, by number, without printing
// any message text.
func describe(b []byte) string {
	names := map[protowire.Number]string{1: "dataMessage", 2: "syncMessage", 3: "callMessage", 4: "nullMessage", 5: "receiptMessage",
		6: "typingMessage", 7: "senderKeyDistribution", 8: "decryptionErrorMessage", 9: "storyMessage", 10: "pniSignatureMessage", 11: "editMessage"}
	var out []string
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "malformed"
		}
		b = b[n:]
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			return "malformed"
		}
		if name, ok := names[num]; ok {
			out = append(out, name)
		} else {
			out = append(out, fmt.Sprintf("field%d", num))
		}
		b = b[m:]
	}
	return fmt.Sprint(out)
}
