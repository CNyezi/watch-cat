package engine

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"

	"watchcat/internal/model"
	"watchcat/internal/store"
)

// ScriptRunner 负责加载和执行 Plan/Step 关联的脚本。
type ScriptRunner struct {
	scriptStore  *store.ScriptStore
	scriptEngine *ScriptEngine
}

// NewScriptRunner 创建脚本执行编排器。
func NewScriptRunner(ss *store.ScriptStore) *ScriptRunner {
	return &ScriptRunner{
		scriptStore:  ss,
		scriptEngine: NewScriptEngine(),
	}
}

// BuildPreRequestContext 根据 HTTPConfig 和 vars 构建脚本上下文。
func BuildPreRequestContext(cfg *HTTPConfig, vars CtxMap) *ScriptContext {
	// 尝试将 body 解析为 JSON 对象
	var bodyObj any
	if cfg.Body != "" {
		if err := json.Unmarshal([]byte(cfg.Body), &bodyObj); err != nil {
			bodyObj = cfg.Body // 保持原始字符串
		}
	}

	return &ScriptContext{
		Vars: vars,
		Request: &ScriptRequest{
			Method:  cfg.Method,
			URL:     cfg.URL,
			Headers: cfg.Headers,
			Body:    bodyObj,
		},
	}
}

// ApplyScriptContextToConfig 将脚本修改后的上下文回写到 HTTPConfig。
func ApplyScriptContextToConfig(ctx *ScriptContext, cfg *HTTPConfig) {
	cfg.Method = ctx.Request.Method
	cfg.URL = ctx.Request.URL
	cfg.Headers = ctx.Request.Headers

	// 将 body 回写：如果是对象则序列化为 JSON，否则转为字符串
	switch b := ctx.Request.Body.(type) {
	case string:
		cfg.Body = b
	case nil:
		cfg.Body = ""
	default:
		data, err := json.Marshal(b)
		if err != nil {
			log.Printf("[script] body 序列化失败: %v", err)
		} else {
			cfg.Body = string(data)
		}
	}
}

// RunPreScripts 执行某个 HTTP 步骤的所有前置脚本（Plan 继承 + Step 自身）。
func (sr *ScriptRunner) RunPreScripts(planID, stepID uuid.UUID, inheritScripts bool, ctx *ScriptContext) error {
	if sr.scriptStore == nil {
		return nil
	}

	var scripts []model.Script

	// Plan 级 Pre 脚本
	if inheritScripts {
		planScripts, err := sr.scriptStore.ScriptsForPlan(planID, "pre")
		if err != nil {
			return fmt.Errorf("加载 Plan Pre 脚本失败: %w", err)
		}
		scripts = append(scripts, planScripts...)
	}

	// Step 级 Pre 脚本
	stepScripts, err := sr.scriptStore.ScriptsForStep(stepID, "pre")
	if err != nil {
		return fmt.Errorf("加载 Step Pre 脚本失败: %w", err)
	}
	scripts = append(scripts, stepScripts...)

	// 逐个执行
	for _, s := range scripts {
		if err := sr.scriptEngine.Run(s.Code, ctx); err != nil {
			return fmt.Errorf("Pre 脚本 [%s] 执行失败: %w", s.Name, err)
		}
	}

	return nil
}

// RunPostScripts 执行某个 HTTP 步骤的所有后置脚本。
func (sr *ScriptRunner) RunPostScripts(planID, stepID uuid.UUID, inheritScripts bool, ctx *ScriptContext) error {
	if sr.scriptStore == nil {
		return nil
	}

	var scripts []model.Script

	if inheritScripts {
		planScripts, err := sr.scriptStore.ScriptsForPlan(planID, "post")
		if err != nil {
			return fmt.Errorf("加载 Plan Post 脚本失败: %w", err)
		}
		scripts = append(scripts, planScripts...)
	}

	stepScripts, err := sr.scriptStore.ScriptsForStep(stepID, "post")
	if err != nil {
		return fmt.Errorf("加载 Step Post 脚本失败: %w", err)
	}
	scripts = append(scripts, stepScripts...)

	for _, s := range scripts {
		if err := sr.scriptEngine.Run(s.Code, ctx); err != nil {
			return fmt.Errorf("Post 脚本 [%s] 执行失败: %w", s.Name, err)
		}
	}

	return nil
}
