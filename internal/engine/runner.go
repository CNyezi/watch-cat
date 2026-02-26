package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"watchcat/internal/model"
)

// StepCallback is invoked after each step completes, useful for SSE progress updates.
type StepCallback func(stepIndex int, result *StepResult)

// Runner orchestrates sequential execution of a plan's steps.
type Runner struct {
	mu           sync.Mutex
	locks        map[uuid.UUID]*sync.Mutex
	metrics      MetricsRecorder
	scriptRunner *ScriptRunner
}

// MetricsRecorder is the subset of metrics.Metrics that Runner needs.
// This avoids a circular import between engine and metrics packages.
type MetricsRecorder interface {
	IncPlansRunning()
	DecPlansRunning()
}

// NewRunner creates a new Runner instance.
func NewRunner() *Runner {
	return &Runner{
		locks: make(map[uuid.UUID]*sync.Mutex),
	}
}

// SetMetrics attaches a metrics recorder to the runner.
func (r *Runner) SetMetrics(m MetricsRecorder) {
	r.metrics = m
}

// SetScriptRunner 设置脚本执行器。
func (r *Runner) SetScriptRunner(sr *ScriptRunner) {
	r.scriptRunner = sr
}

// getPlanLock returns the per-plan mutex, creating one if needed.
func (r *Runner) getPlanLock(planID uuid.UUID) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.locks[planID]; !ok {
		r.locks[planID] = &sync.Mutex{}
	}
	return r.locks[planID]
}

// IsRunning checks if a plan is currently being executed.
func (r *Runner) IsRunning(planID uuid.UUID) bool {
	planLock := r.getPlanLock(planID)
	if planLock.TryLock() {
		planLock.Unlock()
		return false
	}
	return true
}

// Execute runs all steps of a plan sequentially.
// It acquires a per-plan lock to prevent concurrent execution of the same plan.
// The optional callback is invoked after each step completes.
func (r *Runner) Execute(ctx context.Context, plan *model.Plan, steps []model.Step, callback StepCallback) *ExecResult {
	// Acquire per-plan lock (non-blocking)
	planLock := r.getPlanLock(plan.ID)
	if !planLock.TryLock() {
		return &ExecResult{
			PlanID: plan.ID.String(),
			Status: "failed",
			Error:  "plan is already running",
		}
	}
	defer planLock.Unlock()

	if r.metrics != nil {
		r.metrics.IncPlansRunning()
		defer r.metrics.DecPlansRunning()
	}

	start := time.Now()
	result := &ExecResult{
		PlanID:      plan.ID.String(),
		Status:      "success",
		StepResults: make([]StepResult, 0, len(steps)),
	}

	// Apply plan-level timeout
	if plan.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(plan.Timeout)*time.Second)
		defer cancel()
	}

	// Initialize context map from plan variables
	vars := make(CtxMap)
	if len(plan.Variables) > 0 {
		var planVars map[string]any
		if err := json.Unmarshal(plan.Variables, &planVars); err == nil {
			for k, v := range planVars {
				vars[k] = v
			}
		}
	}

	// Execute steps sequentially
	for i, step := range steps {
		// Check if context is already cancelled before starting next step
		if ctx.Err() != nil {
			result.Status = "timeout"
			result.Error = "plan timeout exceeded"
			break
		}

		stepResult, captured := r.executeStep(ctx, plan.ID, step, vars)
		result.StepResults = append(result.StepResults, *stepResult)

		// Notify callback
		if callback != nil {
			callback(i, stepResult)
		}

		// Merge captured variables into context
		if len(captured) > 0 {
			vars = Merge(vars, captured)
		}

		// If step failed or timed out, stop execution
		if stepResult.Status != "success" {
			result.Status = stepResult.Status
			result.Error = fmt.Sprintf("step %d [%s] %s: %s", i+1, step.Name, stepResult.Status, stepResult.Error)
			break
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

// TrimResultsIfNeeded 根据执行结果和 Plan 配置决定是否裁剪步骤详情。
// 成功执行且总耗时低于 Plan 的 DetailLogThreshold 时裁剪。
func TrimResultsIfNeeded(result *ExecResult, thresholdSec int) {
	if thresholdSec <= 0 {
		return
	}
	if result.Status != "success" {
		return
	}
	if result.DurationMs >= int64(thresholdSec)*1000 {
		return
	}
	trimStepResults(result.StepResults)
}

// trimStepResults 裁剪成功执行的步骤结果，只保留摘要信息。
func trimStepResults(results []StepResult) {
	for i := range results {
		results[i].Request = nil
		results[i].Response = nil
		results[i].Captures = nil
		results[i].ScriptLogs = nil
		results[i].ScriptError = ""
		results[i].Error = ""
	}
}

// executeStep dispatches a single step to the appropriate executor based on type.
func (r *Runner) executeStep(ctx context.Context, planID uuid.UUID, step model.Step, vars CtxMap) (*StepResult, CtxMap) {
	cfg := json.RawMessage(step.Config)
	captures := json.RawMessage(step.Captures)
	assertions := json.RawMessage(step.Assertions)

	switch step.Type {
	case "http":
		return ExecuteHTTP(ctx, step.Name, step.ID, planID, cfg, captures, assertions, step.Timeout, vars, step.InheritScripts, r.scriptRunner)
	case "ws":
		return ExecuteWS(ctx, step.Name, cfg, captures, assertions, step.Timeout, vars)
	case "delay":
		return ExecuteDelay(ctx, step.Name, cfg), make(CtxMap)
	default:
		return &StepResult{
			Name:   step.Name,
			Type:   step.Type,
			Status: "failed",
			Error:  fmt.Sprintf("unknown step type: %s", step.Type),
		}, make(CtxMap)
	}
}
