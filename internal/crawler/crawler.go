package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Crawler handles web crawling
type Crawler struct {
	client      *http.Client
	visited     map[string]bool
	visitedMu   sync.RWMutex
	rateLimiter *RateLimiter
	userAgent   string
	maxDepth    int
	timeout     time.Duration
}

// Page represents a crawled page
type Page struct {
	URL   string
	Title string
	Body  string
	Links []string
}

// RateLimiter limits request rate per domain
type RateLimiter struct {
	delays map[string]time.Time
	mu     sync.Mutex
	delay  time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(delay time.Duration) *RateLimiter {
	return &RateLimiter{
		delays: make(map[string]time.Time),
		delay:  delay,
	}
}

// Wait waits for the rate limit on the given domain
func (r *RateLimiter) Wait(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if last, ok := r.delays[domain]; ok {
		elapsed := time.Since(last)
		if elapsed < r.delay {
			time.Sleep(r.delay - elapsed)
		}
	}
	r.delays[domain] = time.Now()
}

// NewCrawler creates a new web crawler
func NewCrawler() *Crawler {
	return &Crawler{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		visited:     make(map[string]bool),
		rateLimiter: NewRateLimiter(1 * time.Second), // 1 second delay between requests per domain
		userAgent:   "AuroraSeek/1.0 (+https://github.com/mazenh0/auroraseek)",
		maxDepth:    3,
		timeout:     30 * time.Second,
	}
}

// NormalizeURL normalizes a URL by removing fragments and trailing slashes
func NormalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Remove fragment
	u.Fragment = ""

	// Normalize scheme
	u.Scheme = strings.ToLower(u.Scheme)

	// Normalize host
	u.Host = strings.ToLower(u.Host)

	// Remove trailing slash from path (except root)
	if u.Path != "/" && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimSuffix(u.Path, "/")
	}
	
	return u.String(), nil
}

// IsVisited checks if a URL has been visited
func (c *Crawler) IsVisited(url string) bool {
	c.visitedMu.RLock()
	defer c.visitedMu.RUnlock()
	return c.visited[url]
}

// MarkVisited marks a URL as visited
func (c *Crawler) MarkVisited(url string) {
	c.visitedMu.Lock()
	defer c.visitedMu.Unlock()
	c.visited[url] = true
}

// Fetch fetches a URL and returns the page content
func (c *Crawler) Fetch(ctx context.Context, rawURL string) (*Page, error) {
	// Normalize URL
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize URL: %w", err)
	}

	// Check if already visited
	if c.IsVisited(normalized) {
		return nil, fmt.Errorf("already visited: %s", normalized)
	}

	// Get domain for rate limiting
	u, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	domain := u.Host

	// Rate limit
	c.rateLimiter.Wait(domain)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", normalized, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	// Fetch page
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return nil, fmt.Errorf("not HTML content: %s", contentType)
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Parse HTML
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Extract title
	title := doc.Find("title").First().Text()
	title = strings.TrimSpace(title)
	if title == "" {
		title = normalized // Fallback to URL
	}

	// Extract body text (remove scripts and styles)
	doc.Find("script, style, noscript").Remove()
	bodyText := doc.Find("body").Text()
	bodyText = strings.TrimSpace(bodyText)

	// Extract links
	var links []string
	baseURL := u // Use the parsed URL as base
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		// Parse the href (could be relative or absolute)
		hrefURL, err := url.Parse(href)
		if err != nil {
			return
		}

		// Resolve relative URLs against the base URL
		absURL := baseURL.ResolveReference(hrefURL)

		// Only HTTP/HTTPS links
		if absURL.Scheme != "http" && absURL.Scheme != "https" {
			return
		}

		// Normalize
		normalizedLink, err := NormalizeURL(absURL.String())
		if err != nil {
			return
		}

		// Skip if already visited
		if c.IsVisited(normalizedLink) {
			return
		}

		links = append(links, normalizedLink)
	})

	// Mark as visited
	c.MarkVisited(normalized)

	return &Page{
		URL:   normalized,
		Title: title,
		Body:  bodyText,
		Links: links,
	}, nil
}

// CrawlQueue manages a queue of URLs to crawl
type CrawlQueue struct {
	queue   []string
	mu      sync.Mutex
	visited map[string]bool
}

// NewCrawlQueue creates a new crawl queue
func NewCrawlQueue() *CrawlQueue {
	return &CrawlQueue{
		queue:   make([]string, 0),
		visited: make(map[string]bool),
	}
}

// Add adds URLs to the queue (deduplicated)
func (q *CrawlQueue) Add(urls ...string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, u := range urls {
		if !q.visited[u] {
			q.visited[u] = true
			q.queue = append(q.queue, u)
		}
	}
}

// Next returns the next URL from the queue
func (q *CrawlQueue) Next() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) == 0 {
		return "", false
	}

	url := q.queue[0]
	q.queue = q.queue[1:]
	return url, true
}

// Size returns the size of the queue
func (q *CrawlQueue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

// IsEmpty checks if the queue is empty
func (q *CrawlQueue) IsEmpty() bool {
	return q.Size() == 0
}
