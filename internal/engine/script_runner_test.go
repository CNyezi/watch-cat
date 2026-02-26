package engine

import (
	"encoding/json"
	"testing"
)

func TestFullSignatureWorkflow(t *testing.T) {
	se := NewScriptEngine()

	// 模拟 HTTPConfig
	bodyJSON := `{"head":{},"body":{"name":"test","amount":100}}`
	var bodyObj any
	json.Unmarshal([]byte(bodyJSON), &bodyObj)

	ctx := &ScriptContext{
		Vars: CtxMap{"signKey": "testkey123"},
		Request: &ScriptRequest{
			Method:  "POST",
			URL:     "https://api.example.com/rpc",
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    bodyObj,
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

	// 验证签名被写入
	reqBody := ctx.Request.Body.(map[string]any)
	head := reqBody["head"].(map[string]any)
	sign, ok := head["sign"].(string)
	if !ok || sign == "" {
		t.Fatal("sign not set in head")
	}
	t.Logf("computed sign: %s", sign)

	// 验证回写到 config
	cfg := &HTTPConfig{Body: bodyJSON}
	ApplyScriptContextToConfig(ctx, cfg)
	if cfg.Body == bodyJSON {
		t.Error("body should have been modified by script")
	}
	t.Logf("modified body: %s", cfg.Body)
}
