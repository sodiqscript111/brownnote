package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func load(t testing.TB) *checker {
	t.Helper()
	c, err := newChecker()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSpelling(t *testing.T) {
	c := load(t)
	for _, word := range []string{"hello", "world", "running", "don't", "i", "london"} {
		r, err := c.check(normalize(word))
		if err != nil || !r.Correct {
			t.Errorf("%s: %+v %v", word, r, err)
		}
	}
	r, err := c.check("helo")
	if err != nil || r.Correct || len(r.Suggestions) == 0 || len(r.Suggestions) > 5 {
		t.Fatalf("%+v %v", r, err)
	}
	found := false
	for _, s := range r.Suggestions {
		if s == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("hello missing: %v", r.Suggestions)
	}
}

func request(c *checker, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/check", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(c, "").ServeHTTP(w, r)
	return w
}

func TestBatch(t *testing.T) {
	c := load(t)
	w := request(c, `{"words":["HELLO","hello","helo","DON’T","don't"]}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var body struct {
		Results []result `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Results) != 3 || body.Results[0].Word != "hello" || !body.Results[0].Correct || body.Results[1].Correct || !body.Results[2].Correct {
		t.Fatal(body)
	}
	if got := request(c, `{"words":[]}`); got.Code != 200 || !strings.Contains(got.Body.String(), `"results":[]`) {
		t.Fatal(got.Body.String())
	}
}

func TestValidation(t *testing.T) {
	c := load(t)
	for _, body := range []string{`{}`, `null`, `{"words":null}`, `{"words":[""]}`, `{"words":["hello!"]}`, `{"words":["a b"]}`, `{"words":[2]}`, `{"words":[],"extra":true}`, `{"words":[]} {}`, `{`, `{"words":["` + strings.Repeat("a", 65) + `"]}`} {
		if w := request(c, body); w.Code != 400 {
			t.Errorf("body %s: %d", body, w.Code)
		}
	}
	w := request(c, `{"words":["`+strings.Repeat("a", maxBody)+`"]}`)
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	many, _ := json.Marshal(map[string]any{"words": make([]string, maxWords+1)})
	if w := request(c, string(many)); w.Code != 400 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "/api/check", nil)
	w = httptest.NewRecorder()
	handler(c, "").ServeHTTP(w, r)
	if w.Code != 405 || w.Header().Get("Allow") != "POST" {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "/api/check", bytes.NewBufferString(`{"words":[]}`))
	w = httptest.NewRecorder()
	handler(c, "").ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatal(w.Code)
	}
}

func TestConcurrentRequests(t *testing.T) {
	c := load(t)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				if w := request(c, `{"words":["hello","helo","running"]}`); w.Code != http.StatusOK {
					t.Error(w.Code)
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkValidation(b *testing.B) {
	c := load(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.check("hello")
	}
}
func BenchmarkSuggestions(b *testing.B) {
	c := load(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.check("helo")
	}
}

type checkerFunc func(string) (result, error)

func (f checkerFunc) check(word string) (result, error) { return f(word) }

func TestGinRoutingAndStaticFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "<title>Brownnote</title>", "editor.css": "body { color: brown; }"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := handler(checkerFunc(func(word string) (result, error) {
		return result{Word: word, Correct: true, Suggestions: []string{}}, nil
	}), dir)
	for _, test := range []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/", 200, "Brownnote"},
		{"GET", "/editor.css", 200, "brown"},
		{"GET", "/missing.css", 404, ""},
		{"GET", "/api", 404, "API route not found"},
		{"POST", "/api/missing", 404, "API route not found"},
		{"POST", "/api/check/", 404, "API route not found"},
		{"GET", "/API/check", 404, ""},
		{"GET", "/api/check", 405, "use POST"},
		{"PUT", "/api/check", 405, "use POST"},
		{"OPTIONS", "/api/check", 405, "use POST"},
	} {
		t.Run(test.method+test.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
			if w.Code != test.status || !strings.Contains(w.Body.String(), test.body) {
				t.Fatalf("status %d, body %s", w.Code, w.Body.String())
			}
			if test.status == 405 && w.Header().Get("Allow") != "POST" {
				t.Fatal("missing Allow header")
			}
			if strings.HasPrefix(test.path, "/api") && (w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json")) {
				t.Fatal("missing API response headers")
			}
		})
	}
}

func TestGinRecovery(t *testing.T) {
	h := handler(checkerFunc(func(string) (result, error) { panic("private failure") }), t.TempDir())
	r := httptest.NewRequest("POST", "/api/check", strings.NewReader(`{"words":["hello"]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 500 || w.Body.String() != `{"error":"internal server error"}` {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
}

func TestGinCancelledRequest(t *testing.T) {
	calls := 0
	h := handler(checkerFunc(func(string) (result, error) { calls++; return result{}, nil }), t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("POST", "/api/check", strings.NewReader(`{"words":["hello"]}`)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if calls != 0 || w.Body.Len() != 0 {
		t.Fatalf("cancelled request checked %d words", calls)
	}
}

func TestInjectedCheckerFailure(t *testing.T) {
	calls := 0
	h := handler(checkerFunc(func(word string) (result, error) { calls++; return result{}, errors.New("dictionary failure") }), t.TempDir())
	r := httptest.NewRequest("POST", "/api/check", strings.NewReader(`{"words":["hello"]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if calls != 1 || w.Code != 500 || strings.Contains(w.Body.String(), "dictionary failure") {
		t.Fatalf("%d calls, status %d, body %s", calls, w.Code, w.Body.String())
	}
}

func TestInjectedCheckerDeduplication(t *testing.T) {
	calls := 0
	h := handler(checkerFunc(func(word string) (result, error) {
		calls++
		return result{Word: word, Correct: true, Suggestions: []string{}}, nil
	}), t.TempDir())
	r := httptest.NewRequest("POST", "/api/check", strings.NewReader(`{"words":["HELLO","hello"]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if calls != 1 || w.Code != 200 {
		t.Fatalf("%d calls, status %d", calls, w.Code)
	}
}
