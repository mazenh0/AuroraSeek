package main


import (
"context"
"encoding/json"
"log"
"net/http"
"os"
"github.com/mazenh0/auroraseek/internal/index"
"github.com/mazenh0/auroraseek/internal/kafka"
)


type Page struct { ID, URL, Title, Body string }


var mem = index.NewMem()


func main(){
brokers := []string{os.Getenv("KAFKA_BROKERS")}
topic := getenv("KAFKA_TOPIC_PAGES", "pages")
group := "indexer"
r := kafka.NewReader(brokers, topic, group)
defer r.Close()


go func(){
http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request){ w.WriteHeader(200); w.Write([]byte("ok")) })
log.Fatal(http.ListenAndServe(":8080", nil))
}()


for {
m, err := r.ReadMessage(context.Background())
if err != nil { log.Fatal(err) }
var pg Page
if err := json.Unmarshal(m.Value, &pg); err != nil { continue }
mem.Add(pg.ID, pg.URL, pg.Title, pg.Body)
log.Printf("indexed %s", pg.ID)
}
}


func getenv(k, d string) string { if v:=os.Getenv(k); v!="" {return v}; return d }

