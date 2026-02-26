package handler

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"watchcat/internal/model"
	"watchcat/internal/store"
)

type ScriptHandler struct {
	scriptStore *store.ScriptStore
}

func NewScriptHandler(ss *store.ScriptStore) *ScriptHandler {
	return &ScriptHandler{scriptStore: ss}
}

func (h *ScriptHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/scripts", h.List)
	e.GET("/scripts/create", h.CreateForm)
	e.POST("/scripts", h.Create)
	e.GET("/scripts/:id", h.Detail)
	e.GET("/scripts/:id/edit", h.EditForm)
	e.PATCH("/scripts/:id", h.Update)
	e.DELETE("/scripts/:id", h.Delete)

	// Plan ↔ Script 关联
	e.GET("/plans/:id/scripts", h.PlanScripts)
	e.POST("/plans/:id/scripts", h.BindToPlan)
	e.DELETE("/plans/:id/scripts/:scriptId", h.UnbindFromPlan)

	// Step ↔ Script 关联
	e.GET("/steps/:id/scripts", h.StepScripts)
	e.POST("/steps/:id/scripts", h.BindToStep)
	e.DELETE("/steps/:id/scripts/:scriptId", h.UnbindFromStep)
}

// --- 脚本库 CRUD ---

func (h *ScriptHandler) List(c echo.Context) error {
	search := c.QueryParam("search")
	scripts, err := h.scriptStore.List(search)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	data := map[string]any{
		"Scripts": scripts,
		"Search":  search,
		"Nav":     "scripts",
		"Title":   "脚本库",
	}
	if isHTMX(c) {
		return c.Render(http.StatusOK, "scripts/_list.html", data)
	}
	return c.Render(http.StatusOK, "scripts/list.html", data)
}

func (h *ScriptHandler) CreateForm(c echo.Context) error {
	return c.Render(http.StatusOK, "scripts/create.html", map[string]any{
		"IsEdit": false,
		"Nav":    "scripts",
		"Title":  "新建脚本",
	})
}

type scriptRequest struct {
	Name        string `json:"name" form:"name"`
	Description string `json:"description" form:"description"`
	Phase       string `json:"phase" form:"phase"`
	Code        string `json:"code" form:"code"`
}

func (h *ScriptHandler) Create(c echo.Context) error {
	var req scriptRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "名称为必填项")
	}
	if req.Phase != "pre" && req.Phase != "post" {
		return echo.NewHTTPError(http.StatusBadRequest, "phase 必须为 pre 或 post")
	}
	if req.Code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "脚本代码为必填项")
	}

	script := &model.Script{
		Name:        req.Name,
		Description: req.Description,
		Phase:       req.Phase,
		Code:        req.Code,
	}
	if err := h.scriptStore.Create(script); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/scripts/"+script.ID.String())
		return c.NoContent(http.StatusCreated)
	}
	return c.JSON(http.StatusCreated, script)
}

func (h *ScriptHandler) Detail(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}
	script, err := h.scriptStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "脚本未找到")
	}

	planCount, stepCount, _ := h.scriptStore.ReferenceCount(id)

	data := map[string]any{
		"Script":    script,
		"PlanCount": planCount,
		"StepCount": stepCount,
		"Nav":       "scripts",
		"Title":     script.Name,
	}
	if isHTMX(c) {
		return c.Render(http.StatusOK, "scripts/_detail.html", data)
	}
	return c.Render(http.StatusOK, "scripts/detail.html", data)
}

func (h *ScriptHandler) EditForm(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}
	script, err := h.scriptStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "脚本未找到")
	}
	return c.Render(http.StatusOK, "scripts/edit.html", map[string]any{
		"IsEdit": true,
		"Script": script,
		"Nav":    "scripts",
		"Title":  "编辑脚本 - " + script.Name,
	})
}

func (h *ScriptHandler) Update(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}
	script, err := h.scriptStore.GetByID(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "脚本未找到")
	}

	var req scriptRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}
	if req.Name != "" {
		script.Name = req.Name
	}
	if req.Description != "" {
		script.Description = req.Description
	}
	if req.Phase == "pre" || req.Phase == "post" {
		script.Phase = req.Phase
	}
	if req.Code != "" {
		script.Code = req.Code
	}

	if err := h.scriptStore.Update(script); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/scripts/"+id.String())
		return c.NoContent(http.StatusOK)
	}
	return c.JSON(http.StatusOK, script)
}

func (h *ScriptHandler) Delete(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}

	planCount, stepCount, _ := h.scriptStore.ReferenceCount(id)
	if planCount+stepCount > 0 && c.QueryParam("force") != "true" {
		return echo.NewHTTPError(http.StatusConflict,
			fmt.Sprintf("该脚本被 %d 个 Plan、%d 个 Step 引用，需确认强制删除", planCount, stepCount))
	}

	if err := h.scriptStore.Delete(id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if isHTMX(c) {
		c.Response().Header().Set("HX-Redirect", "/scripts")
		return c.NoContent(http.StatusOK)
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Plan ↔ Script 关联管理 ---

func (h *ScriptHandler) PlanScripts(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}
	items, err := h.scriptStore.ListByPlanID(planID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	allScripts, _ := h.scriptStore.List("")

	data := map[string]any{
		"PlanID":      planID,
		"PlanScripts": items,
		"AllScripts":  allScripts,
	}
	return c.Render(http.StatusOK, "scripts/_plan_scripts.html", data)
}

type bindRequest struct {
	ScriptID string `json:"script_id" form:"script_id"`
	Seq      int    `json:"seq" form:"seq"`
}

func (h *ScriptHandler) BindToPlan(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}
	var req bindRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}
	scriptID, err := uuid.Parse(req.ScriptID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}
	if err := h.scriptStore.BindToPlan(planID, scriptID, req.Seq); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return h.PlanScripts(c)
}

func (h *ScriptHandler) UnbindFromPlan(c echo.Context) error {
	planID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的计划 ID")
	}
	scriptID, err := uuid.Parse(c.Param("scriptId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}
	if err := h.scriptStore.UnbindFromPlan(planID, scriptID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return h.PlanScripts(c)
}

// --- Step ↔ Script 关联管理 ---

func (h *ScriptHandler) StepScripts(c echo.Context) error {
	stepID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID")
	}
	items, err := h.scriptStore.ListByStepID(stepID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	allScripts, _ := h.scriptStore.List("")

	data := map[string]any{
		"StepID":      stepID,
		"StepScripts": items,
		"AllScripts":  allScripts,
	}
	return c.Render(http.StatusOK, "scripts/_step_scripts.html", data)
}

func (h *ScriptHandler) BindToStep(c echo.Context) error {
	stepID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID")
	}
	var req bindRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的请求体")
	}
	scriptID, err := uuid.Parse(req.ScriptID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}
	if err := h.scriptStore.BindToStep(stepID, scriptID, req.Seq); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return h.StepScripts(c)
}

func (h *ScriptHandler) UnbindFromStep(c echo.Context) error {
	stepID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的步骤 ID")
	}
	scriptID, err := uuid.Parse(c.Param("scriptId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "无效的脚本 ID")
	}
	if err := h.scriptStore.UnbindFromStep(stepID, scriptID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return h.StepScripts(c)
}
