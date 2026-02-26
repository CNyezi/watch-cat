package engine

import (
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

const defaultScriptTimeout = 5 * time.Second

// ScriptRequest 表示 JS 脚本中可操作的请求对象。
type ScriptRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"` // 解析后的 JSON 对象或原始字符串
}

// ScriptResponse 表示 JS 脚本中可读取的响应对象。
type ScriptResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

// ScriptContext 是注入到 JS 脚本中的 ctx 对象。
type ScriptContext struct {
	Vars     CtxMap          `json:"vars"`
	Request  *ScriptRequest  `json:"request,omitempty"`
	Response *ScriptResponse `json:"response,omitempty"`
	Logs     []string        `json:"-"` // console.log 输出，不暴露给 JS
}

// ScriptEngine 管理 goja JS 虚拟机的创建和脚本执行。
type ScriptEngine struct {
	timeout time.Duration
}

// NewScriptEngine 创建一个新的脚本引擎实例。
func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{timeout: defaultScriptTimeout}
}

// Run 在隔离的 goja VM 中执行 JavaScript 代码。
// 脚本可以通过 ctx 对象读写变量、请求和响应。
func (se *ScriptEngine) Run(code string, ctx *ScriptContext) error {
	vm := goja.New()

	// 使用 json tag 作为字段名映射，方法名首字母自动小写
	// 如：Vars→vars, Method→method, Sha1→sha1
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// 注入 ctx 对象
	vm.Set("ctx", ctx)

	// 注入 crypto 模块
	vm.Set("crypto", &CryptoModule{})

	// 注入 console.log — 输出写入 ctx.Logs，最终显示在 StepResult.ScriptLogs
	console := vm.NewObject()
	logFn := func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			parts[i] = arg.String()
		}
		line := fmt.Sprint(strings.Join(parts, " "))
		ctx.Logs = append(ctx.Logs, line)
		return goja.Undefined()
	}
	console.Set("log", logFn)
	console.Set("warn", logFn)
	console.Set("error", logFn)
	vm.Set("console", console)

	// 设置超时保护
	timer := time.AfterFunc(se.timeout, func() {
		vm.Interrupt("脚本执行超时")
	})
	defer timer.Stop()

	_, err := vm.RunString(code)
	if err != nil {
		return fmt.Errorf("脚本执行失败: %w", err)
	}
	return nil
}
