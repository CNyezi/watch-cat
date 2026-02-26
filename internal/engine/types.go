package engine

import "encoding/json"

// StepResult holds the outcome of executing a single step.
type StepResult struct {
	Name       string         `json:"name"`
	Type       string         `json:"type"`                  // http, ws, delay
	Status     string         `json:"status"`                // success, failed, timeout
	DurationMs int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Captures   map[string]any `json:"captures,omitempty"`
	Request     any            `json:"request,omitempty"`     // request summary (debug)
	Response    any            `json:"response,omitempty"`    // response summary (debug)
	ScriptLogs  []string       `json:"script_logs,omitempty"`
	ScriptError string         `json:"script_error,omitempty"`
}

// ExecResult holds the outcome of executing an entire plan.
type ExecResult struct {
	PlanID      string       `json:"plan_id"`
	Status      string       `json:"status"`      // success, failed, timeout
	DurationMs  int64        `json:"duration_ms"`
	StepResults []StepResult `json:"step_results"`
	Error       string       `json:"error,omitempty"`
}

// Assertion operators.
const (
	OpEq       = "eq"
	OpNe       = "ne"
	OpGt       = "gt"
	OpLt       = "lt"
	OpContains = "contains"
)

// HTTPConfig is the JSON structure stored in step.config for type="http".
type HTTPConfig struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// WSMessage defines one send→receive round within a WebSocket step.
type WSMessage struct {
	Send       json.RawMessage `json:"send,omitempty"`
	Expect     map[string]any  `json:"expect,omitempty"`
	Captures   []CaptureRule   `json:"captures,omitempty"`
	Assertions []AssertionRule `json:"assertions,omitempty"`
	// Pomelo-specific fields (ignored for plain ws):
	Route   string `json:"route,omitempty"`    // pomelo route, e.g. "connector.entryHandler.enter"
	MsgType string `json:"msg_type,omitempty"` // "notify" (default) or "request"
}

// WSConfig is the JSON structure stored in step.config for type="ws".
type WSConfig struct {
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers,omitempty"`
	Protocol      string            `json:"protocol,omitempty"`       // "pomelo" or empty (plain ws)
	HandshakeData map[string]any    `json:"handshake_data,omitempty"` // extra fields merged into pomelo handshake
	Messages      []WSMessage       `json:"messages,omitempty"`       // multi-round (preferred)
	// Single-round backward compat fields:
	Send   json.RawMessage `json:"send,omitempty"`
	Expect map[string]any  `json:"expect,omitempty"`
}

// DelayConfig is the JSON structure stored in step.config for type="delay".
type DelayConfig struct {
	DurationMs int `json:"duration_ms"`
}

// CaptureRule defines how to extract a value from a response.
type CaptureRule struct {
	Source string `json:"source"` // body, header
	Path   string `json:"path"`   // gjson path (e.g. data.token)
	As     string `json:"as"`     // variable name to store as
}

// AssertionRule defines a condition to check on a response.
type AssertionRule struct {
	Source string `json:"source"` // status, body, header
	Path   string `json:"path"`   // gjson path
	Op     string `json:"op"`     // eq, ne, gt, lt, contains
	Value  any    `json:"value"`
}
