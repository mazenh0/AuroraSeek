package bm25

// Scorer defines an interface for BM25-like scoring.
type Scorer interface {
    Score(tf, df, dl, avgDl float64, N float64) float64
}

// BM25 is a simple configurable BM25 scorer placeholder.
type BM25 struct {
    K1 float64
    B  float64
}

// New returns a BM25 with typical defaults.
func New() *BM25 { return &BM25{K1: 1.5, B: 0.75} }

// Score returns a naive placeholder BM25 formula result.
func (b *BM25) Score(tf, df, dl, avgDl float64, N float64) float64 {
    // Minimal, not production-ready; replace with a correct formula.
    if df == 0 || N == 0 {
        return 0
    }
    idf := 1.0 // placeholder idf
    denom := tf + b.K1*(1.0-b.B+b.B*dl/avgDl)
    if denom == 0 {
        return 0
    }
    return idf * ((tf * (b.K1 + 1.0)) / denom)
}

