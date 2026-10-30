package main

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"
)

// Search that never hangs. Subject, sender and recipients are searched without the full-text index
// (the header cache answers in a second or two); the letters' text uses the index, which needs
// ~0.5 ms per hit — a word found in 65 000 letters takes 20–40 s. The text search gets a few
// seconds; if it is not done, the answer has the header hits and "partial", the search goes on in
// the background, and the page asks again a bit later (then the result comes from this cache).
// Further pages of the same search are cut from the cached result too.

const textBudget = 3 * time.Second

type textSearch struct {
	done    chan struct{}
	uids    []uint32
	err     error
	started time.Time
}

var (
	textSearches   = map[string]*textSearch{}
	textSearchesMu sync.Mutex
)

// withHeaderText adds "subject, from, to or cc contains text" to the criteria.
func withHeaderText(crit *imap.SearchCriteria, text string) *imap.SearchCriteria {
	h := func(name string) *imap.SearchCriteria {
		c := imap.NewSearchCriteria()
		c.Header.Add(name, text)
		return c
	}
	or := func(a, b *imap.SearchCriteria) *imap.SearchCriteria {
		c := imap.NewSearchCriteria()
		c.Or = [][2]*imap.SearchCriteria{{a, b}}
		return c
	}
	crit.Or = append(crit.Or, [2]*imap.SearchCriteria{h("Subject"), or(h("From"), or(h("To"), h("Cc")))})
	return crit
}

// searchHeaders runs criteria on a session without the full-text index (see dialSearch).
func (s *Server) searchHeaders(account, folder string, crit *imap.SearchCriteria) ([]uint32, error) {
	key := "search\x00" + account
	c := conns.get(key)
	if c == nil {
		var err error
		if c, err = s.dialSearch(account); err != nil {
			return nil, err
		}
	}
	if _, err := c.Select(folder, true); err != nil {
		_ = c.Terminate()
		return nil, err
	}
	uids, err := c.UidSearch(crit)
	if err == nil {
		conns.put(key, c)
	} else {
		_ = c.Terminate()
	}
	return uids, err
}

// quickSearch: the fast part of a search — the database index (subject, addresses and the
// letters' text) or, in a folder not indexed yet, Dovecot's headers. indexed: the index answered.
func (s *Server) quickSearch(ctx context.Context, account, folder, text string, base func() *imap.SearchCriteria, before uint32, page int) ([]uint32, bool, error) {
	if uids, ok, err := s.indexSearch(ctx, account, folder, text, base, before, page); ok && err == nil {
		return uids, true, nil
	} else if err != nil {
		log.Printf("index search %s/%s: %v", account, folder, err)
	}
	uids, err := s.searchHeaders(account, folder, withHeaderText(base(), text))
	return uids, false, err
}

// searchText: the quick hits at once, plus Dovecot's full-text search (the text of attachments,
// and of letters in folders not indexed yet) within a time budget. The index has the letters'
// text already, so with it the answer waits for Dovecot only a moment.
func (s *Server) searchText(ctx context.Context, account, folder, text string, base func() *imap.SearchCriteria, before uint32, page int) ([]uint32, bool, bool, error) {
	headers, indexed, herr := s.quickSearch(ctx, account, folder, text, base, before, page)
	budget := textBudget
	if indexed {
		budget = 0 // the attachments come with a later ask, or now if already searched
	}
	deadline := time.Now().Add(budget)
	// The same filters make the same search: the key without the page.
	key := account + "\x00" + folder + "\x00" + strings.ToLower(text) + "\x00" + critKey(base())

	textSearchesMu.Lock()
	for k, j := range textSearches { // forget old results: new mail may have come
		if time.Since(j.started) > 10*time.Minute {
			delete(textSearches, k)
		}
	}
	job := textSearches[key]
	// One text search per mailbox at a time: each reads the whole index, and two at once on a
	// small server push everything else out of memory (a one-second search took a minute).
	// A new one waits for its turn; the page keeps asking and gets it then.
	busy := false
	if job == nil {
		for k, j := range textSearches {
			if strings.HasPrefix(k, account+"\x00") && !isDone(j) {
				busy = true
				break
			}
		}
	}
	if job == nil && busy {
		textSearchesMu.Unlock()
		return headers, true, indexed, herr
	}
	if job == nil {
		job = &textSearch{done: make(chan struct{}), started: time.Now()}
		textSearches[key] = job
		go func() {
			defer close(job.done)
			if indexed {
				// The answer is already given from the index: let its letters load first.
				time.Sleep(1500 * time.Millisecond)
			}
			c, err := s.dialAs(account)
			if err != nil {
				job.err = err
				return
			}
			defer c.Logout()
			c.Timeout = 5 * time.Minute // runs on after the page got its first answer
			if _, err := c.Select(folder, true); err != nil {
				job.err = err
				return
			}
			crit := base()
			crit.Body = []string{text}
			t := time.Now()
			job.uids, job.err = c.UidSearch(crit)
			if d := time.Since(t); d > textBudget {
				log.Printf("search %s %q: text took %s", account, text, d.Round(time.Second))
			}
		}()
	}
	textSearchesMu.Unlock()

	select {
	case <-job.done:
	case <-time.After(time.Until(deadline)):
	case <-ctx.Done():
	}
	var body []uint32
	partial := false
	select {
	case <-job.done:
		if job.err != nil {
			textSearchesMu.Lock()
			if textSearches[key] == job {
				delete(textSearches, key) // try again next time
			}
			textSearchesMu.Unlock()
			if herr != nil {
				return nil, false, indexed, job.err
			}
		}
		body = job.uids
	default:
		partial = true
	}
	if herr != nil && partial {
		return nil, false, indexed, herr
	}
	seen := make(map[uint32]bool, len(headers)+len(body))
	out := make([]uint32, 0, len(headers)+len(body))
	for _, list := range [][]uint32{headers, body} {
		for _, u := range list {
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, partial, indexed, nil
}

// critKey: the filters of a search as a string (dates, flags), for the cache key.
func critKey(c *imap.SearchCriteria) string {
	return c.Since.Format("2006-01-02") + "|" + c.Before.Format("2006-01-02") + "|" + strings.Join(c.WithFlags, ",") + "|" + strings.Join(c.WithoutFlags, ",")
}

func isDone(j *textSearch) bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}
