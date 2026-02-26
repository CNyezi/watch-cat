package engine

import (
	"testing"
)

func TestScriptEngine_BasicVarsAccess(t *testing.T) {
	se := NewScriptEngine()
	ctx := &ScriptContext{
		Vars: CtxMap{"host": "example.com", "token": "abc123"},
	}
	code := `ctx.vars.computed = ctx.vars.host + "/api";`
	if err := se.Run(code, ctx); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if ctx.Vars["computed"] != "example.com/api" {
		t.Errorf("expected 'example.com/api', got %v", ctx.Vars["computed"])
	}
}

func TestScriptEngine_ModifyRequest(t *testing.T) {
	se := NewScriptEngine()
	ctx := &ScriptContext{
		Vars: CtxMap{},
		Request: &ScriptRequest{
			Method:  "POST",
			URL:     "https://api.example.com/login",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"head": map[string]any{}, "body": map[string]any{"name": "test"}},
		},
	}
	code := `ctx.request.headers["X-Custom"] = "hello"; ctx.request.body.head.sign = "computed";`
	if err := se.Run(code, ctx); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if ctx.Request.Headers["X-Custom"] != "hello" {
		t.Errorf("expected header X-Custom=hello, got %v", ctx.Request.Headers["X-Custom"])
	}
	body := ctx.Request.Body.(map[string]any)
	head := body["head"].(map[string]any)
	if head["sign"] != "computed" {
		t.Errorf("expected sign=computed, got %v", head["sign"])
	}
}

func TestScriptEngine_CryptoSHA1(t *testing.T) {
	se := NewScriptEngine()
	ctx := &ScriptContext{Vars: CtxMap{}}
	// SHA1("hello") = "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	code := `ctx.vars.hash = crypto.sha1("hello");`
	if err := se.Run(code, ctx); err != nil {
		t.Fatalf("script error: %v", err)
	}
	expected := "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	if ctx.Vars["hash"] != expected {
		t.Errorf("expected %s, got %v", expected, ctx.Vars["hash"])
	}
}

func TestScriptEngine_CryptoMD5(t *testing.T) {
	se := NewScriptEngine()
	ctx := &ScriptContext{Vars: CtxMap{}}
	// MD5("hello") = "5d41402abc4b2a76b9719d911017c592"
	code := `ctx.vars.hash = crypto.md5("hello");`
	if err := se.Run(code, ctx); err != nil {
		t.Fatalf("script error: %v", err)
	}
	expected := "5d41402abc4b2a76b9719d911017c592"
	if ctx.Vars["hash"] != expected {
		t.Errorf("expected %s, got %v", expected, ctx.Vars["hash"])
	}
}

func TestScriptEngine_Timeout(t *testing.T) {
	se := NewScriptEngine()
	ctx := &ScriptContext{Vars: CtxMap{}}
	code := `while(true) {}`
	err := se.Run(code, ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestScriptEngine_ReadResponse(t *testing.T) {
	se := NewScriptEngine()
	ctx := &ScriptContext{
		Vars: CtxMap{},
		Response: &ScriptResponse{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    map[string]any{"data": map[string]any{"token": "xyz"}},
		},
	}
	code := `ctx.vars.status = ctx.response.status; ctx.vars.token = ctx.response.body.data.token;`
	if err := se.Run(code, ctx); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if ctx.Vars["token"] != "xyz" {
		t.Errorf("expected xyz, got %v", ctx.Vars["token"])
	}
}

func TestScriptEngine_SignatureScript(t *testing.T) {
	// 模拟用户的 Apifox 签名脚本场景
	se := NewScriptEngine()
	ctx := &ScriptContext{
		Vars: CtxMap{"signKey": "kH2msJT79pE28HvPizIJ7nojG8udHqSB"},
		Request: &ScriptRequest{
			Method:  "POST",
			URL:     "https://api.example.com/rpc",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body: map[string]any{
				"head": map[string]any{},
				"body": map[string]any{
					"name":   "test",
					"amount": 100,
				},
			},
		},
	}
	code := `
var body = ctx.request.body.body;
var keys = Object.keys(body).sort();
var s = "";
for (var i = 0; i < keys.length; i++) {
    var k = keys[i];
    if (body[k] === "" || body[k] === false || body[k] === 0) continue;
    s += k + "=" + body[k] + "&";
}
s += "key=" + ctx.vars.signKey;
ctx.request.body.head.sign = crypto.sha1(s);
`
	if err := se.Run(code, ctx); err != nil {
		t.Fatalf("script error: %v", err)
	}
	body := ctx.Request.Body.(map[string]any)
	head := body["head"].(map[string]any)
	if head["sign"] == nil || head["sign"] == "" {
		t.Error("expected sign to be set, got empty")
	}
}
