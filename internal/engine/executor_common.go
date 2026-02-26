package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// executeCaptures extracts variables from a response body and headers.
// Returns captured values as map[string]any and also writes them into ctxMap.
func executeCaptures(capturesJSON json.RawMessage, body string, headers http.Header, ctxMap CtxMap) map[string]any {
	var rules []CaptureRule
	if len(capturesJSON) > 0 {
		_ = json.Unmarshal(capturesJSON, &rules)
	}

	captured := make(map[string]any)
	for _, c := range rules {
		var val string
		switch c.Source {
		case "header":
			val = headers.Get(c.Path)
		default: // "body" or empty
			r := gjson.Get(body, c.Path)
			if r.Exists() {
				val = r.String()
			}
		}
		if c.As != "" {
			ctxMap[c.As] = val
			captured[c.As] = val
		}
	}
	return captured
}

// executeAssertions checks all assertion rules against a response.
// Returns nil if all pass, or an error describing the first failure.
func executeAssertions(assertionsJSON json.RawMessage, statusCode int, body string, headers http.Header, vars CtxMap) error {
	var rules []AssertionRule
	if len(assertionsJSON) > 0 {
		_ = json.Unmarshal(assertionsJSON, &rules)
	}

	for _, a := range rules {
		var actual string
		switch a.Source {
		case "status":
			actual = strconv.Itoa(statusCode)
		case "header":
			actual = headers.Get(a.Path)
		default: // "body" or empty
			r := gjson.Get(body, a.Path)
			if r.Exists() {
				actual = r.String()
			}
		}

		expected := fmt.Sprintf("%v", a.Value)
		expected = Render(expected, vars)
		if !checkAssertion(actual, a.Op, expected) {
			source := a.Source
			if a.Path != "" {
				source += "." + a.Path
			}
			return fmt.Errorf("assertion failed: %s %s %q, got %q", source, a.Op, expected, actual)
		}
	}
	return nil
}

// checkAssertion evaluates a single assertion condition.
func checkAssertion(actual, op, expected string) bool {
	switch op {
	case OpEq, "":
		return actual == expected
	case OpNe:
		return actual != expected
	case OpContains:
		return strings.Contains(actual, expected)
	case OpGt:
		a, err1 := strconv.ParseFloat(actual, 64)
		e, err2 := strconv.ParseFloat(expected, 64)
		if err1 != nil || err2 != nil {
			return actual > expected
		}
		return a > e
	case OpLt:
		a, err1 := strconv.ParseFloat(actual, 64)
		e, err2 := strconv.ParseFloat(expected, 64)
		if err1 != nil || err2 != nil {
			return actual < expected
		}
		return a < e
	default:
		return false
	}
}

// truncate limits a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
