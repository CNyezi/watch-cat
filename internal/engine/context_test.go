package engine

import (
	"testing"
)

func TestMerge(t *testing.T) {
	base := CtxMap{"a": "1", "b": "2"}
	overlay := CtxMap{"b": "override", "c": "3"}
	result := Merge(base, overlay)

	if result["a"] != "1" {
		t.Errorf("expected a=1, got %v", result["a"])
	}
	if result["b"] != "override" {
		t.Errorf("expected b=override, got %v", result["b"])
	}
	if result["c"] != "3" {
		t.Errorf("expected c=3, got %v", result["c"])
	}
	// Ensure base was not modified
	if base["b"] != "2" {
		t.Errorf("base was modified: b=%v", base["b"])
	}
}

func TestRenderSimple(t *testing.T) {
	ctx := CtxMap{"name": "world", "port": 8080}
	got := Render("Hello {{name}} on port {{port}}", ctx)
	want := "Hello world on port 8080"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUndefined(t *testing.T) {
	ctx := CtxMap{"a": "1"}
	got := Render("{{a}} and {{b}}", ctx)
	want := "1 and {{b}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderNil(t *testing.T) {
	ctx := CtxMap{"empty": nil}
	got := Render("value={{empty}}", ctx)
	want := "value="
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderNestedJSON(t *testing.T) {
	ctx := CtxMap{
		"resp": `{"data":{"token":"abc123","count":42}}`,
	}
	got := Render("Token: {{resp.data.token}}, Count: {{resp.data.count}}", ctx)
	want := "Token: abc123, Count: 42"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderJSON(t *testing.T) {
	ctx := CtxMap{"token": "xyz", "base_url": "https://api.example.com"}
	got := RenderJSON(`{"url":"{{base_url}}/auth","headers":{"Authorization":"Bearer {{token}}"}}`, ctx)
	want := `{"url":"https://api.example.com/auth","headers":{"Authorization":"Bearer xyz"}}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
