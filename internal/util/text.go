package util

import (
    "regexp"
    "strings"
)

var ws = regexp.MustCompile(`\s+`)

// Tokenize splits text on whitespace and lowercases; placeholder.
func Tokenize(s string) []string {
    s = strings.ToLower(s)
    s = strings.TrimSpace(s)
    if s == "" {
        return nil
    }
    parts := ws.Split(s, -1)
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        if p != "" {
            out = append(out, p)
        }
    }
    return out
}

