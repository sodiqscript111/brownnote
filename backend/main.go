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
	"github.com/gin-gonic/gin"
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
type spellChecker interface{ check(string) (result, error) }
type api struct{ checker spellChecker }

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
	return &checker{dict: d}, nil
}

func normalize(word string) string {
	return strings.ToLower(strings.ReplaceAll(norm.NFC.String(word), "’", "'"))
}

func (c *checker) check(word string) (result, error) {
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

func jsonResponse(c *gin.Context, status int, value any) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.JSON(status, value)
}
func fail(c *gin.Context, status int, message string) {
	jsonResponse(c, status, gin.H{"error": message})
}

func (a *api) serveCheck(c *gin.Context) {
	r := c.Request
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		fail(c, 415, "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(c.Writer, r.Body, maxBody)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var request struct {
		Words []string `json:"words"`
	}
	if err := dec.Decode(&request); err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			fail(c, 413, "request exceeds 128 KiB")
		} else {
			fail(c, 400, "invalid JSON body")
		}
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			fail(c, 413, "request exceeds 128 KiB")
		} else {
			fail(c, 400, "body must contain one JSON object")
		}
		return
	}
	if request.Words == nil || len(request.Words) > maxWords {
		fail(c, 400, "words must be an array with at most 4096 entries")
		return
	}
	unique := make([]string, 0, len(request.Words))
	seen := map[string]bool{}
	for _, raw := range request.Words {
		word := normalize(raw)
		if !utf8.ValidString(word) || utf8.RuneCountInString(word) > 64 || !wordPattern.MatchString(word) {
			fail(c, 400, "each word must contain 1–64 letters, optionally joined by apostrophes")
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
		checked, err := a.checker.check(word)
		if err != nil {
			log.Printf("spell check failed: %v", err)
			fail(c, 500, "spell checker unavailable")
			return
		}
		results = append(results, checked)
	}
	jsonResponse(c, 200, map[string]any{"results": results})
}

func handler(c spellChecker, staticDir string) http.Handler {
	router := gin.New()
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	_ = router.SetTrustedProxies(nil)
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		fail(c, http.StatusInternalServerError, "internal server error")
		c.Abort()
	}))
	router.POST("/api/check", (&api{checker: c}).serveCheck)
	files := http.FileServer(http.Dir(staticDir))
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api/check" {
			c.Header("Allow", "POST")
			fail(c, http.StatusMethodNotAllowed, "use POST")
		} else if path == "/api" || strings.HasPrefix(path, "/api/") {
			fail(c, http.StatusNotFound, "API route not found")
		} else {
			files.ServeHTTP(c.Writer, c.Request)
		}
	})
	return router
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	staticDir := flag.String("static", "../dist", "built frontend directory")
	flag.Parse()
	gin.SetMode(gin.ReleaseMode)
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
