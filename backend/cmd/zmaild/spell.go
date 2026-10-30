package main

import (
	"bufio"
	"context"
	"io"
	"log"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Spelling check for the letter editor, on our own server (the text does not leave it): Hunspell
// with the Russian and English dictionaries, one long-running process per language in its pipe
// mode (-a). The browser sends the words of the letter, we answer which are wrong and how to write
// them. Hunspell's own order of suggestions is poor for typos (for the Russian typo "zdelat" the
// intended "sdelat" comes last), so they are re-ranked by how close they are to the word, with sounds
// that are easy to confuse in Russian (o/a, e/i, z/s, d/t…) cheap.

const (
	spellMaxWords = 400
	spellMaxSugg  = 6
	spellCacheMax = 50000
)

type hunspell struct {
	dict string
	mu   sync.Mutex
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
}

var (
	spellRu = &hunspell{dict: "ru_RU"}
	spellEn = &hunspell{dict: "en_US"}

	spellCacheMu sync.Mutex
	spellCache   = map[string][]string{} // word → nil if right, suggestions (maybe empty) if wrong
)

func (h *hunspell) start() error {
	cmd := exec.Command("hunspell", "-a", "-i", "UTF-8", "-d", h.dict)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	h.cmd, h.in, h.out = cmd, in, bufio.NewReader(out)
	if _, err := h.out.ReadString('\n'); err != nil { // the "@(#) International Ispell" banner
		h.stop()
		return err
	}
	return nil
}

func (h *hunspell) stop() {
	if h.cmd != nil {
		_ = h.in.Close()
		_ = h.cmd.Process.Kill()
		_ = h.cmd.Wait()
		h.cmd = nil
	}
}

// check answers for each word: ok, and the suggestions if it is wrong. A stuck process is killed
// after a few seconds and started again on the next call.
func (h *hunspell) check(words []string) (ok []bool, sugg [][]string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cmd == nil {
		if err := h.start(); err != nil {
			return nil, nil, err
		}
	}
	proc := h.cmd.Process
	timer := time.AfterFunc(5*time.Second, func() { _ = proc.Kill() })
	defer timer.Stop()
	var b strings.Builder
	for _, w := range words {
		b.WriteString("^") // the line is text, never a command
		b.WriteString(w)
		b.WriteString("\n")
	}
	if _, err := io.WriteString(h.in, b.String()); err != nil {
		h.stop()
		return nil, nil, err
	}
	ok = make([]bool, len(words))
	sugg = make([][]string, len(words))
	for i := range words {
		ok[i] = true
		// One line of results per part of the word (Hunspell may split it), then an empty line.
		for {
			line, err := h.out.ReadString('\n')
			if err != nil {
				h.stop()
				return nil, nil, err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			switch line[0] {
			case '&': // & word count offset: s1, s2
				ok[i] = false
				if p := strings.Index(line, ": "); p >= 0 {
					sugg[i] = append(sugg[i], strings.Split(line[p+2:], ", ")...)
				}
			case '#':
				ok[i] = false
			}
		}
	}
	return ok, sugg, nil
}

func isCyrillic(w string) bool {
	for _, r := range w {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// spellSkip: words not worth checking — abbreviations and names written in capitals (LLC, ACME),
// words with digits, single letters, words mixing alphabets (codes, models).
func spellSkip(w string) bool {
	n, upper, cyr, lat := 0, 0, false, false
	for _, r := range w {
		switch {
		case unicode.IsDigit(r) || r == '_':
			return true
		case unicode.IsLetter(r):
			n++
			if unicode.IsUpper(r) {
				upper++
			}
			if unicode.Is(unicode.Cyrillic, r) {
				cyr = true
			} else {
				lat = true
			}
		}
	}
	return n < 2 || upper == n || (cyr && lat) || (upper > 1 && !unicode.IsUpper([]rune(w)[0]))
}

// spellCheck returns the wrong words with their suggestions.
func spellCheck(words []string) (map[string][]string, error) {
	bad := map[string][]string{}
	var ru, en []string
	spellCacheMu.Lock()
	for _, w := range words {
		if s, ok := spellCache[w]; ok {
			if s != nil {
				bad[w] = s
			}
		} else if isCyrillic(w) {
			ru = append(ru, w)
		} else {
			en = append(en, w)
		}
	}
	spellCacheMu.Unlock()
	for _, part := range []struct {
		h     *hunspell
		words []string
	}{{spellRu, ru}, {spellEn, en}} {
		if len(part.words) == 0 {
			continue
		}
		ok, sugg, err := part.h.check(part.words)
		if err != nil {
			return nil, err
		}
		spellCacheMu.Lock()
		if len(spellCache) > spellCacheMax {
			spellCache = map[string][]string{}
		}
		for i, w := range part.words {
			if ok[i] {
				spellCache[w] = nil
				continue
			}
			s := rankSuggestions(w, sugg[i])
			spellCache[w] = s
			bad[w] = s
		}
		spellCacheMu.Unlock()
	}
	return bad, nil
}

// Letters a Russian writer easily swaps by ear or by a slip of the finger (Cyrillic letter pairs:
// detection data).
var spellNear = func() map[[2]rune]bool {
	m := map[[2]rune]bool{}
	for _, p := range []string{"оа", "еи", "ея", "иы", "зс", "дт", "бп", "гк", "вф", "жш", "щш", "ьъ", "еэ", "её", "цс", "ую", "ао"} {
		r := []rune(p)
		m[[2]rune{r[0], r[1]}] = true
		m[[2]rune{r[1], r[0]}] = true
	}
	return m
}()

// A forgotten or extra Russian soft sign, hard sign or short i is a small slip.
func spellGap(r rune) float64 {
	if r == 'ь' || r == 'ъ' || r == 'й' {
		return 0.5
	}
	return 1
}

// spellDist: Damerau–Levenshtein distance where a swap of letters that sound alike costs half.
func spellDist(a, b []rune) float64 {
	d := make([][]float64, len(a)+1)
	for i := range d {
		d[i] = make([]float64, len(b)+1)
		if i > 0 {
			d[i][0] = d[i-1][0] + spellGap(a[i-1])
		}
	}
	for j := 1; j <= len(b); j++ {
		d[0][j] = d[0][j-1] + spellGap(b[j-1])
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			sub := 0.0
			if a[i-1] != b[j-1] {
				sub = 1
				if spellNear[[2]rune{a[i-1], b[j-1]}] {
					sub = 0.5
				}
			}
			v := min(d[i-1][j]+spellGap(a[i-1]), d[i][j-1]+spellGap(b[j-1]), d[i-1][j-1]+sub)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, d[i-2][j-2]+0.6)
			}
			d[i][j] = v
		}
	}
	return d[len(a)][len(b)]
}

func rankSuggestions(word string, sugg []string) []string {
	lw := []rune(strings.ToLower(word))
	type cand struct {
		s     string
		score float64
		pos   int
	}
	seen := map[string]bool{}
	var cs []cand
	for i, s := range sugg {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		ls := strings.ToLower(s)
		score := spellDist(lw, []rune(strings.ReplaceAll(ls, "-", "")))
		if strings.ContainsAny(s, " -") { // "every one" for "everyone": splitting a word is rarely what was meant
			score += 1.2
		}
		if ls != "" && len(lw) > 0 && []rune(ls)[0] != lw[0] {
			score += 0.3
		}
		cs = append(cs, cand{s, score + float64(i)*0.02, i})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].score < cs[j].score })
	out := []string{}
	upperFirst := unicode.IsUpper([]rune(word)[0])
	for _, c := range cs {
		s := c.s
		if upperFirst { // the word stood at the start of a sentence: keep its capital letter
			r, n := utf8.DecodeRuneInString(s)
			s = string(unicode.ToUpper(r)) + s[n:]
		}
		out = append(out, s)
		if len(out) == spellMaxSugg {
			break
		}
	}
	return out
}

// POST /api/spell {"words": [...]} → {"bad": {"word": ["suggestion", …]}}. Words of the user's own
// dictionary are never wrong.
func (s *Server) handleSpell(w http.ResponseWriter, r *http.Request, sess *Session) {
	var in struct {
		Words []string `json:"words"`
		Pairs []string `json:"pairs"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request")
		return
	}
	mine, err := s.spellDict(r.Context(), sess.UserID)
	if err != nil {
		serverErr(w, "spell dict", err)
		return
	}
	var words []string
	seen := map[string]bool{}
	for _, wd := range in.Words {
		wd = strings.TrimSpace(wd)
		if wd == "" || seen[wd] || utf8.RuneCountInString(wd) > 40 || spellSkip(wd) || mine[strings.ToLower(wd)] {
			continue
		}
		seen[wd] = true
		words = append(words, wd)
		if len(words) == spellMaxWords {
			break
		}
	}
	bad, err := spellCheck(words)
	if err != nil {
		log.Printf("spell: %v", err)
		writeErr(w, http.StatusServiceUnavailable, "Spell checking is temporarily unavailable")
		return
	}
	pairs, err := spellPairs(in.Pairs, mine)
	if err != nil {
		log.Printf("spell: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bad": bad, "pairs": pairs})
}

// One-letter words of Russian (and English) that may stand alone after a moved space (detection data).
const spellOneLetter = "аиоуявксжai"

// spellPairs fixes a misplaced space between two words: "thisf eature" → "this feature",
// "hel lo" → "hello". Each pair is "word1 word2"; the answer lists the readings in which every
// word is right, the nearest shift of the space first.
func spellPairs(pairs []string, mine map[string]bool) (map[string][]string, error) {
	out := map[string][]string{}
	type cand struct {
		parts []string
		shift int
	}
	cands := map[string][]cand{}
	var words []string
	for i, p := range pairs {
		if i == 50 {
			break
		}
		a, b, ok := strings.Cut(p, " ")
		if !ok || a == "" || b == "" || strings.Contains(b, " ") || utf8.RuneCountInString(p) > 50 {
			continue
		}
		joined := []rune(a + b)
		cut := utf8.RuneCountInString(a)
		var cs []cand
		cs = append(cs, cand{[]string{string(joined)}, 0})
		for k := 1; k < len(joined); k++ {
			if k == cut || k > cut+3 || k < cut-3 { // the space moves by a few letters at most
				continue
			}
			d := k - cut
			if d < 0 {
				d = -d
			}
			cs = append(cs, cand{[]string{string(joined[:k]), string(joined[k:])}, d})
		}
		cands[p] = cs
		for _, c := range cs {
			for _, w := range c.parts {
				if utf8.RuneCountInString(w) > 1 {
					words = append(words, w)
				}
			}
		}
	}
	if len(words) == 0 {
		return out, nil
	}
	bad, err := spellCheck(words)
	if err != nil {
		return out, err
	}
	right := func(w string) bool {
		if utf8.RuneCountInString(w) == 1 {
			return strings.ContainsRune(spellOneLetter, unicode.ToLower([]rune(w)[0]))
		}
		_, wrong := bad[w]
		return !wrong || mine[strings.ToLower(w)]
	}
	for p, cs := range cands {
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].shift < cs[j].shift })
		var fixes []string
		for _, c := range cs {
			ok := true
			for _, w := range c.parts {
				ok = ok && right(w)
			}
			if ok {
				fixes = append(fixes, strings.Join(c.parts, " "))
			}
		}
		if len(fixes) > 3 {
			fixes = fixes[:3]
		}
		out[p] = fixes
	}
	return out, nil
}

func (s *Server) spellDict(ctx context.Context, user int64) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `SELECT word FROM spell_dict WHERE user_id=$1`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var wd string
		if err := rows.Scan(&wd); err != nil {
			return nil, err
		}
		m[wd] = true
	}
	return m, rows.Err()
}

// GET /api/spell/dict → {"words": [...]}; POST {"word"} adds; DELETE ?word= removes.
func (s *Server) handleSpellDict(w http.ResponseWriter, r *http.Request, sess *Session) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		m, err := s.spellDict(ctx, sess.UserID)
		if err != nil {
			serverErr(w, "spell dict", err)
			return
		}
		words := make([]string, 0, len(m))
		for wd := range m {
			words = append(words, wd)
		}
		sort.Strings(words)
		writeJSON(w, http.StatusOK, map[string]any{"words": words})
	case http.MethodPost:
		var in struct {
			Word string `json:"word"`
		}
		err := readJSON(r, &in)
		wd := strings.ToLower(strings.TrimSpace(in.Word))
		if err != nil || wd == "" || utf8.RuneCountInString(wd) > 40 || strings.ContainsAny(wd, " \t\n") {
			writeErr(w, http.StatusBadRequest, "Only a single word can be added to the dictionary")
			return
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO spell_dict (user_id, word) VALUES ($1,$2) ON CONFLICT DO NOTHING`, sess.UserID, wd); err != nil {
			serverErr(w, "spell dict add", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		wd := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("word")))
		if _, err := s.db.Exec(ctx, `DELETE FROM spell_dict WHERE user_id=$1 AND word=$2`, sess.UserID, wd); err != nil {
			serverErr(w, "spell dict remove", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
