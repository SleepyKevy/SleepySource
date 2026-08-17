package main

import "strings"

func splitSourceTokens(value string) []string {
	parts := strings.Split(strings.ToLower(value), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sourceMatchesAny(source string, tokens []string) bool {
	source = strings.ToLower(source)
	for _, token := range tokens {
		if strings.Contains(source, token) {
			return true
		}
	}
	return false
}

func mediaSourceAllowed(source string, s Settings) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return false
	}
	if sourceMatchesAny(source, splitSourceTokens(s.MediaSourceExclude)) {
		return false
	}
	switch s.MediaSourceMode {
	case "any":
		return true
	case "custom":
		include := splitSourceTokens(s.MediaSourceInclude)
		return len(include) > 0 && sourceMatchesAny(source, include)
	default:
		return strings.Contains(source, "spotify")
	}
}
