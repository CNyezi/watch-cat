package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/robfig/cron/v3"

	"watchcat/internal/engine"
	"watchcat/internal/model"
	"watchcat/internal/store"
)

type PlanHandler struct {
	planStore *store.PlanStore
	stepStore *store.StepStore
	logStore  *store.LogStore
	scheduler *engine.Scheduler
}

func NewPlanHandler(ps *store.PlanStore, ss *store.StepStore, ls *store.LogStore, sched *engine.Scheduler) *PlanHandler {
	return &PlanHandler{
		planStore: ps,
		stepStore: ss,
		logStore:  ls,
		scheduler: sched,
	}
}

func (h *PlanHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/plans", h.List)
	e.GET("/plans/create", h.CreateForm)
	e.POST("/plans", h.Create)
	e.GET("/plans/:id", h.Detail)
	e.GET("/plans/:id/edit", h.EditForm)
	e.PATCH("/plans/:id", h.Update)
	e.PATCH("/plans/:id/toggle", h.Toggle)
	e.DELETE("/plans/:id", h.Delete)
	e.POST("/plans/:id/clone", h.Clone)
}

// List returns all plans, with optional search filter.
func (h *PlanHandler) List(c echo.Context) error {
	search := c.QueryParam("search")
	plans, err := h.planStore.List(search)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.logStore.FillLastRunStatus(plans)

	// Collect plan IDs for uptime bar
	planIDs := make([]uuid.UUID, len(plans))
	for i, p := range plans {
		planIDs[i] = p.ID
	}
	uptimeBars, _ := h.logStore.GetRecentStatuses(planIDs, 15)

	data := map[string]any{
		"Plans":      plans,
		"Search":     search,
		"Nav":        "plans",
		"Title":      "检测计划",
		"UptimeBars": uptimeBars,
	}

	if isHTMX(c) {
		return c.Render(http.StatusOK, "plans/_list.html", data)
	}
	return c.Render(http.StatusOK, "plans/list.html", data)
}

// CreateForm renders the plan creation form.
func (h *PlanHandler) CreateForm(c echo.Context) error {
	return c.Render(http.StatusOK, "plans/create.html", map[string]any{
		"IsEdit": false,
		"Nav":    "plans",
		"Title":  "新建计划",
	})
}

// EditForm renders the plan edit form.
func (h *PlanHandler) EditForm(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	plan, err := h.planStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "计划未找到")
	}

	return c.Render(http.StatusOK, "plans/edit.html", map[string]any{
		"IsEdit": true,
		"Plan":   plan,
		"Nav":    "plans",
		"Title":  "编辑计划 - " + plan.Name,
	})
}

// createRequest is the form/JSON payload for creating a plan.
type createRequest struct {
	Name               string `json:"name" form:"name"`
	Description        string `json:"description" form:"description"`
	Cron               string `json:"cron" form:"cron"`
	Timeout            int    `json:"timeout" form:"timeout"`
	Variables          string `json:"variables" form:"variables"`
	DetailLogThreshold int    `json:"detail_log_threshold" form:"detail_log_threshold"`
}

// Create validates input and creates a new plan.
func (h *PlanHandler) Create(c echo.Context) error {
	var req createRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "名称为必填项")
	}
	if req.Cron == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Cron 表达式为必填项")
	}

	// Validate cron expression using robfig/cron (with seconds)
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(req.Cron); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的 Cron 表达式: "+err.Error())
	}

	// Validate variables JSON
	var varsJSON model.JSON
	if req.Variables != "" {
		var tmp map[string]any
		if err := json.Unmarshal([]byte(req.Variables), &tmp); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "变量必须是有效的 JSON 对象")
		}
		varsJSON = model.JSON(req.Variables)
	} else {
		varsJSON = model.JSON("{}")
	}

	if req.Timeout <= 0 {
		req.Timeout = 30
	}

	plan := &model.Plan{
		Name:               req.Name,
		Description:        req.Description,
		Cron:               req.Cron,
		Timeout:            req.Timeout,
		Enabled:            false,
		Variables:          varsJSON,
		DetailLogThreshold: req.DetailLogThreshold,
	}

	if err := h.planStore.Create(plan); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/plans/"+plan.ID.String())
		return c.NoContent(http.StatusCreated)
	}
	return c.JSON(http.StatusCreated, plan)
}

// Detail renders the plan detail page with steps and recent logs.
func (h *PlanHandler) Detail(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	plan, err := h.planStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "计划未找到")
	}

	// Fetch recent 5 logs
	logs, _, err := h.logStore.ListByPlanID(id, "", 5, "", nil, nil)
	if err != nil {
		logs = nil
	}

	data := map[string]any{
		"Plan":       plan,
		"StepCount":  len(plan.Steps),
		"RecentLogs": logs,
		"Nav":        "plans",
		"Title":      plan.Name,
	}

	if isHTMX(c) {
		return c.Render(http.StatusOK, "plans/_detail.html", data)
	}
	return c.Render(http.StatusOK, "plans/detail.html", data)
}

// updateRequest is the form/JSON payload for updating a plan.
type updateRequest struct {
	Name               *string `json:"name" form:"name"`
	Description        *string `json:"description" form:"description"`
	Cron               *string `json:"cron" form:"cron"`
	Timeout            *int    `json:"timeout" form:"timeout"`
	Variables          *string `json:"variables" form:"variables"`
	DetailLogThreshold *int    `json:"detail_log_threshold" form:"detail_log_threshold"`
}

// Update patches an existing plan and notifies the scheduler.
func (h *PlanHandler) Update(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	plan, err := h.planStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "计划未找到")
	}

	var req updateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}

	if req.Name != nil {
		plan.Name = *req.Name
	}
	if req.Description != nil {
		plan.Description = *req.Description
	}
	if req.Cron != nil {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(*req.Cron); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "无效的 Cron 表达式: "+err.Error())
		}
		plan.Cron = *req.Cron
	}
	if req.Timeout != nil {
		plan.Timeout = *req.Timeout
	}
	if req.Variables != nil {
		var tmp map[string]any
		if err := json.Unmarshal([]byte(*req.Variables), &tmp); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "变量必须是有效的 JSON 对象")
		}
		plan.Variables = model.JSON(*req.Variables)
	}
	if req.DetailLogThreshold != nil {
		plan.DetailLogThreshold = *req.DetailLogThreshold
	}

	if err := h.planStore.Update(plan); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Notify scheduler to reload this plan's cron entry
	if err := h.scheduler.ReloadPlan(id); err != nil {
		c.Logger().Errorf("scheduler reload failed for plan %s: %v", id, err)
	}

	if isHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/plans/"+id.String())
		return c.NoContent(http.StatusOK)
	}
	return c.JSON(http.StatusOK, plan)
}

// Toggle flips the plan's enabled flag and returns the updated row partial.
func (h *PlanHandler) Toggle(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	plan, err := h.planStore.ToggleEnabled(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "计划未找到")
	}

	// Fill LastRunStatus for the single plan
	tmp := []model.Plan{*plan}
	h.logStore.FillLastRunStatus(tmp)
	plan.LastRunStatus = tmp[0].LastRunStatus

	// Notify scheduler
	if err := h.scheduler.ReloadPlan(id); err != nil {
		c.Logger().Errorf("scheduler reload failed for plan %s: %v", id, err)
	}

	if isHTMX(c) {
		entries, _ := h.logStore.GetRecentStatuses([]uuid.UUID{plan.ID}, 15)
		return c.Render(http.StatusOK, "plans/_row.html", map[string]any{
			"Plan":          plan,
			"UptimeEntries": entries[plan.ID],
		})
	}
	return c.JSON(http.StatusOK, plan)
}

// Delete removes a plan and its scheduler entry.
func (h *PlanHandler) Delete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	h.scheduler.RemovePlan(id)

	if err := h.planStore.Delete(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		return c.NoContent(http.StatusOK)
	}
	return c.NoContent(http.StatusNoContent)
}

// Clone deep-copies a plan and redirects to the new plan's detail page.
func (h *PlanHandler) Clone(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	cloned, err := h.planStore.Clone(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/plans/"+cloned.ID.String())
		return c.NoContent(http.StatusCreated)
	}
	return c.JSON(http.StatusCreated, cloned)
}
