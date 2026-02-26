package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"watchcat/internal/model"
	"watchcat/internal/store"
)

type AlertHandler struct {
	logs  *store.LogStore
	plans *store.PlanStore
}

func NewAlertHandler(logs *store.LogStore, plans *store.PlanStore) *AlertHandler {
	return &AlertHandler{logs: logs, plans: plans}
}

func (h *AlertHandler) Register(e *echo.Echo) {
	e.GET("/errors", h.List)
}

// alertLogEntry holds a single failure log for display in the alerts page.
type alertLogEntry struct {
	LogID      uuid.UUID
	PlanID     uuid.UUID
	PlanName   string
	Status     string
	Error      string
	FailedStep string
	Duration   string
	StartedAt  interface{ Format(string) string }
}

// alertFilters holds parsed query filters for the alerts page.
type alertFilters struct {
	PlanID uuid.UUID
	From   string
	To     string
}

// List renders the global error aggregation page.
func (h *AlertHandler) List(c echo.Context) error {
	cursor := c.QueryParam("cursor")
	planIDStr := c.QueryParam("plan_id")
	fromStr := c.QueryParam("from")
	toStr := c.QueryParam("to")

	var planID *uuid.UUID
	var filterPlanID uuid.UUID
	if planIDStr != "" {
		id, err := uuid.Parse(planIDStr)
		if err == nil {
			planID = &id
			filterPlanID = id
		}
	}

	var from, to *time.Time
	if fromStr != "" {
		if t, err := time.Parse("2006-01-02T15:04", fromStr); err == nil {
			from = &t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02T15:04", toStr); err == nil {
			to = &t
		}
	}

	logs, nextCursor, err := h.logs.ListFailed(cursor, 20, planID, from, to)
	if err != nil {
		return err
	}

	// Build a plan name lookup
	planNames := h.buildPlanNameMap(logs)

	// Build display entries
	entries := make([]alertLogEntry, 0, len(logs))
	for _, l := range logs {
		entry := alertLogEntry{
			LogID:     l.ID,
			PlanID:    l.PlanID,
			PlanName:  planNames[l.PlanID],
			Status:    l.Status,
			Error:     l.Error,
			Duration:  formatDuration(l.DurationMs),
			StartedAt: l.CreatedAt,
		}
		// Extract failed step name from step results
		entry.FailedStep = extractFailedStep(l.StepResults)
		entries = append(entries, entry)
	}

	// Get all plans for the filter dropdown
	allPlans, _ := h.plans.List("")

	data := map[string]any{
		"Nav":   "errors",
		"Title": "错误汇总",
		"Plans": allPlans,
		"Logs":  entries,
		"Filters": alertFilters{
			PlanID: filterPlanID,
			From:   fromStr,
			To:     toStr,
		},
		"Cursor":  nextCursor,
		"HasMore": nextCursor != "",
	}

	if isHTMX(c) {
		return c.Render(http.StatusOK, "alerts", data)
	}
	return c.Render(http.StatusOK, "alerts", data)
}

// buildPlanNameMap creates a map of plan ID -> name for display.
func (h *AlertHandler) buildPlanNameMap(logs []model.ExecLog) map[uuid.UUID]string {
	names := make(map[uuid.UUID]string)
	for _, l := range logs {
		if _, ok := names[l.PlanID]; !ok {
			plan, err := h.plans.GetByID(l.PlanID)
			if err == nil && plan != nil {
				names[l.PlanID] = plan.Name
			}
		}
	}
	return names
}

// extractFailedStep finds the name of the first failed step in step results JSON.
func extractFailedStep(stepResultsJSON model.JSON) string {
	if len(stepResultsJSON) == 0 {
		return ""
	}
	var results []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stepResultsJSON, &results); err != nil {
		return ""
	}
	for _, r := range results {
		if r.Status == "failed" || r.Status == "timeout" {
			return r.Name
		}
	}
	return ""
}
