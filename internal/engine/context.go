package engine

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// CtxMap is the variable context passed between steps.
type CtxMap map[string]any

// placeholder matches {{varName}} or {{varName.nested.path}}.
var placeholder = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_.]*)\}\}`)

// Merge combines base and overlay maps. overlay values override base.
// Returns a new CtxMap without modifying either input.
func Merge(base, overlay CtxMap) CtxMap {
	result := make(CtxMap, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// resolve looks up a key in the context, supporting dot-notation for nested JSON values.
// For example, if ctx["resp"] = `{"data":{"token":"abc"}}`, then resolve(ctx, "resp.data.token") = "abc".
// Returns (value, found).
func resolve(ctx CtxMap, key string) (string, bool) {
	// Try direct key first
	if val, ok := ctx[key]; ok {
		return toString(val), true
	}

	// Try dot-notation: split at first dot, look up root in ctx, then use gjson for the rest
	dot := strings.IndexByte(key, '.')
	if dot < 0 {
		return "", false
	}

	root := key[:dot]
	path := key[dot+1:]

	val, ok := ctx[root]
	if !ok {
		return "", false
	}

	str := toString(val)
	// Use gjson to extract nested value from JSON string
	result := gjson.Get(str, path)
	if !result.Exists() {
		return "", false
	}
	return result.String(), true
}

// toString converts any value to its string representation.
func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

// Render replaces {{var}} placeholders in a template string with values from ctx.
// Undefined variables are left as-is (e.g. {{unknown}} stays {{unknown}}).
// Nil/empty values are replaced with an empty string.
func Render(template string, ctx CtxMap) string {
	return placeholder.ReplaceAllStringFunc(template, func(match string) string {
		// Extract key from {{key}}
		key := match[2 : len(match)-2]
		if val, found := resolve(ctx, key); found {
			return val
		}
		// Undefined variable: keep original placeholder
		return match
	})
}

// RenderJSON is an alias for Render, operating on JSON strings.
// It replaces {{var}} placeholders inside JSON content.
func RenderJSON(jsonStr string, ctx CtxMap) string {
	return Render(jsonStr, ctx)
}
