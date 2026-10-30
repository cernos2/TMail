package main

import (
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// uidExpungeCmd is UID EXPUNGE (RFC 4315): plain EXPUNGE would also remove every other message
// that happens to carry \Deleted in the folder (e.g. marked by a phone client).
type uidExpungeCmd struct{ set *imap.SeqSet }

func (u uidExpungeCmd) Command() *imap.Command {
	return &imap.Command{Name: "UID", Arguments: []interface{}{imap.RawString("EXPUNGE"), u.set}}
}

func uidExpunge(c *client.Client, set *imap.SeqSet) error {
	if ok, _ := c.Support("UIDPLUS"); !ok {
		return c.Expunge(nil)
	}
	status, err := c.Execute(uidExpungeCmd{set}, nil)
	if err != nil {
		return err
	}
	return status.Err()
}
