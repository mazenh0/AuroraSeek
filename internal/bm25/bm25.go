package bm25

import (
	"math"
)

type BM25 struct {
	K1    float64
	B     float64
	AvgDL float64
	N     int
	DF    map[string]int // doc frequency per term
}

func New(N int, avgDL float64, df map[string]int) *BM25 {
	return &BM25{K1: 1.2, B: 0.75, AvgDL: avgDL, N: N, DF: df}
}

func (b *BM25) IDF(term string) float64 {
	df := b.DF[term]
	if df == 0 {
		df = 1
	}
	return math.Log((float64(b.N)-float64(df)+0.5)/(float64(df)+0.5) + 1)
}

func (b *BM25) Score(tf map[string]int, dl int, terms []string) float64 {
	var score float64
	for _, t := range terms {
		f := float64(tf[t])
		if f == 0 {
			continue
		}
		idf := b.IDF(t)
		denom := f + b.K1*(1-b.B+b.B*float64(dl)/b.AvgDL)
		score += idf * (f * (b.K1 + 1)) / denom
	}
	return score
}
