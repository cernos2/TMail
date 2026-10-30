package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Event is pushed to browsers over Server-Sent Events.
type Event struct {
	Type    string    `json:"type"` // new | counts | ping
	Account string    `json:"account,omitempty"`
	Message *Summary  `json:"message,omitempty"`
	Event   *CalEvent `json:"event,omitempty"` // type "reminder"
}

type subscriber struct {
	user     int64
	accounts map[string]bool
	ch       chan Event
}

// Hub runs one IMAP IDLE watcher per mailbox that has at least one open browser tab,
// and fans its events out to those tabs.
type Hub struct {
	s        *Server
	mu       sync.Mutex
	subs     map[*subscriber]struct{}
	watchers map[string]*watcher
}

type watcher struct {
	refs   int
	cancel context.CancelFunc
	timer  *time.Timer // delayed stop after the last tab closes
}

func newHub(s *Server) *Hub {
	return &Hub{s: s, subs: map[*subscriber]struct{}{}, watchers: map[string]*watcher{}}
}

func (h *Hub) subscribe(user int64, accounts []string) *subscriber {
	sub := &subscriber{user: user, accounts: map[string]bool{}, ch: make(chan Event, 64)}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[sub] = struct{}{}
	for _, a := range accounts {
		sub.accounts[a] = true
		w := h.watchers[a]
		if w == nil {
			ctx, cancel := context.WithCancel(context.Background())
			w = &watcher{cancel: cancel}
			h.watchers[a] = w
			go h.watch(ctx, a)
		}
		if w.timer != nil {
			w.timer.Stop()
			w.timer = nil
		}
		w.refs++
	}
	return sub
}

func (h *Hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, sub)
	for a := range sub.accounts {
		w := h.watchers[a]
		if w == nil {
			continue
		}
		w.refs--
		if w.refs <= 0 {
			acc := a
			// Keep the IDLE connection briefly: page reloads reconnect within seconds.
			w.timer = time.AfterFunc(time.Minute, func() {
				h.mu.Lock()
				defer h.mu.Unlock()
				if cur := h.watchers[acc]; cur == w && w.refs <= 0 {
					w.cancel()
					delete(h.watchers, acc)
				}
			})
		}
	}
}

func (h *Hub) publish(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		if ev.Account != "" && !sub.accounts[ev.Account] {
			continue
		}
		select {
		case sub.ch <- ev:
		default: // slow client: drop; it refreshes counts on the next event
		}
	}
}

// publishUser sends an event to every open tab of one user and reports how many got it.
func (h *Hub) publishUser(user int64, ev Event) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for sub := range h.subs {
		if sub.user != user {
			continue
		}
		select {
		case sub.ch <- ev:
			n++
		default:
		}
	}
	return n
}

// countsChanged tells tabs to refresh unread counters (after reading, moving, sending…).
func (h *Hub) countsChanged(account string) { h.publish(Event{Type: "counts", Account: account}) }

func (h *Hub) watch(ctx context.Context, account string) {
	backoff := 2 * time.Second
	var next, validity uint32 // kept across reconnects: letters that came meanwhile are announced too
	for ctx.Err() == nil {
		start := time.Now()
		err := h.watchOnce(ctx, account, &next, &validity)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > time.Minute {
			backoff = 2 * time.Second
		}
		slog.Warn("idle watcher reconnecting", "account", account, "err", err, "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (h *Hub) watchOnce(ctx context.Context, account string, next, validity *uint32) error {
	c, err := h.s.dialAs(account)
	if err != nil {
		return err
	}
	defer c.Logout()
	updates := make(chan client.Update, 64)
	c.Updates = updates

	mbox, err := c.Select("INBOX", true)
	if err != nil {
		return err
	}
	// Letters that came while the connection was being restored. Not after a long break or a
	// rebuilt folder (an import): then there would be a flood of old news.
	if *validity == mbox.UidValidity && *next > 0 && *next < mbox.UidNext && mbox.UidNext-*next <= 20 {
		if n, err := h.announceNew(c, account, *next); err == nil {
			*next = n
		}
	}
	if *validity != mbox.UidValidity || *next < mbox.UidNext {
		*next = mbox.UidNext
	}
	*validity = mbox.UidValidity
	// After a reconnect, tabs may have missed changes.
	h.countsChanged(account)

	for {
		stop := make(chan struct{})
		idleDone := make(chan error, 1)
		go func() {
			// IDLE is renewed before the connection's 2-minute command timeout (c.Timeout) runs out:
			// with a longer one the connection was cut every few minutes and letters slipped by.
			idleDone <- c.Idle(stop, &client.IdleOptions{LogoutTimeout: 100 * time.Second, PollInterval: time.Minute})
		}()
		var changed bool
		select {
		case <-ctx.Done():
			close(stop)
			<-idleDone
			return nil
		case err := <-idleDone:
			return fmt.Errorf("idle ended: %w", err)
		case u := <-updates:
			changed = isMailboxChange(u)
			close(stop)
			if err := <-idleDone; err != nil {
				return err
			}
		}
		// Drain queued updates.
		for drained := false; !drained; {
			select {
			case u := <-updates:
				changed = changed || isMailboxChange(u)
			default:
				drained = true
			}
		}
		if !changed {
			continue
		}
		newNext, err := h.announceNew(c, account, *next)
		if err != nil {
			return err
		}
		*next = newNext
		h.countsChanged(account)
	}
}

func isMailboxChange(u client.Update) bool {
	switch u.(type) {
	case *client.MailboxUpdate, *client.ExpungeUpdate, *client.MessageUpdate:
		return true
	}
	return false
}

// announceNew publishes summaries of messages with UID >= next and returns the new UIDNEXT.
func (h *Hub) announceNew(c *client.Client, account string, next uint32) (uint32, error) {
	crit := imap.NewSearchCriteria()
	set := new(imap.SeqSet)
	set.AddRange(next, 0) // next:*
	crit.Uid = set
	uids, err := c.UidSearch(crit)
	if err != nil {
		return next, err
	}
	var fresh []uint32
	for _, u := range uids {
		if u >= next { // "next:*" also matches the last message when nothing is new
			fresh = append(fresh, u)
		}
	}
	if len(fresh) == 0 {
		return next, nil
	}
	msgs, err := fetchSummaries(c, account, "INBOX", fresh)
	if err != nil {
		return next, err
	}
	// A mailbox with "hidden" folders is sorted before anything is announced: their letters must
	// not flash in the Inbox. Letters filed into an ordinary folder are announced with that folder.
	var movedTo map[uint32]string
	hidden := h.s.hiddenFolders(context.Background(), account)
	if len(hidden) > 0 {
		// The new letters themselves by sender rules first: the sorter goes through the Inbox in
		// order and may still be busy with older letters (a big import), and a letter for a hidden
		// folder must not be announced meanwhile.
		movedTo = h.s.fileBySenderRules(account, msgs)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		more, _ := h.s.sortInbox(ctx, account)
		cancel()
		for u, f := range more {
			movedTo[u] = f
		}
	}
	for i := len(msgs) - 1; i >= 0; i-- { // oldest first
		m := msgs[i]
		if m.UID >= next {
			next = m.UID + 1
		}
		if f, ok := movedTo[m.UID]; ok {
			if !hidden[f] {
				m.Folder = f
				h.publish(Event{Type: "filed", Account: account, Message: &m})
			}
			continue
		}
		h.publish(Event{Type: "new", Account: account, Message: &m})
	}
	return next, nil
}

// handleEvents streams events for all mailboxes the user can open.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, sess *Session) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "The event stream is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	accounts := make([]string, len(sess.Accounts))
	for i, a := range sess.Accounts {
		accounts[i] = a.Address
	}
	sub := s.hub.subscribe(sess.UserID, accounts)
	defer s.hub.unsubscribe(sub)

	write := func(ev Event) bool {
		b, _ := json.Marshal(ev)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	fmt.Fprint(w, "retry: 3000\n\n")
	if !write(Event{Type: "ready"}) {
		return
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub.ch:
			if !write(ev) {
				return
			}
		case <-ping.C:
			if !write(Event{Type: "ping"}) {
				return
			}
		}
	}
}
