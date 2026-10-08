package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type api struct{ checker spellChecker }

var wordPattern = regexp.MustCompile(`^\p{L}[\p{L}\p{M}]*('\p{L}[\p{L}\p{M}]*)*$`)

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
