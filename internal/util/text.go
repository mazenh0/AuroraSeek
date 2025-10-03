package util

import (
	"regexp"
	"strings"
)

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func Normalize(s string) string {
	s = strings.ToLower(s)
	s = nonAlphaNum.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func Tokens(s string) []string {
	s = Normalize(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}
