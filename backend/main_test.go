package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

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
	c.serveCheck(w, r)
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
	c.serveCheck(w, r)
	if w.Code != 405 || w.Header().Get("Allow") != "POST" {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "/api/check", bytes.NewBufferString(`{"words":[]}`))
	w = httptest.NewRecorder()
	c.serveCheck(w, r)
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
