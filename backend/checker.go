package main

import (
	"embed"
	"strings"
	"sync"

	"github.com/client9/gospell"
	"golang.org/x/text/unicode/norm"
)

//go:embed dictionary/en_US.aff dictionary/en_US.dic
var dictionary embed.FS

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
