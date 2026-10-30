package main

import (
	"crypto/tls"
	"fmt"
	"github.com/emersion/go-imap/responses"
	"github.com/emersion/go-imap/utf7"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// dialAs opens an IMAP session to the given mailbox through the Dovecot master user.
// Access rights are checked by the caller against the session's account list.
func (s *Server) dialAs(account string) (*client.Client, error) {
	c, err := client.DialTLS(s.cfg.IMAPAddr, &tls.Config{ServerName: s.cfg.Hostname})
	if err != nil {
		return nil, err
	}
	c.Timeout = 2 * time.Minute
	if err := c.Login(account+"*"+s.cfg.MasterUser, s.cfg.MasterPass); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("login %s: %w", account, err)
	}
	return c, nil
}

// dialSearch opens a session for searches by address: the "zmail-search" master login has no
// full-text index (Dovecot userdb override), so FROM/TO/CC are answered from the header cache in
// ~2 s even when the address is in 65 000 letters (the index takes a minute for that).
func (s *Server) dialSearch(account string) (*client.Client, error) {
	c, err := client.DialTLS(s.cfg.IMAPAddr, &tls.Config{ServerName: s.cfg.Hostname})
	if err != nil {
		return nil, err
	}
	c.Timeout = 2 * time.Minute
	if err := c.Login(account+"*zmail-search", s.cfg.MasterPass); err != nil {
		_ = c.Logout()
		return nil, fmt.Errorf("login %s: %w", account, err)
	}
	return c, nil
}

// reAddressQuery: a search for an address or a domain (ivan@firma.ru, @firma.ru, firma.ru).
var reAddressQuery = regexp.MustCompile(`^@?([\pL\d._%+-]+@)?[\pL\d-]+(\.[\pL\d-]+)*\.[\pL]{2,}$`)

// withAccount checks access, opens IMAP and closes it after fn.
func (s *Server) withAccount(w http.ResponseWriter, r *http.Request, sess *Session, account string, needSend bool, fn func(c *client.Client, acc Account)) {
	acc, ok := sess.account(account)
	if !ok {
		writeErr(w, http.StatusForbidden, fmt.Sprintf("No access to mailbox %s", account))
		return
	}
	if needSend && !acc.CanSend {
		writeErr(w, http.StatusForbidden, fmt.Sprintf("Mailbox %s is read-only for you", account))
		return
	}
	key := "app\x00" + acc.Address
	c := conns.get(key)
	if c == nil {
		var err error
		if c, err = s.dialAs(acc.Address); err != nil {
			serverErr(w, "imap dial", err)
			return
		}
	}
	fn(c, acc)
	// Kept for the next click, unless the reader left mid-way (a download cut off may leave a
	// command unfinished on the session).
	if r.Context().Err() == nil {
		conns.put(key, c)
	} else {
		_ = c.Terminate()
	}
}

// ---------- folders ----------

type Folder struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Unseen uint32 `json:"unseen"`
	Total  uint32 `json:"total"`
}

var roleOrder = map[string]int{"inbox": 0, "drafts": 1, "sent": 2, "archive": 3, "junk": 4, "trash": 5, "": 6}

func roleOf(info *imap.MailboxInfo) string {
	if strings.EqualFold(info.Name, "INBOX") {
		return "inbox"
	}
	for _, a := range info.Attributes {
		switch a {
		case imap.SentAttr:
			return "sent"
		case imap.DraftsAttr:
			return "drafts"
		case imap.JunkAttr:
			return "junk"
		case imap.TrashAttr:
			return "trash"
		case imap.ArchiveAttr:
			return "archive"
		}
	}
	return ""
}

func listFolders(c *client.Client, withCounts bool) ([]Folder, error) {
	ch := make(chan *imap.MailboxInfo, 64)
	done := make(chan error, 1)
	go func() { done <- c.List("", "*", ch) }()
	var out []Folder
	for info := range ch {
		if hasAttr(info.Attributes, imap.NoSelectAttr) {
			continue
		}
		out = append(out, Folder{Name: info.Name, Role: roleOf(info)})
	}
	if err := <-done; err != nil {
		return nil, err
	}
	if withCounts {
		if counts, err := listStatus(c); err == nil { // one command for every folder
			for i := range out {
				if st, ok := counts[out[i].Name]; ok {
					out[i].Total, out[i].Unseen = st.Messages, st.Unseen
				}
			}
			withCounts = false
		}
	}
	if withCounts { // a server without LIST-STATUS: folder by folder
		for i := range out {
			st, err := c.Status(out[i].Name, []imap.StatusItem{imap.StatusMessages, imap.StatusUnseen})
			if err != nil {
				return nil, err
			}
			out[i].Total, out[i].Unseen = st.Messages, st.Unseen
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := roleOrder[out[i].Role], roleOrder[out[j].Role]
		if ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// listStatus asks the counts of all folders at once (RFC 5819 LIST-STATUS): a STATUS per folder
// was a round trip each, ~0.7 s for a mailbox with many folders.
func listStatus(c *client.Client) (map[string]*imap.MailboxStatus, error) {
	out := map[string]*imap.MailboxStatus{}
	cmd := &imap.Command{Name: "LIST", Arguments: []interface{}{"", "*", imap.RawString("RETURN (STATUS (MESSAGES UNSEEN))")}}
	h := responses.HandlerFunc(func(resp imap.Resp) error {
		name, fields, ok := imap.ParseNamedResp(resp)
		if !ok {
			return responses.ErrUnhandled
		}
		switch name {
		case "LIST":
			return nil // the folders themselves come from the plain LIST
		case "STATUS":
			if len(fields) < 2 {
				return nil
			}
			raw, err := imap.ParseString(fields[0])
			if err != nil {
				return nil
			}
			mbox, err := utf7.Encoding.NewDecoder().String(raw)
			if err != nil {
				return nil
			}
			items, _ := fields[1].([]interface{})
			st := imap.NewMailboxStatus(mbox, nil)
			if st.Parse(items) == nil {
				out[mbox] = st
			}
			return nil
		}
		return responses.ErrUnhandled
	})
	status, err := c.Execute(cmd, h)
	if err == nil {
		err = status.Err()
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func hasAttr(attrs []string, a string) bool {
	for _, x := range attrs {
		if strings.EqualFold(x, a) {
			return true
		}
	}
	return false
}

// folderCache remembers special-use folder names per account (they rarely change).
type folderCache struct {
	mu sync.Mutex
	m  map[string]map[string]string
}

func newFolderCache() *folderCache { return &folderCache{m: map[string]map[string]string{}} }

func (fc *folderCache) byRole(c *client.Client, account, role string) (string, error) {
	if role == "inbox" {
		return "INBOX", nil
	}
	fc.mu.Lock()
	name, ok := fc.m[account][role]
	fc.mu.Unlock()
	if ok {
		return name, nil
	}
	folders, err := listFolders(c, false)
	if err != nil {
		return "", err
	}
	roles := map[string]string{}
	for _, f := range folders {
		if f.Role != "" {
			roles[f.Role] = f.Name
		}
	}
	fc.mu.Lock()
	fc.m[account] = roles
	fc.mu.Unlock()
	if name, ok := roles[role]; ok {
		return name, nil
	}
	return "", fmt.Errorf("the mailbox has no folder with the role %s", role)
}

func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request, sess *Session) {
	type accFolders struct {
		Account string   `json:"account"`
		Folders []Folder `json:"folders"`
		Error   string   `json:"error,omitempty"`
	}
	out := make([]accFolders, len(sess.Accounts))
	var wg sync.WaitGroup
	for i, a := range sess.Accounts {
		wg.Add(1)
		go func(i int, a Account) {
			defer wg.Done()
			out[i].Account = a.Address
			key := "app\x00" + a.Address
			c := conns.get(key)
			if c == nil {
				var err error
				if c, err = s.dialAs(a.Address); err != nil {
					out[i].Error = "The mailbox is temporarily unavailable"
					return
				}
			}
			f, err := listFolders(c, true)
			if err != nil {
				_ = c.Terminate()
				out[i].Error = "Could not get the folders"
				return
			}
			conns.put(key, c)
			out[i].Folders = f
		}(i, a)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out})
}
