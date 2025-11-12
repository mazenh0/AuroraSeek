package index

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"go.etcd.io/bbolt"
)

const (
	// Default database file name
	DefaultDBPath = "/data/index.db"

	// Bucket names
	bucketDocs     = "docs"
	bucketPostings = "postings"
	bucketDF       = "df"
	bucketMeta     = "meta"
)

// PersistentIndex wraps MemIndex with bbolt persistence
type PersistentIndex struct {
	*MemIndex
	db   *bbolt.DB
	path string
	mu   sync.RWMutex
}

// NewPersistent creates a new persistent index that loads from disk if it exists
func NewPersistent(dbPath string) (*PersistentIndex, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create index directory: %w", err)
	}

	// Open bbolt database
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize buckets
	err = db.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range []string{bucketDocs, bucketPostings, bucketDF, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	pi := &PersistentIndex{
		MemIndex: NewMem(),
		db:       db,
		path:     dbPath,
	}

	// Load existing data from disk
	if err := pi.load(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	return pi, nil
}

// load reads all data from bbolt and populates the in-memory index
func (p *PersistentIndex) load() error {
	return p.db.View(func(tx *bbolt.Tx) error {
		// Load documents
		docsBucket := tx.Bucket([]byte(bucketDocs))
		if docsBucket == nil {
			return nil // No data yet
		}

		// Track which documents we've loaded to avoid duplicates
		loadedDocs := make(map[string]bool)

		c := docsBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			docID := string(k)
			if loadedDocs[docID] {
				continue // Skip duplicates
			}
			loadedDocs[docID] = true

			var doc Doc
			if err := json.Unmarshal(v, &doc); err != nil {
				log.Printf("Warning: failed to unmarshal document %s: %v", docID, err)
				continue // Skip corrupted documents
			}

			// Rebuild in-memory structures using Add method to ensure consistency
			p.MemIndex.mu.Lock()
			p.MemIndex.docs[docID] = &doc
			p.MemIndex.totalLen += doc.DL

			// Rebuild postings and df
			seen := map[string]bool{}
			for term, tf := range doc.TF {
				if p.MemIndex.postings[term] == nil {
					p.MemIndex.postings[term] = map[string]int{}
				}
				p.MemIndex.postings[term][docID] = tf
				// Only increment df once per document per term
				if !seen[term] {
					p.MemIndex.df[term]++
					seen[term] = true
				}
			}
			p.MemIndex.mu.Unlock()
		}

		return nil
	})
}

// Add persists a document to disk and updates in-memory index
func (p *PersistentIndex) Add(id, url, title, body string) error {
	// First update in-memory index (for fast queries)
	p.MemIndex.Add(id, url, title, body)

	// Get the document we just added
	p.MemIndex.mu.RLock()
	doc := p.MemIndex.docs[id]
	p.MemIndex.mu.RUnlock()

	if doc == nil {
		return fmt.Errorf("document not found after adding")
	}

	// Persist to disk
	return p.db.Update(func(tx *bbolt.Tx) error {
		// Store document
		docBytes, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal document: %w", err)
		}

		docsBucket := tx.Bucket([]byte(bucketDocs))
		if err := docsBucket.Put([]byte(id), docBytes); err != nil {
			return fmt.Errorf("failed to store document: %w", err)
		}

		// Update document frequency (df) for each term in this document
		dfBucket := tx.Bucket([]byte(bucketDF))
		p.MemIndex.mu.RLock()
		// Copy df map to avoid race conditions
		dfCopy := make(map[string]int)
		for term, count := range p.MemIndex.df {
			dfCopy[term] = count
		}
		p.MemIndex.mu.RUnlock()

		// Store df values for all terms in this document
		seen := map[string]bool{}
		for term := range doc.TF {
			// Only update df once per term in this document
			if !seen[term] {
				count := dfCopy[term]
				dfBytes := make([]byte, 8)
				binary.BigEndian.PutUint64(dfBytes, uint64(count))
				if err := dfBucket.Put([]byte(term), dfBytes); err != nil {
					return fmt.Errorf("failed to update df: %w", err)
				}
				seen[term] = true
			}
		}

		// Store postings (term -> docID -> tf)
		postingsBucket := tx.Bucket([]byte(bucketPostings))
		for term, tf := range doc.TF {
			termBucket, err := postingsBucket.CreateBucketIfNotExists([]byte(term))
			if err != nil {
				return fmt.Errorf("failed to create term bucket: %w", err)
			}

			tfBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(tfBytes, uint64(tf))
			if err := termBucket.Put([]byte(id), tfBytes); err != nil {
				return fmt.Errorf("failed to store posting: %w", err)
			}
		}

		// Update metadata (total document count, total length)
		metaBucket := tx.Bucket([]byte(bucketMeta))
		p.MemIndex.mu.RLock()
		N := len(p.MemIndex.docs)
		totalLen := p.MemIndex.totalLen
		p.MemIndex.mu.RUnlock()

		meta := map[string]interface{}{
			"doc_count": N,
			"total_len": totalLen,
		}
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		if err := metaBucket.Put([]byte("stats"), metaBytes); err != nil {
			return fmt.Errorf("failed to store metadata: %w", err)
		}

		return nil
	})
}

// Close closes the database connection
func (p *PersistentIndex) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// Stats returns index statistics
func (p *PersistentIndex) Stats() (docCount int, totalLen int, err error) {
	p.MemIndex.mu.RLock()
	defer p.MemIndex.mu.RUnlock()
	return len(p.MemIndex.docs), p.MemIndex.totalLen, nil
}

