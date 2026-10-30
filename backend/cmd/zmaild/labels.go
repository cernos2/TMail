package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Labels: names and colours in the database, the marks themselves as IMAP keywords on letters.

type label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func labelKeyword(id int64) string { return "$tl_" + strconv.FormatInt(id, 10) }

func labelOfKeyword(f string) (int64, bool) {
	rest, ok := strings.CutPrefix(strings.ToLower(f), "$tl_")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	return id, err == nil && id > 0
}

var labelColors = map[string]bool{"blue": true, "green": true, "yellow": true, "red": true, "purple": true, "gray": true, "teal": true, "orange": true}

func (s *Server) handleLabels(w http.ResponseWriter, r *http.Request, sess *Session) {
	rows, err := s.db.Query(r.Context(), `SELECT id, name, color FROM labels ORDER BY sort, name`)
	if err != nil {
		serverErr(w, "labels", err)
		return
	}
	defer rows.Close()
	out := []label{}
	for rows.Next() {
		var l label
		if rows.Scan(&l.ID, &l.Name, &l.Color) == nil {
			out = append(out, l)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleLabelSave(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in label
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid data")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 40 {
		writeErr(w, http.StatusBadRequest, "Label name: 1 to 40 characters")
		return
	}
	if !labelColors[in.Color] {
		in.Color = "blue"
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var err error
	if id == 0 {
		err = s.db.QueryRow(r.Context(), `INSERT INTO labels(name, color, created_by, sort) VALUES ($1,$2,$3,(SELECT COALESCE(max(sort),0)+10 FROM labels)) RETURNING id`,
			in.Name, in.Color, sess.Login).Scan(&in.ID)
	} else {
		in.ID = id
		_, err = s.db.Exec(r.Context(), `UPDATE labels SET name=$2, color=$3 WHERE id=$1`, id, in.Name, in.Color)
	}
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			writeErr(w, http.StatusConflict, "This label already exists")
			return
		}
		serverErr(w, "label save", err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// Deleting a label removes it from the list; the keyword stays on letters but is no longer shown.
func (s *Server) handleLabelDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	_, _ = s.db.Exec(r.Context(), `DELETE FROM labels WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// searchAllFolders runs one search over the mailbox's folders (except Trash, Junk, Drafts),
// newest first.
func (s *Server) searchAllFolders(c *client.Client, account string, crit *imap.SearchCriteria, limit int) ([]Summary, error) {
	folders, err := listFolders(c, false)
	if err != nil {
		return nil, err
	}
	all := []Summary{}
	for _, f := range folders {
		if f.Role == "trash" || f.Role == "junk" || f.Role == "drafts" {
			continue
		}
		if _, err := c.Select(f.Name, true); err != nil {
			continue
		}
		uids, err := c.UidSearch(crit)
		if err != nil || len(uids) == 0 {
			continue
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
		if len(uids) > limit {
			uids = uids[:limit]
		}
		msgs, err := fetchSummaries(c, account, f.Name, uids)
		if err != nil {
			continue
		}
		all = append(all, msgs...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Date.After(all[j].Date) })
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}
