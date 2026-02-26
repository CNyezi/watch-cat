package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"watchcat/internal/store"
)

type LogHandler struct {
	logStore *store.LogStore
}

func NewLogHandler(ls *store.LogStore) *LogHandler {
	return &LogHandler{logStore: ls}
}

func (h *LogHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/plans/:id/logs", h.List)
	e.GET("/plans/:id/logs/:lid", h.Detail)
}

// List returns exec logs for a plan with cursor-based pagination and filters.
func (h *LogHandler) List(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	cursor := c.QueryParam("cursor")
	status := c.QueryParam("status")

	var from, to *time.Time
	if f := c.QueryParam("from"); f != "" {
		t, err := time.Parse(time.RFC3339, f)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "无效的 'from' 时间格式，请使用 RFC3339")
		}
		from = &t
	}
	if raw := c.QueryParam("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "无效的 'to' 时间格式，请使用 RFC3339")
		}
		to = &t
	}

	logs, nextCursor, err := h.logStore.ListByPlanID(planID, cursor, 0, status, from, to)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	fromStr := c.QueryParam("from")
	toStr := c.QueryParam("to")

	data := map[string]any{
		"PlanID":  planID,
		"Logs":    logs,
		"Cursor":  nextCursor,
		"HasMore": nextCursor != "",
		"Filters": map[string]string{
			"Status": status,
			"From":   fromStr,
			"To":     toStr,
		},
	}

	if isHTMX(c) {
		return c.Render(http.StatusOK, "logs/_list.html", data)
	}
	return c.JSON(http.StatusOK, data)
}

// Detail returns a single exec log entry.
func (h *LogHandler) Detail(c echo.Context) error {
	lid, err := uuid.Parse(c.Param("lid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的日志 ID")
	}

	execLog, err := h.logStore.GetByID(lid)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "日志未找到")
	}

	if isHTMX(c) {
		return c.Render(http.StatusOK, "logs/_detail.html", map[string]any{"Log": execLog})
	}
	return c.JSON(http.StatusOK, execLog)
}
