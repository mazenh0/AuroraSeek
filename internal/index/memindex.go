package index

import "sync"

// Doc represents a simple document in the index.
type Doc struct {
    ID    string
    Title string
    Text  string
}

// Posting captures term frequency within a document.
type Posting struct {
    DocID string
    TF    int
}

// MemIndex is an in-memory inverted index placeholder.
type MemIndex struct {
    mu       sync.RWMutex
    postings map[string][]Posting // term -> postings
    docs     map[string]Doc
    totalDl  int
}

// NewMem creates a new empty memory index.
func NewMem() *MemIndex {
    return &MemIndex{
        postings: make(map[string][]Posting),
        docs:     make(map[string]Doc),
    }
}

// AddDoc indexes a document with a simple whitespace tokenizer.
func (mi *MemIndex) AddDoc(d Doc, tokenize func(string) []string) {
    mi.mu.Lock()
    defer mi.mu.Unlock()
    mi.docs[d.ID] = d
    terms := tokenize(d.Text)
    tf := map[string]int{}
    for _, t := range terms {
        tf[t]++
    }
    for term, cnt := range tf {
        mi.postings[term] = append(mi.postings[term], Posting{DocID: d.ID, TF: cnt})
    }
    mi.totalDl += len(terms)
}

// Stats returns document count and average document length.
func (mi *MemIndex) Stats() (docs int, avgDl float64) {
    mi.mu.RLock()
    defer mi.mu.RUnlock()
    n := len(mi.docs)
    if n == 0 {
        return 0, 0
    }
    return n, float64(mi.totalDl) / float64(n)
}

// Postings returns postings for a term.
func (mi *MemIndex) Postings(term string) []Posting {
    mi.mu.RLock()
    defer mi.mu.RUnlock()
    return mi.postings[term]
}

// Doc returns a document by ID.
func (mi *MemIndex) Doc(id string) (Doc, bool) {
    mi.mu.RLock()
    defer mi.mu.RUnlock()
    d, ok := mi.docs[id]
    return d, ok
}

