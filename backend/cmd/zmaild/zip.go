package main

import (
	"archive/zip"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

type fileData struct {
	Name string
	Mime string
	Data []byte
	Cid  string // Content-ID of inline images
}

// collectInline downloads the parts whose Content-ID is in cids (images embedded in the body).
func collectInline(c *client.Client, uid uint32, cids map[string]bool) ([]fileData, error) {
	set := new(imap.SeqSet)
	set.AddNum(uid)
	m, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure})
	if err != nil || m == nil || m.BodyStructure == nil {
		return nil, err
	}
	var out []fileData
	var walkErr error
	m.BodyStructure.Walk(func(p []int, part *imap.BodyStructure) bool {
		id := strings.Trim(part.Id, "<>")
		if len(p) == 0 || id == "" || !cids[id] || walkErr != nil {
			return true
		}
		raw, err := fetchPart(c, uid, p)
		if err != nil {
			walkErr = err
			return false
		}
		name := partFilename(part)
		if name == "" {
			name = "image"
		}
		out = append(out, fileData{Name: name, Mime: strings.ToLower(part.MIMEType + "/" + part.MIMESubType), Data: decodeTransfer(raw, part.Encoding), Cid: id})
		return true
	})
	return out, walkErr
}

// collectAttachments downloads every attachment of a message in the currently selected folder.
// keep limits the result to the given part paths (nil = all attachments).
func collectAttachments(c *client.Client, uid uint32, keep map[string]bool) ([]fileData, error) {
	set := new(imap.SeqSet)
	set.AddNum(uid)
	m, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure})
	if err != nil {
		return nil, err
	}
	if m == nil || m.BodyStructure == nil {
		return nil, fmt.Errorf("message not found")
	}
	var out []fileData
	for _, a := range attachmentsOf(m.BodyStructure) {
		if keep != nil && !keep[a.Part] {
			continue
		}
		path, _ := parsePath(a.Part)
		var part *imap.BodyStructure
		m.BodyStructure.Walk(func(p []int, bs *imap.BodyStructure) bool {
			if pathString(p) == a.Part {
				part = bs
			}
			return part == nil
		})
		if part == nil {
			continue
		}
		raw, err := fetchPart(c, uid, path)
		if err != nil {
			return nil, err
		}
		out = append(out, fileData{Name: a.Name, Mime: a.Mime, Data: decodeTransfer(raw, part.Encoding)})
	}
	return out, nil
}

func (s *Server) handleZip(w http.ResponseWriter, r *http.Request, sess *Session) {
	q := r.URL.Query()
	uid, err := strconv.ParseUint(q.Get("uid"), 10, 32)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "No message specified")
		return
	}
	s.withAccount(w, r, sess, q.Get("account"), false, func(c *client.Client, acc Account) {
		folder, err := s.resolveFolder(c, acc.Address, r)
		if err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		if _, err := c.Select(folder, true); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		files, err := collectAttachments(c, uint32(uid), nil)
		if err != nil {
			serverErr(w, "collect attachments", err)
			return
		}
		if len(files) == 0 {
			writeErr(w, http.StatusNotFound, "The message has no attachments")
			return
		}
		for _, f := range files { // every file needs a clean verdict before the archive is made
			if !s.avGate(w, r, sess, f.Name, f.Data) {
				return
			}
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "attachments-" + time.Now().Format("2006-01-02") + ".zip"}))
		zw := zip.NewWriter(w)
		used := map[string]int{}
		for _, f := range files {
			// Backslashes count as folders in Windows unzip tools: "..\\..\\x.exe" must stay a plain name.
			base := strings.Trim(filepath.Base(strings.ReplaceAll(f.Name, "\\", "/")), ". ")
			if base == "" || base == "/" {
				base = "file"
			}
			name := uniqueName(used, base)
			// UTF-8 names (Cyrillic) are flagged in the header so Windows Explorer shows them correctly.
			fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now(), NonUTF8: false})
			if err != nil {
				return
			}
			_, _ = fw.Write(f.Data)
		}
		_ = zw.Close()
	})
}

func uniqueName(used map[string]int, name string) string {
	if name == "" || name == "." {
		name = "attachment"
	}
	used[name]++
	if n := used[name]; n > 1 {
		ext := filepath.Ext(name)
		return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n, ext)
	}
	return name
}
