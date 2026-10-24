package cli

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aren55555/signal-cli-go/internal/attachment"
	"github.com/aren55555/signal-cli-go/internal/store"
)

// runGetAttachment is `getAttachment --id ID` (attachments.md §6.4): the
// stored file, base64; recipient and group are not used.
func runGetAttachment(e *env, g *globals, id string) error {
	return withAccount(g, func(*store.Account) error {
		data, err := os.ReadFile(filepath.Join(g.dataDir, "attachments", attachment.Sanitize(id)))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return userError("Could not find attachment with ID: " + id)
		case err != nil:
			return &cliError{msg: "An error occurred reading attachment: " + id, code: 2}
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		if g.json() {
			o := &jsonObject{}
			o.add("data", b64)
			fmt.Fprintln(e.stdout, o.String())
		} else {
			fmt.Fprintln(e.stdout, b64)
		}
		return nil
	})
}
