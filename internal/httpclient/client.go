package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// Client is an HTTP client with timeout and retry logic
type Client struct {
	httpClient *http.Client
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// Config configures the HTTP client
type Config struct {
	Timeout    time.Duration
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		Timeout:    10 * time.Second,
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
	}
}

// NewClient creates a new HTTP client with timeout and retry logic
func NewClient(config Config) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		maxRetries: config.MaxRetries,
		baseDelay:  config.BaseDelay,
		maxDelay:   config.MaxDelay,
	}
}

// PostJSON sends a POST request with JSON body and retry logic
func (c *Client) PostJSON(ctx context.Context, url string, body interface{}) (*http.Response, error) {
	// Marshal request body
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	var lastErr error
	delay := c.baseDelay

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// Create request with context
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		// Execute request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < c.maxRetries {
				log.Printf("HTTP request failed (attempt %d/%d): %v, retrying in %v", attempt+1, c.maxRetries+1, err, delay)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
					// Exponential backoff
					delay = time.Duration(float64(delay) * 2)
					if delay > c.maxDelay {
						delay = c.maxDelay
					}
				}
				continue
			}
			return nil, fmt.Errorf("HTTP request failed after %d attempts: %w", c.maxRetries+1, err)
		}

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			lastErr = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			if attempt < c.maxRetries && isRetryableStatus(resp.StatusCode) {
				log.Printf("HTTP request returned status %d (attempt %d/%d), retrying in %v", resp.StatusCode, attempt+1, c.maxRetries+1, delay)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
					// Exponential backoff
					delay = time.Duration(float64(delay) * 2)
					if delay > c.maxDelay {
						delay = c.maxDelay
					}
				}
				continue
			}
			return nil, lastErr
		}

		// Success
		if attempt > 0 {
			log.Printf("HTTP request succeeded after %d attempts", attempt+1)
		}
		return resp, nil
	}

	return nil, fmt.Errorf("HTTP request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// isRetryableStatus checks if a status code is retryable
func isRetryableStatus(statusCode int) bool {
	// Retry on server errors (5xx) and some client errors (429, 408)
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

// DecodeJSON decodes JSON response body
func DecodeJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return nil
}

