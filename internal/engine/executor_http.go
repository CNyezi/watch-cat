package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ExecuteHTTP executes an HTTP step.
func ExecuteHTTP(ctx context.Context, name string, stepID, planID uuid.UUID, stepCfg, capturesJSON, assertionsJSON json.RawMessage, timeout int, vars CtxMap, inheritScripts *bool, scriptRunner *ScriptRunner) (*StepResult, CtxMap) {
	start := time.Now()
	result := &StepResult{Name: name, Type: "http", Status: "success"}
	captured := make(CtxMap)

	// Parse config
	var cfg HTTPConfig
	if err := json.Unmarshal(stepCfg, &cfg); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid http config: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}

	// Render template variables
	cfg.URL = Render(cfg.URL, vars)
	cfg.Body = Render(cfg.Body, vars)
	for k, v := range cfg.Headers {
		cfg.Headers[k] = Render(v, vars)
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}

	// 执行 Pre-Scripts（在模板渲染之后、构建请求之前）
	if scriptRunner != nil {
		inherit := inheritScripts == nil || *inheritScripts // 默认 true
		scriptCtx := BuildPreRequestContext(&cfg, vars)
		if err := scriptRunner.RunPreScripts(planID, stepID, inherit, scriptCtx); err != nil {
			result.Status = "failed"
			result.ScriptError = err.Error()
			result.Error = err.Error()
			result.DurationMs = time.Since(start).Milliseconds()
			return result, captured
		}
		// 回写脚本修改到 cfg 和 vars
		ApplyScriptContextToConfig(scriptCtx, &cfg)
		vars = scriptCtx.Vars
		result.ScriptLogs = append(result.ScriptLogs, scriptCtx.Logs...)
	}

	// Store request summary for debug
	result.Request = map[string]any{
		"method":  cfg.Method,
		"url":     cfg.URL,
		"headers": cfg.Headers,
		"body":    cfg.Body,
	}

	// Build request
	var bodyReader io.Reader
	if cfg.Body != "" {
		bodyReader = strings.NewReader(cfg.Body)
	}

	// Apply step-level timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(cfg.Method), cfg.URL, bodyReader)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("build request failed: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = "timeout"
			result.Error = "step timeout exceeded"
		} else {
			result.Status = "failed"
			result.Error = fmt.Sprintf("request failed: %v", err)
		}
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("read response body failed: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}
	bodyStr := string(body)

	// 执行 Post-Scripts（在读取响应体之后、capture/assertion 之前）
	if scriptRunner != nil {
		inherit := inheritScripts == nil || *inheritScripts
		scriptCtx := &ScriptContext{
			Vars:    vars,
			Request: &ScriptRequest{Method: cfg.Method, URL: cfg.URL, Headers: cfg.Headers, Body: cfg.Body},
			Response: &ScriptResponse{
				Status:  resp.StatusCode,
				Headers: flattenHeaders(resp.Header),
				Body:    parseJSONBody(bodyStr),
			},
		}
		if err := scriptRunner.RunPostScripts(planID, stepID, inherit, scriptCtx); err != nil {
			result.ScriptError = err.Error()
			// Post 脚本失败标记但不跳过 capture/assertion
		}
		vars = scriptCtx.Vars
		result.ScriptLogs = append(result.ScriptLogs, scriptCtx.Logs...)
	}

	// Store response summary for debug
	result.Response = map[string]any{
		"status_code": resp.StatusCode,
		"body":        truncate(bodyStr, 4096),
	}

	// Execute captures (common function)
	result.Captures = executeCaptures(capturesJSON, bodyStr, resp.Header, captured)

	// Execute assertions (common function)
	if err := executeAssertions(assertionsJSON, resp.StatusCode, bodyStr, resp.Header, vars); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, captured
}

// flattenHeaders 将 http.Header（多值 map）压平为单值 map。
func flattenHeaders(h http.Header) map[string]string {
	flat := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			flat[k] = v[0]
		}
	}
	return flat
}

// parseJSONBody 尝试将 body 字符串解析为 JSON 对象，失败则返回原始字符串。
func parseJSONBody(body string) any {
	var obj any
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return body
	}
	return obj
}
