package main

import (
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// imapPool keeps a few logged-in IMAP sessions per mailbox for a minute after use: a new session
// costs a TLS handshake and a login (~50 ms) on every click, the reused one a NOOP (~1 ms).
type imapPool struct {
	mu   sync.Mutex
	idle map[string][]pooledConn
}

type pooledConn struct {
	c  *client.Client
	at time.Time
}

const (
	poolIdle    = time.Minute
	poolPerUser = 3
)

var conns = &imapPool{idle: map[string][]pooledConn{}}

// get returns a live idle session for the key, or nil.
func (p *imapPool) get(key string) *client.Client {
	for {
		p.mu.Lock()
		list := p.idle[key]
		if len(list) == 0 {
			p.mu.Unlock()
			return nil
		}
		pc := list[len(list)-1]
		p.idle[key] = list[:len(list)-1]
		p.mu.Unlock()
		if time.Since(pc.at) < poolIdle && usable(pc.c) {
			pc.c.Timeout = 3 * time.Second // a live session answers in a millisecond
			ok := pc.c.Noop() == nil
			pc.c.Timeout = 2 * time.Minute
			if ok {
				return pc.c
			}
		}
		_ = pc.c.Terminate()
	}
}

// put returns a session after use; a broken one, or one too many, is closed.
func (p *imapPool) put(key string, c *client.Client) {
	if c == nil {
		return
	}
	if !usable(c) {
		_ = c.Terminate()
		return
	}
	p.mu.Lock()
	if len(p.idle[key]) < poolPerUser {
		p.idle[key] = append(p.idle[key], pooledConn{c, time.Now()})
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	_ = c.Logout()
}

func usable(c *client.Client) bool {
	st := c.State()
	return st == imap.AuthenticatedState || st == imap.SelectedState
}

// reap closes sessions idle for more than a minute.
func (p *imapPool) reap() {
	for range time.Tick(30 * time.Second) {
		var old []*client.Client
		p.mu.Lock()
		for k, list := range p.idle {
			keep := list[:0]
			for _, pc := range list {
				if time.Since(pc.at) < poolIdle {
					keep = append(keep, pc)
				} else {
					old = append(old, pc.c)
				}
			}
			if len(keep) == 0 {
				delete(p.idle, k)
			} else {
				p.idle[k] = keep
			}
		}
		p.mu.Unlock()
		for _, c := range old {
			_ = c.Logout()
		}
	}
}
