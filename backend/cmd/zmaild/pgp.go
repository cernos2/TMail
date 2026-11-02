package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	gomsg "github.com/emersion/go-message"
)

// OpenPGP: signed and encrypted letters (PGP/MIME, RFC 3156), as Thunderbird, Proton Mail,
// Outlook with gpg4o and K-9/Thunderbird for Android write them. Every mailbox may have a key
// pair; its private part is kept on the server, sealed with the server's secret, so the
// webmail can sign, encrypt and read without the user handling key files. Correspondents' public
// keys come from their letters (the Autocrypt header, attached keys) or are added by hand.

var pgp = crypto.PGP()

type pgpStatus struct {
	Encrypted bool   `json:"encrypted"`
	Decrypted bool   `json:"decrypted"`
	Signed    bool   `json:"signed"`
	Verified  bool   `json:"verified"`
	Signer    string `json:"signer,omitempty"`  // fingerprint of the key that signed
	Problem   string `json:"problem,omitempty"` // nokey (no key to decrypt) | unknown (signer's key unknown) | bad (signature wrong) | broken
}

// ---------- keys ----------

func (s *Server) mailboxKey(ctx context.Context, mailbox string) (*crypto.Key, error) {
	var enc []byte
	if err := s.db.QueryRow(ctx, `SELECT private_enc FROM pgp_keys WHERE mailbox=$1`, strings.ToLower(mailbox)).Scan(&enc); err != nil {
		return nil, err
	}
	armored, err := s.unseal(enc)
	if err != nil {
		return nil, err
	}
	return crypto.NewKeyFromArmored(armored)
}

// publicKeyOf: a correspondent's key, or our own mailbox's public key.
func (s *Server) publicKeyOf(ctx context.Context, email string) (*crypto.Key, error) {
	var armored string
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errNotFound
	}
	err := s.db.QueryRow(ctx, `SELECT public_key FROM pgp_keys WHERE mailbox=$1 UNION ALL SELECT public_key FROM pgp_contacts WHERE email=$1 LIMIT 1`, email).Scan(&armored)
	if err != nil {
		return nil, err
	}
	return crypto.NewKeyFromArmored(armored)
}

type pgpKeyInfo struct {
	Mailbox     string    `json:"mailbox"`
	Fingerprint string    `json:"fingerprint"`
	SignDefault bool      `json:"signDefault"`
	Created     time.Time `json:"created"`
	CreatedBy   string    `json:"createdBy"`
}

type pgpContact struct {
	Email       string    `json:"email"`
	Fingerprint string    `json:"fingerprint"`
	Source      string    `json:"source"`
	Updated     time.Time `json:"updated"`
}

// GET /api/pgp — keys of the user's mailboxes and the correspondents' keys.
func (s *Server) handlePGP(w http.ResponseWriter, r *http.Request, sess *Session) {
	ctx := r.Context()
	keys := []pgpKeyInfo{}
	if rows, err := s.db.Query(ctx, `SELECT mailbox, fingerprint, sign_default, created_at, created_by FROM pgp_keys WHERE mailbox = ANY($1) ORDER BY mailbox`, userAccounts(sess)); err == nil {
		for rows.Next() {
			var k pgpKeyInfo
			if rows.Scan(&k.Mailbox, &k.Fingerprint, &k.SignDefault, &k.Created, &k.CreatedBy) == nil {
				keys = append(keys, k)
			}
		}
		rows.Close()
	}
	contacts := []pgpContact{}
	if rows, err := s.db.Query(ctx, `SELECT email, fingerprint, source, updated_at FROM pgp_contacts ORDER BY email LIMIT 2000`); err == nil {
		for rows.Next() {
			var c pgpContact
			if rows.Scan(&c.Email, &c.Fingerprint, &c.Source, &c.Updated) == nil {
				contacts = append(contacts, c)
			}
		}
		rows.Close()
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys, "contacts": contacts})
}

// POST /api/pgp/keys {mailbox, action: generate | import, armored, passphrase}
func (s *Server) handlePGPKeySave(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Mailbox    string `json:"mailbox"`
		Action     string `json:"action"`
		Armored    string `json:"armored"`
		Passphrase string `json:"passphrase"`
	}
	if err := json_read(r, &in, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	acc, ok := sess.account(in.Mailbox)
	if !ok || !acc.CanSend {
		writeErr(w, http.StatusForbidden, fmt.Sprintf("You cannot send from the address %s", in.Mailbox))
		return
	}
	var key *crypto.Key
	var err error
	switch in.Action {
	case "generate":
		key, err = pgp.KeyGeneration().AddUserId(acc.Name, acc.Address).New().GenerateKey()
		if err != nil {
			serverErr(w, "pgp generate", err)
			return
		}
	case "import":
		key, err = crypto.NewKeyFromArmored(strings.TrimSpace(in.Armored))
		if err != nil || !key.IsPrivate() {
			writeErr(w, http.StatusBadRequest, "This is not a private OpenPGP key (it starts with -----BEGIN PGP PRIVATE KEY BLOCK-----)")
			return
		}
		if locked, _ := key.IsLocked(); locked {
			if key, err = key.Unlock([]byte(in.Passphrase)); err != nil {
				writeErr(w, http.StatusBadRequest, "Wrong passphrase of the key")
				return
			}
		}
		if !keyHasEmail(key, acc.Address) {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("The key is not for %s", acc.Address))
			return
		}
	default:
		writeErr(w, http.StatusBadRequest, "Unknown action")
		return
	}
	priv, err := key.Armor()
	if err != nil {
		serverErr(w, "pgp armor", err)
		return
	}
	pub, err := key.GetArmoredPublicKey()
	if err != nil {
		serverErr(w, "pgp public", err)
		return
	}
	sealed, err := s.seal(priv)
	if err != nil {
		serverErr(w, "pgp seal", err)
		return
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO pgp_keys(mailbox, fingerprint, public_key, private_enc, created_by) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (mailbox) DO UPDATE SET fingerprint=EXCLUDED.fingerprint, public_key=EXCLUDED.public_key, private_enc=EXCLUDED.private_enc,
			created_by=EXCLUDED.created_by, created_at=now()`, acc.Address, strings.ToUpper(key.GetFingerprint()), pub, sealed, sess.Login); err != nil {
		serverErr(w, "pgp save", err)
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "pgp.key_"+in.Action, acc.Address, map[string]any{"fingerprint": key.GetFingerprint()})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "fingerprint": strings.ToUpper(key.GetFingerprint())})
}

func json_read(r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, limit)
	return readJSON(r, v)
}

func keyHasEmail(key *crypto.Key, email string) bool {
	for _, id := range key.GetEntity().Identities {
		if id.UserId != nil && strings.EqualFold(id.UserId.Email, email) {
			return true
		}
	}
	return false
}

// PUT /api/pgp/keys {mailbox, signDefault}
func (s *Server) handlePGPKeySettings(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Mailbox     string `json:"mailbox"`
		SignDefault bool   `json:"signDefault"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if acc, ok := sess.account(in.Mailbox); !ok || !acc.CanSend {
		writeErr(w, http.StatusForbidden, "No access")
		return
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE pgp_keys SET sign_default=$2 WHERE mailbox=$1`, strings.ToLower(in.Mailbox), in.SignDefault)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /api/pgp/keys?mailbox=
func (s *Server) handlePGPKeyDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	mb := strings.ToLower(r.URL.Query().Get("mailbox"))
	if acc, ok := sess.account(mb); !ok || !acc.CanSend {
		writeErr(w, http.StatusForbidden, "No access")
		return
	}
	_, _ = s.db.Exec(r.Context(), `DELETE FROM pgp_keys WHERE mailbox=$1`, mb)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "pgp.key_delete", mb, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/pgp/public?mailbox= | ?email= — a public key as an .asc file.
func (s *Server) handlePGPPublic(w http.ResponseWriter, r *http.Request, sess *Session) {
	email := strings.ToLower(firstNonEmpty(r.URL.Query().Get("mailbox"), r.URL.Query().Get("email")))
	key, err := s.publicKeyOf(r.Context(), email)
	if err != nil {
		writeErr(w, http.StatusNotFound, "No key")
		return
	}
	pub, _ := key.GetArmoredPublicKey()
	w.Header().Set("Content-Type", "application/pgp-keys")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": email + ".asc"}))
	_, _ = io.WriteString(w, pub)
}

// POST /api/pgp/contacts {armored} — a correspondent's public key (also from an attachment:
// {account, folder, uid, part}).
func (s *Server) handlePGPContactAdd(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Armored string `json:"armored"`
		Account string `json:"account"`
		Folder  string `json:"folder"`
		UID     uint32 `json:"uid"`
		Part    string `json:"part"`
	}
	if err := json_read(r, &in, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	source := "manual"
	if in.Armored == "" && in.UID > 0 {
		if _, ok := sess.account(in.Account); !ok {
			writeErr(w, http.StatusForbidden, "No access")
			return
		}
		path, ok := parsePath(in.Part)
		if !ok {
			writeErr(w, http.StatusBadRequest, "Invalid request")
			return
		}
		c, err := s.dialAs(in.Account)
		if err != nil {
			serverErr(w, "pgp contact dial", err)
			return
		}
		defer c.Logout()
		if _, err := c.Select(in.Folder, true); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		raw, err := fetchPart(c, in.UID, path)
		if err != nil {
			writeErr(w, http.StatusNotFound, "Attachment not found")
			return
		}
		in.Armored = string(decodeTransferGuess(raw))
		source = "attachment"
	}
	emails, err := s.saveContactKeys(r.Context(), in.Armored, source)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "pgp.contact_add", strings.Join(emails, ","), nil)
	writeJSON(w, http.StatusOK, map[string]any{"emails": emails})
}

// decodeTransferGuess: an attached key is usually plain text or base64.
func decodeTransferGuess(raw []byte) []byte {
	if bytes.Contains(raw, []byte("-----BEGIN PGP")) {
		return raw
	}
	if b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(raw)), "")); err == nil {
		return b
	}
	return raw
}

// saveContactKeys stores a public key under every address it names.
func (s *Server) saveContactKeys(ctx context.Context, armored, source string) ([]string, error) {
	var key *crypto.Key
	var err error
	if strings.Contains(armored, "-----BEGIN PGP") {
		key, err = crypto.NewKeyFromArmored(strings.TrimSpace(armored))
	} else {
		key, err = crypto.NewKey([]byte(armored))
	}
	if err != nil {
		return nil, errors.New("This is not an OpenPGP public key")
	}
	pubKey, err := key.ToPublic()
	if err != nil {
		return nil, errors.New("This is not an OpenPGP public key")
	}
	pub, err := pubKey.GetArmoredPublicKey()
	if err != nil {
		return nil, err
	}
	var emails []string
	for _, id := range key.GetEntity().Identities {
		if id.UserId == nil || id.UserId.Email == "" {
			continue
		}
		e := strings.ToLower(id.UserId.Email)
		if _, err := s.db.Exec(ctx, `INSERT INTO pgp_contacts(email, fingerprint, public_key, source) VALUES ($1,$2,$3,$4)
			ON CONFLICT (email) DO UPDATE SET fingerprint=EXCLUDED.fingerprint, public_key=EXCLUDED.public_key, source=EXCLUDED.source, updated_at=now()`,
			e, strings.ToUpper(key.GetFingerprint()), pub, source); err == nil {
			emails = append(emails, e)
		}
	}
	if len(emails) == 0 {
		return nil, errors.New("The key names no e-mail address")
	}
	return emails, nil
}

// DELETE /api/pgp/contacts?email=
func (s *Server) handlePGPContactDelete(w http.ResponseWriter, r *http.Request, sess *Session) {
	e := strings.ToLower(r.URL.Query().Get("email"))
	_, _ = s.db.Exec(r.Context(), `DELETE FROM pgp_contacts WHERE email=$1`, e)
	s.audit(r.Context(), sess.UserID, sess.Login, clientIP(r), "pgp.contact_delete", e, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/pgp/status?from=&to=a,b — can this letter be signed and encrypted?
func (s *Server) handlePGPStatus(w http.ResponseWriter, r *http.Request, sess *Session) {
	ctx := r.Context()
	from := strings.ToLower(r.URL.Query().Get("from"))
	out := map[string]any{"fromKey": false, "signDefault": false, "missing": []string{}}
	if _, ok := sess.account(from); ok {
		var sd bool
		if s.db.QueryRow(ctx, `SELECT sign_default FROM pgp_keys WHERE mailbox=$1`, from).Scan(&sd) == nil {
			out["fromKey"], out["signDefault"] = true, sd
		}
	}
	missing := []string{}
	for _, e := range strings.Split(r.URL.Query().Get("to"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e == "" {
			continue
		}
		if _, err := s.publicKeyOf(ctx, e); err != nil {
			missing = append(missing, e)
		}
	}
	out["missing"] = missing
	writeJSON(w, http.StatusOK, out)
}

// ---------- sending ----------

func newBoundary() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "tmail-pgp-" + hex.EncodeToString(b)
}

var contentHeaders = map[string]bool{"content-type": true, "content-transfer-encoding": true, "content-disposition": true,
	"content-id": true, "content-description": true, "content-language": true}

// splitEntity separates a message into its transport headers and its MIME entity (content
// headers + body), the part PGP/MIME signs or encrypts.
func splitEntity(msg []byte) (outer []string, entity []byte, err error) {
	end := bytes.Index(msg, []byte("\r\n\r\n"))
	if end < 0 {
		return nil, nil, errors.New("malformed message")
	}
	var fields []string
	for _, l := range strings.SplitAfter(string(msg[:end+2]), "\r\n") {
		if l == "" {
			continue
		}
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(fields) > 0 {
			fields[len(fields)-1] += l
			continue
		}
		fields = append(fields, l)
	}
	var inner strings.Builder
	for _, f := range fields {
		name, _, _ := strings.Cut(f, ":")
		if contentHeaders[strings.ToLower(strings.TrimSpace(name))] {
			inner.WriteString(f)
		} else {
			outer = append(outer, f)
		}
	}
	inner.WriteString("\r\n")
	return outer, append([]byte(inner.String()), msg[end+4:]...), nil
}

// pgpWrap signs and/or encrypts a built message (PGP/MIME) and adds the Autocrypt header.
func (s *Server) pgpWrap(ctx context.Context, msg []byte, from string, rcpts []string, sign, encrypt bool) ([]byte, error) {
	own, ownErr := s.mailboxKey(ctx, from)
	if !sign && !encrypt {
		if ownErr != nil {
			return msg, nil
		}
		return addAutocrypt(msg, from, own), nil
	}
	if ownErr != nil {
		return nil, fmt.Errorf("The mailbox %s has no OpenPGP key: create it in Settings → Encryption", from)
	}
	var recipients *crypto.KeyRing
	if encrypt {
		recipients, _ = crypto.NewKeyRing(nil)
		var missing []string
		seen := map[string]bool{}
		for _, rc := range rcpts {
			rc = strings.ToLower(rc)
			if seen[rc] {
				continue
			}
			seen[rc] = true
			k, err := s.publicKeyOf(ctx, rc)
			if err != nil {
				missing = append(missing, rc)
				continue
			}
			_ = recipients.AddKey(k)
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("No OpenPGP key for %s: the letter cannot be encrypted for them", strings.Join(missing, ", "))
		}
	}
	out, err := wrapPGP(msg, own, recipients, sign, encrypt)
	if err != nil {
		return nil, err
	}
	return addAutocrypt(out, from, own), nil
}

// wrapPGP builds the PGP/MIME letter: the entity signed by own and/or encrypted to recipients
// (and to own, so the copy in Sent can be read).
func wrapPGP(msg []byte, own *crypto.Key, recipients *crypto.KeyRing, sign, encrypt bool) ([]byte, error) {
	outer, entity, err := splitEntity(msg)
	if err != nil {
		return nil, err
	}
	body := entity
	ct := ""
	if sign {
		signer, err := pgp.Sign().SigningKey(own).Detached().New()
		if err != nil {
			return nil, err
		}
		sig, err := signer.Sign(entity, crypto.Armor)
		if err != nil {
			return nil, err
		}
		b := newBoundary()
		var sb bytes.Buffer
		sb.WriteString("This is an OpenPGP/MIME signed message (RFC 4880 and 3156)\r\n")
		sb.WriteString("--" + b + "\r\n")
		sb.Write(entity)
		sb.WriteString("\r\n--" + b + "\r\n")
		sb.WriteString("Content-Type: application/pgp-signature; name=\"signature.asc\"\r\nContent-Description: OpenPGP digital signature\r\n" +
			"Content-Disposition: attachment; filename=\"signature.asc\"\r\n\r\n")
		sb.Write(bytes.ReplaceAll(bytes.ReplaceAll(sig, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n")))
		sb.WriteString("\r\n--" + b + "--\r\n")
		ct = `multipart/signed; micalg=pgp-sha256; protocol="application/pgp-signature"; boundary="` + b + `"`
		body = append([]byte("Content-Type: "+ct+"\r\n\r\n"), sb.Bytes()...)
		if !encrypt {
			body = sb.Bytes()
		}
	}
	if encrypt {
		if recipients == nil {
			recipients, _ = crypto.NewKeyRing(nil)
		}
		pub, _ := own.ToPublic()
		_ = recipients.AddKey(pub)
		enc, err := pgp.Encryption().Recipients(recipients).New()
		if err != nil {
			return nil, err
		}
		plain := body
		if !sign {
			plain = entity
		}
		m, err := enc.Encrypt(plain)
		if err != nil {
			return nil, err
		}
		armored, err := m.Armor()
		if err != nil {
			return nil, err
		}
		b := newBoundary()
		var eb bytes.Buffer
		eb.WriteString("This is an OpenPGP/MIME encrypted message (RFC 4880 and 3156)\r\n")
		eb.WriteString("--" + b + "\r\nContent-Type: application/pgp-encrypted\r\nContent-Description: PGP/MIME version identification\r\n\r\nVersion: 1\r\n\r\n")
		eb.WriteString("--" + b + "\r\nContent-Type: application/octet-stream; name=\"encrypted.asc\"\r\nContent-Description: OpenPGP encrypted message\r\n" +
			"Content-Disposition: inline; filename=\"encrypted.asc\"\r\n\r\n")
		eb.WriteString(strings.ReplaceAll(strings.ReplaceAll(armored, "\r\n", "\n"), "\n", "\r\n"))
		eb.WriteString("\r\n--" + b + "--\r\n")
		ct = `multipart/encrypted; protocol="application/pgp-encrypted"; boundary="` + b + `"`
		body = eb.Bytes()
	}
	var out bytes.Buffer
	for _, f := range outer {
		out.WriteString(f)
	}
	out.WriteString("Content-Type: " + ct + "\r\n\r\n")
	out.Write(body)
	return out.Bytes(), nil
}

// addAutocrypt: the sender's public key in a header, so the correspondent's program can encrypt
// the answer (https://autocrypt.org).
func addAutocrypt(msg []byte, from string, key *crypto.Key) []byte {
	pub, err := key.ToPublic()
	if err != nil {
		return msg
	}
	bin, err := pub.Serialize()
	if err != nil {
		return msg
	}
	data := base64.StdEncoding.EncodeToString(bin)
	var h strings.Builder
	h.WriteString("Autocrypt: addr=" + from + "; prefer-encrypt=mutual; keydata=")
	for len(data) > 0 {
		n := min(76, len(data))
		h.WriteString("\r\n " + data[:n])
		data = data[n:]
	}
	h.WriteString("\r\n")
	return append([]byte(h.String()), msg...)
}

// ---------- reading ----------

// fetchSection fetches BODY.PEEK[<path>.MIME] or BODY.PEEK[<path>] raw.
func fetchSection(c *client.Client, uid uint32, path []int, spec imap.PartSpecifier) ([]byte, error) {
	set := new(imap.SeqSet)
	set.AddNum(uid)
	sec := &imap.BodySectionName{BodyPartName: imap.BodyPartName{Specifier: spec, Path: path}, Peek: true}
	m, err := fetchOne(c, set, []imap.FetchItem{sec.FetchItem()})
	if err != nil || m == nil {
		return nil, errNotFound
	}
	lit := m.GetBody(sec)
	if lit == nil {
		return nil, errNotFound
	}
	return io.ReadAll(lit)
}

func isPGPSigned(bs *imap.BodyStructure) bool {
	return bs != nil && strings.EqualFold(bs.MIMEType, "multipart") && strings.EqualFold(bs.MIMESubType, "signed") &&
		strings.EqualFold(bs.Params["protocol"], "application/pgp-signature") && len(bs.Parts) == 2
}

func isPGPEncrypted(bs *imap.BodyStructure) bool {
	return bs != nil && strings.EqualFold(bs.MIMEType, "multipart") && strings.EqualFold(bs.MIMESubType, "encrypted") &&
		strings.EqualFold(bs.Params["protocol"], "application/pgp-encrypted") && len(bs.Parts) == 2
}

// verifyDetached checks a signature with the sender's key.
func (s *Server) verifyDetached(ctx context.Context, sender string, data, sig []byte) *pgpStatus {
	st := &pgpStatus{Signed: true}
	key, err := s.publicKeyOf(ctx, sender)
	if err != nil {
		st.Problem = "unknown"
		return st
	}
	kr, _ := crypto.NewKeyRing(key)
	v, err := pgp.Verify().VerificationKeys(kr).New()
	if err != nil {
		st.Problem = "broken"
		return st
	}
	res, err := v.VerifyDetached(data, sig, crypto.Auto)
	if err != nil {
		st.Problem = "broken"
		return st
	}
	if res.SignatureError() != nil {
		st.Problem = "bad"
		return st
	}
	st.Verified, st.Signer = true, strings.ToUpper(hex.EncodeToString(res.SignedByFingerprint()))
	return st
}

// pgpInspect looks at a signed or encrypted letter while it is opened: verifies the signature,
// or decrypts it and puts its text and files in place of the encrypted blob.
func (s *Server) pgpInspect(ctx context.Context, c *client.Client, acc Account, folder string, uid uint32, bs *imap.BodyStructure, full *FullMessage) {
	switch {
	case isPGPSigned(bs):
		head, err1 := fetchSection(c, uid, []int{1}, imap.MIMESpecifier)
		body, err2 := fetchSection(c, uid, []int{1}, imap.EntireSpecifier)
		sig, err3 := fetchPart(c, uid, []int{2})
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}
		full.PGP = s.verifyDetached(ctx, full.From.Email, append(head, body...), decodeTransfer(sig, bs.Parts[1].Encoding))
		full.Attachments = withoutPGPParts(full.Attachments)
	case isPGPEncrypted(bs):
		st := &pgpStatus{Encrypted: true}
		full.PGP = st
		full.Attachments = withoutPGPParts(full.Attachments)
		key, err := s.mailboxKey(ctx, acc.Address)
		if err != nil {
			st.Problem = "nokey"
			return
		}
		raw, err := fetchPart(c, uid, []int{2})
		if err != nil {
			st.Problem = "broken"
			return
		}
		inner, res, err := s.decryptFor(ctx, key, full.From.Email, decodeTransfer(raw, bs.Parts[1].Encoding))
		if err != nil {
			st.Problem = "nokey"
			return
		}
		st.Decrypted = true
		if res != nil {
			*st = *res
			st.Encrypted, st.Decrypted = true, true
		}
		s.showInner(ctx, acc, folder, uid, inner, full)
	}
}

// decryptFor decrypts an OpenPGP message; a signature inside it (or an inner multipart/signed)
// is checked against the sender's key.
func (s *Server) decryptFor(ctx context.Context, key *crypto.Key, sender string, armored []byte) ([]byte, *pgpStatus, error) {
	kr, err := crypto.NewKeyRing(key)
	if err != nil {
		return nil, nil, err
	}
	b := pgp.Decryption().DecryptionKeys(kr)
	var senderKey *crypto.Key
	if k, err := s.publicKeyOf(ctx, sender); err == nil {
		senderKey = k
		vkr, _ := crypto.NewKeyRing(k)
		b = b.VerificationKeys(vkr)
	}
	dec, err := b.New()
	if err != nil {
		return nil, nil, err
	}
	res, err := dec.Decrypt(armored, crypto.Auto)
	if err != nil {
		return nil, nil, err
	}
	data := res.Bytes()
	var st *pgpStatus
	if sig := res.SignedByKeyId(); sig != 0 {
		st = &pgpStatus{Signed: true}
		switch {
		case senderKey == nil:
			st.Problem = "unknown"
		case res.SignatureError() != nil:
			st.Problem = "bad"
		default:
			st.Verified, st.Signer = true, strings.ToUpper(hex.EncodeToString(res.SignedByFingerprint()))
		}
	}
	// PGP/MIME signed inside the encryption (sign-then-encrypt, RFC 3156 §6.1).
	if h, body, ok := splitHeaderBody(data); ok {
		mt, params, _ := mime.ParseMediaType(h.Get("Content-Type"))
		if strings.EqualFold(mt, "multipart/signed") && strings.EqualFold(params["protocol"], "application/pgp-signature") {
			if parts := splitMultipart(body, params["boundary"]); len(parts) == 2 {
				_, sigBody, _ := splitHeaderBody(parts[1])
				st = s.verifyDetached(ctx, sender, parts[0], sigBody)
				data = parts[0]
			}
		}
	}
	return data, st, nil
}

func splitHeaderBody(data []byte) (textproto.MIMEHeader, []byte, bool) {
	i := bytes.Index(data, []byte("\r\n\r\n"))
	sep := 4
	if j := bytes.Index(data, []byte("\n\n")); i < 0 || (j >= 0 && j < i) {
		i, sep = j, 2
	}
	if i < 0 {
		return nil, data, false
	}
	h, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(append(append([]byte{}, data[:i]...), "\r\n\r\n"...)))).ReadMIMEHeader()
	if err != nil {
		return nil, data, false
	}
	return h, data[i+sep:], true
}

// splitMultipart cuts a multipart body into its raw parts (headers and body, exactly as written).
func splitMultipart(body []byte, boundary string) [][]byte {
	if boundary == "" {
		return nil
	}
	delim := []byte("--" + boundary)
	var parts [][]byte
	rest := body
	start := -1
	for {
		i := bytes.Index(rest, delim)
		if i < 0 {
			break
		}
		if i > 0 && rest[i-1] != '\n' {
			rest = rest[i+len(delim):]
			continue
		}
		if start >= 0 {
			end := i
			if end >= 2 && rest[end-2] == '\r' && rest[end-1] == '\n' {
				end -= 2
			} else if end >= 1 && rest[end-1] == '\n' {
				end--
			}
			parts = append(parts, append([]byte{}, rest[:end]...))
		}
		after := rest[i+len(delim):]
		if bytes.HasPrefix(after, []byte("--")) {
			break
		}
		nl := bytes.IndexByte(after, '\n')
		if nl < 0 {
			break
		}
		rest = after[nl+1:]
		start = 0
	}
	return parts
}

func withoutPGPParts(atts []Attachment) []Attachment {
	out := atts[:0:0]
	for _, a := range atts {
		if a.Mime == "application/pgp-signature" || a.Mime == "application/pgp-encrypted" || strings.EqualFold(a.Name, "encrypted.asc") ||
			(a.Mime == "application/octet-stream" && strings.HasSuffix(strings.ToLower(a.Name), ".asc")) {
			continue
		}
		out = append(out, a)
	}
	return out
}

type innerPart struct {
	mime, name, cid, charset string
	data                     []byte
	inline                   bool
}

// parseInner reads a decrypted MIME entity: the text to show and its files in order.
func parseInner(data []byte) (html, text string, parts []innerPart) {
	ent, err := gomsg.Read(bytes.NewReader(data))
	if err != nil && ent == nil {
		return "", string(data), nil
	}
	_ = ent.Walk(func(path []int, e *gomsg.Entity, err error) error {
		if err != nil {
			return nil
		}
		mt, params, _ := e.Header.ContentType()
		if strings.HasPrefix(mt, "multipart/") {
			return nil
		}
		disp, dparams, _ := e.Header.ContentDisposition()
		name := dparams["filename"]
		if name == "" {
			name = params["name"]
		}
		body, _ := io.ReadAll(io.LimitReader(e.Body, 50<<20))
		cid := strings.Trim(e.Header.Get("Content-Id"), "<> ")
		switch {
		case disp != "attachment" && name == "" && mt == "text/html" && html == "":
			html = string(body)
		case disp != "attachment" && name == "" && (mt == "text/plain" || mt == "") && text == "":
			text = string(body)
		default:
			if name == "" {
				name = "attachment"
			}
			parts = append(parts, innerPart{mime: mt, name: name, cid: cid, charset: params["charset"], data: body, inline: cid != "" && strings.HasPrefix(mt, "image/")})
		}
		return nil
	})
	return html, text, parts
}

func (s *Server) showInner(ctx context.Context, acc Account, folder string, uid uint32, inner []byte, full *FullMessage) {
	html, text, parts := parseInner(inner)
	partURL := func(n int) string {
		return "/api/attachment?" + url.Values{"account": {acc.Address}, "folder": {folder}, "uid": {strconv.Itoa(int(uid))}, "part": {"pgp." + strconv.Itoa(n)}}.Encode()
	}
	cidURL := func(cid string) string {
		for i, p := range parts {
			if strings.EqualFold(p.cid, strings.Trim(cid, "<>")) {
				return partURL(i)
			}
		}
		return ""
	}
	switch {
	case html != "":
		full.HTML, full.RemoteImgs = sanitizeHTML(html, cidURL)
	default:
		full.HTML = textToHTML(text)
	}
	if text != "" {
		full.Text = text
	}
	for i, p := range parts {
		full.Attachments = append(full.Attachments, Attachment{Part: "pgp." + strconv.Itoa(i), Name: p.name, Mime: p.mime, Size: uint32(len(p.data)),
			Inline: inlineSafe(p.mime), Preview: previewKind(p.mime, p.name), Risk: attachmentRisk(p.name), Embedded: p.inline})
	}
}

// servePGPPart serves a file of a decrypted letter (part "pgp.N").
func (s *Server) servePGPPart(w http.ResponseWriter, r *http.Request, sess *Session, uid uint32, n int) {
	s.withAccount(w, r, sess, r.URL.Query().Get("account"), false, func(c *client.Client, acc Account) {
		folder, err := s.resolveFolder(c, acc.Address, r)
		if err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		if _, err := c.Select(folder, true); err != nil {
			writeErr(w, http.StatusNotFound, "Folder not found")
			return
		}
		set := new(imap.SeqSet)
		set.AddNum(uid)
		m, err := fetchOne(c, set, []imap.FetchItem{imap.FetchUid, imap.FetchBodyStructure, imap.FetchEnvelope})
		if err != nil || m == nil || !isPGPEncrypted(m.BodyStructure) {
			writeErr(w, http.StatusNotFound, "Attachment not found")
			return
		}
		key, err := s.mailboxKey(r.Context(), acc.Address)
		if err != nil {
			writeErr(w, http.StatusNotFound, "Attachment not found")
			return
		}
		raw, err := fetchPart(c, uid, []int{2})
		if err != nil {
			writeErr(w, http.StatusNotFound, "Attachment not found")
			return
		}
		sender := ""
		if m.Envelope != nil && len(m.Envelope.From) > 0 {
			sender = addrOf(m.Envelope.From[0]).Email
		}
		inner, _, err := s.decryptFor(r.Context(), key, sender, decodeTransfer(raw, m.BodyStructure.Parts[1].Encoding))
		if err != nil {
			writeErr(w, http.StatusNotFound, "Attachment not found")
			return
		}
		_, _, parts := parseInner(inner)
		if n < 0 || n >= len(parts) {
			writeErr(w, http.StatusNotFound, "Attachment not found")
			return
		}
		p := parts[n]
		s.serveFile(w, r, sess, p.mime, p.name, p.charset, p.data, r.URL.Query().Get("download") == "1")
	})
}

var reAutocrypt = regexp.MustCompile(`(?i)(?:^|;)\s*addr=([^;\s]+)`)

// collectAutocrypt keeps the key a correspondent announced in the Autocrypt header of their
// letter, unless a key for that address was added by hand.
func (s *Server) collectAutocrypt(ctx context.Context, from string, header string) {
	if header == "" {
		return
	}
	m := reAutocrypt.FindStringSubmatch(header)
	if m == nil || !strings.EqualFold(m[1], from) {
		return
	}
	i := strings.Index(strings.ToLower(header), "keydata=")
	if i < 0 {
		return
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(header[i+8:]), ""))
	if err != nil {
		return
	}
	var src string
	if s.db.QueryRow(ctx, `SELECT source FROM pgp_contacts WHERE email=$1`, strings.ToLower(from)).Scan(&src) == nil && src != "autocrypt" {
		return
	}
	_, _ = s.saveContactKeys(ctx, string(data), "autocrypt")
}
