package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"watchcat/internal/engine"
	"watchcat/internal/model"
	"watchcat/internal/store"
)

type ExecHandler struct {
	planStore *store.PlanStore
	logStore  *store.LogStore
	runner    *engine.Runner
}

func NewExecHandler(ps *store.PlanStore, ls *store.LogStore, runner *engine.Runner) *ExecHandler {
	return &ExecHandler{
		planStore: ps,
		logStore:  ls,
		runner:    runner,
	}
}

func (h *ExecHandler) RegisterRoutes(e *echo.Echo) {
	e.POST("/plans/:id/exec", h.Exec)
	e.GET("/plans/:id/exec/stream", h.Stream)
}

// Exec triggers a one-shot execution and returns the log ID.
func (h *ExecHandler) Exec(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	plan, err := h.planStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "计划未找到")
	}

	if h.runner.IsRunning(plan.ID) {
		return echo.NewHTTPError(http.StatusConflict, "该计划正在执行中")
	}

	ctx := c.Request().Context()
	result := h.runner.Execute(ctx, plan, plan.Steps, nil)
	engine.TrimResultsIfNeeded(result, plan.DetailLogThreshold)

	// Persist exec log
	stepResultsJSON, _ := json.Marshal(result.StepResults)
	execLog := &model.ExecLog{
		PlanID:      plan.ID,
		Status:      result.Status,
		DurationMs:  int(result.DurationMs),
		StepResults: model.JSON(stepResultsJSON),
		Error:       result.Error,
	}
	if err := h.logStore.Create(execLog); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "保存执行日志失败")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"log_id": execLog.ID,
		"status": result.Status,
	})
}

// Stream provides an SSE endpoint for real-time execution progress.
func (h *ExecHandler) Stream(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	plan, err := h.planStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "计划未找到")
	}

	if h.runner.IsRunning(plan.ID) {
		return echo.NewHTTPError(http.StatusConflict, "该计划正在执行中")
	}

	// Set SSE headers
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "不支持流式传输")
	}

	// Callback pushes each step result as an SSE event
	callback := func(idx int, result *engine.StepResult) {
		data, _ := json.Marshal(map[string]any{
			"index":       idx,
			"name":        result.Name,
			"type":        result.Type,
			"status":      result.Status,
			"error":       result.Error,
			"duration_ms": result.DurationMs,
		})
		fmt.Fprintf(c.Response(), "event: step-result\ndata: %s\n\n", data)
		flusher.Flush()
	}

	// Execute with client context (cancelled on disconnect)
	ctx := c.Request().Context()
	execResult := h.runner.Execute(ctx, plan, plan.Steps, callback)
	engine.TrimResultsIfNeeded(execResult, plan.DetailLogThreshold)

	// Persist exec log
	stepResultsJSON, _ := json.Marshal(execResult.StepResults)
	execLog := &model.ExecLog{
		PlanID:      plan.ID,
		Status:      execResult.Status,
		DurationMs:  int(execResult.DurationMs),
		StepResults: model.JSON(stepResultsJSON),
		Error:       execResult.Error,
	}
	_ = h.logStore.Create(execLog)

	// Push done event
	doneData, _ := json.Marshal(map[string]any{
		"log_id":      execLog.ID,
		"status":      execResult.Status,
		"duration_ms": execResult.DurationMs,
		"error":       execResult.Error,
	})
	fmt.Fprintf(c.Response(), "event: done\ndata: %s\n\n", doneData)
	flusher.Flush()

	return nil
}
