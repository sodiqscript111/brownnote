package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/client9/gospell"
	"golang.org/x/text/unicode/norm"
)

//go:embed dictionary/en_US.aff dictionary/en_US.dic
var dictionary embed.FS

const maxBody = 128 << 10
const maxWords = 4096

var wordPattern = regexp.MustCompile(`^\p{L}[\p{L}\p{M}]*('\p{L}[\p{L}\p{M}]*)*$`)

type result struct {
	Word        string   `json:"word"`
	Correct     bool     `json:"correct"`
	Suggestions []string `json:"suggestions"`
}

type checker struct {
	mu   sync.Mutex
	dict *gospell.GoSpell
}

func newChecker() (*checker, error) {
	aff, err := dictionary.Open("dictionary/en_US.aff")
	if err != nil {
		return nil, err
	}
	defer aff.Close()
	dic, err := dictionary.Open("dictionary/en_US.dic")
	if err != nil {
		return nil, err
	}
	defer dic.Close()
	d, err := gospell.NewGoSpellReader(aff, dic)
	if err != nil {
		return nil, err
	}
	// Case-insensitive queries still recognize proper nouns and contractions such as I.
	// Keep the actual dictionary's case rules by trying uppercase too.
	return &checker{dict: d}, nil
}

func normalize(word string) string {
	return strings.ToLower(strings.ReplaceAll(norm.NFC.String(word), "’", "'"))
}

func (c *checker) check(word string) (result, error) {
	// gospell's lazy surface lookup mutates internal maps even in Spell.
	c.mu.Lock()
	defer c.mu.Unlock()
	r := result{Word: word, Correct: c.dict.Spell(word) || c.dict.Spell(strings.ToUpper(word)), Suggestions: []string{}}
	if r.Correct {
		return r, nil
	}
	suggestions, err := c.dict.Suggest(word, 5)
	if err != nil {
		return r, err
	}
	seen := map[string]bool{}
	for _, s := range suggestions {
		if !seen[s.Word] && normalize(s.Word) != word {
			r.Suggestions = append(r.Suggestions, s.Word)
			seen[s.Word] = true
		}
		if len(r.Suggestions) == 5 {
			break
		}
	}
	return r, nil
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

func (c *checker) serveCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		fail(w, 405, "use POST")
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		fail(w, 415, "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var request struct {
		Words []string `json:"words"`
	}
	if err := dec.Decode(&request); err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			fail(w, 413, "request exceeds 128 KiB")
		} else {
			fail(w, 400, "invalid JSON body")
		}
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			fail(w, 413, "request exceeds 128 KiB")
		} else {
			fail(w, 400, "body must contain one JSON object")
		}
		return
	}
	if request.Words == nil || len(request.Words) > maxWords {
		fail(w, 400, "words must be an array with at most 4096 entries")
		return
	}
	unique := make([]string, 0, len(request.Words))
	seen := map[string]bool{}
	for _, raw := range request.Words {
		word := normalize(raw)
		if !utf8.ValidString(word) || utf8.RuneCountInString(word) > 64 || !wordPattern.MatchString(word) {
			fail(w, 400, "each word must contain 1–64 letters, optionally joined by apostrophes")
			return
		}
		if !seen[word] {
			seen[word] = true
			unique = append(unique, word)
		}
	}
	results := make([]result, 0, len(unique))
	for _, word := range unique {
		if r.Context().Err() != nil {
			return
		}
		checked, err := c.check(word)
		if err != nil {
			log.Printf("spell check failed: %v", err)
			fail(w, 500, "spell checker unavailable")
			return
		}
		results = append(results, checked)
	}
	jsonResponse(w, 200, map[string]any{"results": results})
}

func handler(c *checker, staticDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/check", c.serveCheck)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "API route not found") })
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	return mux
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	staticDir := flag.String("static", "../dist", "built frontend directory")
	flag.Parse()
	c, err := newChecker()
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: *addr, Handler: handler(c, *staticDir), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Brownnote: http://%s (dictionary loaded once)", *addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
