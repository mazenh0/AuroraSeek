package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"

	pb "github.com/mazenh0/auroraseek/gen/searchpb"
	"github.com/mazenh0/auroraseek/internal/bm25"
	"github.com/mazenh0/auroraseek/internal/index"
	"github.com/mazenh0/auroraseek/internal/util"
	"google.golang.org/grpc"
)

// For demo, share in-memory index in-process.
var mem = index.NewMem()

// Seed a couple docs so Search works even before indexer loads.
func seed() {
	mem.Add("1", "https://example.com", "Example Domain", "This domain is for use in illustrative examples in documents.")
	mem.Add("2", "https://golang.org", "Go", "Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.")
}

type server struct {
	pb.UnimplementedSearchServiceServer
}

func (s *server) Search(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	terms := util.Tokens(req.Query)
	cand := mem.Candidates(terms)

	N, avgDL, df := mem.Snapshot()
	bm := bm25.New(N, avgDL, df)

	type scored struct {
		d     *index.Doc
		score float64
	}
	var list []scored

	for _, d := range cand {
		list = append(list, scored{d, bm.Score(d.TF, d.DL, terms)})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].score > list[j].score
	})

	// Prepare candidates for reranker
	top := min(len(list), max(20, int(req.K)))
	cands := make([]map[string]string, 0, top)
	for i := 0; i < top; i++ {
		cands = append(cands, map[string]string{
			"id":    list[i].d.ID,
			"title": list[i].d.Title,
			"body":  list[i].d.Body,
		})
	}

	reranked := callReranker(os.Getenv("RERANKER_URL"), req.Query, cands)

	// Merge reranked order
	id2score := map[string]float64{}
	for i, id := range reranked {
		id2score[id] = float64(len(reranked) - i)
	}

	sort.Slice(list, func(i, j int) bool {
		si := id2score[list[i].d.ID]
		sj := id2score[list[j].d.ID]
		if si == sj {
			return list[i].score > list[j].score
		}
		return si > sj
	})

	k := min(len(list), int(req.K))
	res := &pb.QueryResponse{}
	for i := 0; i < k; i++ {
		d := list[i].d
		res.Results = append(res.Results, &pb.ScoredDocument{
			Doc: &pb.Document{
				Id:    d.ID,
				Url:   d.URL,
				Title: d.Title,
				Body:  truncate(d.Body, 512),
			},
			Score: list[i].score,
		})
	}

	return res, nil
}

func callReranker(url, query string, cands []map[string]string) []string {
	if url == "" {
		return ids(cands)
	}

	body := map[string]any{"query": query, "candidates": cands}
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", strings.NewReader(string(b)))
	if err != nil {
		return ids(cands)
	}
	defer resp.Body.Close()

	var out struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ids(cands)
	}
	if len(out.Order) == 0 {
		return ids(cands)
	}
	return out.Order
}

func ids(c []map[string]string) []string {
	r := make([]string, len(c))
	for i, x := range c {
		r[i] = x["id"]
	}
	return r
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func main() {
	seed()

	lis, err := net.Listen("tcp", getenv("QUERY_ADDR", ":50051"))
	if err != nil {
		log.Fatal(err)
	}

	s := grpc.NewServer()
	pb.RegisterSearchServiceServer(s, &server{})
	log.Println("query service listening on", lis.Addr())

	if err := s.Serve(lis); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
