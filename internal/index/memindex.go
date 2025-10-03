package index

import (
	"sync"

	"github.com/mazenh0/auroraseek/internal/util"
)

type Doc struct {
	ID    string
	URL   string
	Title string
	Body  string
	TF    map[string]int
	DL    int
}

type MemIndex struct {
	mu       sync.RWMutex
	postings map[string]map[string]int // term -> docID -> tf
	docs     map[string]*Doc
	df       map[string]int
	totalLen int
}

func NewMem() *MemIndex {
	return &MemIndex{
		postings: map[string]map[string]int{},
		docs:     map[string]*Doc{},
		df:       map[string]int{},
	}
}

func (m *MemIndex) Add(id, url, title, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	toks := util.Tokens(title + " " + body)
	tf := map[string]int{}
	for _, t := range toks {
		tf[t]++
	}

	d := &Doc{ID: id, URL: url, Title: title, Body: body, TF: tf, DL: len(toks)}
	m.docs[id] = d
	m.totalLen += d.DL

	seen := map[string]bool{}
	for term, f := range tf {
		if m.postings[term] == nil {
			m.postings[term] = map[string]int{}
		}
		m.postings[term][id] = f
		if !seen[term] {
			m.df[term]++
			seen[term] = true
		}
	}
}

func (m *MemIndex) Snapshot() (N int, avgDL float64, df map[string]int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	N = len(m.docs)
	if N == 0 {
		return 0, 1, map[string]int{}
	}
	avgDL = float64(m.totalLen) / float64(N)

	// shallow copy of df
	df = map[string]int{}
	for k, v := range m.df {
		df[k] = v
	}
	return
}

func (m *MemIndex) Candidates(terms []string) map[string]*Doc {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cand := map[string]*Doc{}
	for _, t := range terms {
		for docID := range m.postings[t] {
			cand[docID] = m.docs[docID]
		}
	}
	return cand
}
