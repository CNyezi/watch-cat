package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"watchcat/internal/model"
	"watchcat/internal/store"
)

type StepHandler struct {
	stepStore *store.StepStore
}

func NewStepHandler(ss *store.StepStore) *StepHandler {
	return &StepHandler{stepStore: ss}
}

func (h *StepHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/plans/:id/steps", h.List)
	e.GET("/plans/:id/steps/create", h.CreateForm)
	e.POST("/plans/:id/steps", h.Create)
	e.GET("/plans/:id/steps/:sid/edit", h.EditForm)
	e.PUT("/plans/:id/steps/:sid", h.Update)
	e.DELETE("/plans/:id/steps/:sid", h.Delete)
	e.POST("/plans/:id/steps/reorder", h.Reorder)
	e.GET("/plans/:id/steps/:sid/variables", h.Variables)
}

// List returns steps for a plan.
func (h *StepHandler) List(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	steps, err := h.stepStore.GetByPlanID(planID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	data := map[string]any{
		"PlanID": planID,
		"Steps":  steps,
	}

	if isHTMX(c) {
		return c.Render(http.StatusOK, "steps/_list.html", data)
	}
	return c.JSON(http.StatusOK, steps)
}

// CreateForm renders the step creation form partial.
func (h *StepHandler) CreateForm(c echo.Context) error {
	planID := c.Param("id")
	parsedPlanID, _ := uuid.Parse(planID)

	// New step is appended at the end; seq = len(existing steps)
	existing, _ := h.stepStore.GetByPlanID(parsedPlanID)
	vars, _ := h.stepStore.GetAvailableVariables(parsedPlanID, len(existing))
	varNames := make([]string, 0, len(vars))
	for k := range vars {
		varNames = append(varNames, k)
	}

	return c.Render(http.StatusOK, "steps/_edit.html", map[string]any{
		"PlanID":        planID,
		"Step":          nil,
		"IsEdit":        false,
		"AvailableVars": varNames,
	})
}

// stepRequest is the JSON payload for creating/updating a step.
type stepRequest struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Config     json.RawMessage `json:"config"`
	Captures   json.RawMessage `json:"captures"`
	Assertions json.RawMessage `json:"assertions"`
	Timeout    int             `json:"timeout"`
}

// Create adds a new step to a plan.
func (h *StepHandler) Create(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	var req stepRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "名称为必填项")
	}
	if req.Type == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "类型为必填项")
	}

	// Determine next seq number
	existing, err := h.stepStore.GetByPlanID(planID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	nextSeq := len(existing)

	if req.Timeout <= 0 {
		req.Timeout = 10
	}

	config := model.JSON("{}")
	if len(req.Config) > 0 {
		config = model.JSON(req.Config)
	}
	captures := model.JSON("[]")
	if len(req.Captures) > 0 {
		captures = model.JSON(req.Captures)
	}
	assertions := model.JSON("[]")
	if len(req.Assertions) > 0 {
		assertions = model.JSON(req.Assertions)
	}

	step := &model.Step{
		PlanID:     planID,
		Seq:        nextSeq,
		Name:       req.Name,
		Type:       req.Type,
		Config:     config,
		Captures:   captures,
		Assertions: assertions,
		Timeout:    req.Timeout,
	}

	if err := h.stepStore.Create(step); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		steps, _ := h.stepStore.GetByPlanID(planID)
		return c.Render(http.StatusCreated, "steps/_list.html", map[string]any{
			"PlanID": planID,
			"Steps":  steps,
		})
	}
	return c.JSON(http.StatusCreated, step)
}

// EditForm renders the edit form partial for a step.
func (h *StepHandler) EditForm(c echo.Context) error {
	sid, err := uuid.Parse(c.Param("sid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID")
	}

	step, err := h.stepStore.GetByID(sid)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "步骤未找到")
	}

	planID := c.Param("id")
	parsedPlanID, _ := uuid.Parse(planID)

	// Get available variables for this step's position
	vars, _ := h.stepStore.GetAvailableVariables(parsedPlanID, step.Seq)
	varNames := make([]string, 0, len(vars))
	for k := range vars {
		varNames = append(varNames, k)
	}

	return c.Render(http.StatusOK, "steps/_edit.html", map[string]any{
		"PlanID":        planID,
		"Step":          step,
		"IsEdit":        true,
		"AvailableVars": varNames,
	})
}

// Update modifies an existing step.
func (h *StepHandler) Update(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	sid, err := uuid.Parse(c.Param("sid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID")
	}

	step, err := h.stepStore.GetByID(sid)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "步骤未找到")
	}

	var req stepRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}

	if req.Name != "" {
		step.Name = req.Name
	}
	if req.Type != "" {
		step.Type = req.Type
	}
	if len(req.Config) > 0 {
		step.Config = model.JSON(req.Config)
	}
	if len(req.Captures) > 0 {
		step.Captures = model.JSON(req.Captures)
	}
	if len(req.Assertions) > 0 {
		step.Assertions = model.JSON(req.Assertions)
	}
	if req.Timeout > 0 {
		step.Timeout = req.Timeout
	}

	if err := h.stepStore.Update(step); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		steps, _ := h.stepStore.GetByPlanID(planID)
		return c.Render(http.StatusOK, "steps/_list.html", map[string]any{
			"PlanID": planID,
			"Steps":  steps,
		})
	}
	return c.JSON(http.StatusOK, step)
}

// Delete removes a step.
func (h *StepHandler) Delete(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	sid, err := uuid.Parse(c.Param("sid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID")
	}

	if err := h.stepStore.Delete(sid); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		steps, _ := h.stepStore.GetByPlanID(planID)
		return c.Render(http.StatusOK, "steps/_list.html", map[string]any{
			"PlanID": planID,
			"Steps":  steps,
		})
	}
	return c.NoContent(http.StatusNoContent)
}

// reorderRequest holds the ordered list of step IDs.
type reorderRequest struct {
	IDs []string `json:"ids"`
}

// Reorder updates the seq of all steps in a plan.
func (h *StepHandler) Reorder(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	var req reorderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}

	ids := make([]uuid.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID: "+raw)
		}
		ids = append(ids, id)
	}

	if err := h.stepStore.UpdateSeqBatch(planID, ids); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		steps, _ := h.stepStore.GetByPlanID(planID)
		return c.Render(http.StatusOK, "steps/_list.html", map[string]any{
			"PlanID": planID,
			"Steps":  steps,
		})
	}
	return c.NoContent(http.StatusOK)
}

// Variables returns the available variables for a specific step.
func (h *StepHandler) Variables(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}

	sid, err := uuid.Parse(c.Param("sid"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID")
	}

	step, err := h.stepStore.GetByID(sid)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "步骤未找到")
	}

	vars, err := h.stepStore.GetAvailableVariables(planID, step.Seq)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, vars)
}
