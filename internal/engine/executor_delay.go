package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ExecuteDelay executes a delay step, pausing for the configured duration.
// It respects context cancellation for early exit.
func ExecuteDelay(ctx context.Context, name string, stepCfg json.RawMessage) *StepResult {
	start := time.Now()
	result := &StepResult{Name: name, Type: "delay", Status: "success"}

	var cfg DelayConfig
	if err := json.Unmarshal(stepCfg, &cfg); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid delay config: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	if cfg.DurationMs <= 0 {
		result.DurationMs = 0
		return result
	}

	select {
	case <-time.After(time.Duration(cfg.DurationMs) * time.Millisecond):
		result.DurationMs = time.Since(start).Milliseconds()
	case <-ctx.Done():
		result.DurationMs = time.Since(start).Milliseconds()
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = "timeout"
			result.Error = "delay interrupted by timeout"
		} else {
			result.Status = "failed"
			result.Error = "delay cancelled"
		}
	}

	return result
}
