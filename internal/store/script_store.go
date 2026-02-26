package store

import (
	"watchcat/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ScriptStore struct {
	db *gorm.DB
}

func NewScriptStore(db *gorm.DB) *ScriptStore {
	return &ScriptStore{db: db}
}

func (s *ScriptStore) Create(script *model.Script) error {
	return s.db.Create(script).Error
}

func (s *ScriptStore) GetByID(id uuid.UUID) (*model.Script, error) {
	var script model.Script
	err := s.db.First(&script, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &script, nil
}

func (s *ScriptStore) List(search string) ([]model.Script, error) {
	q := s.db.Order("created_at DESC")
	if search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}
	var scripts []model.Script
	err := q.Find(&scripts).Error
	return scripts, err
}

func (s *ScriptStore) Update(script *model.Script) error {
	return s.db.Save(script).Error
}

func (s *ScriptStore) Delete(id uuid.UUID) error {
	return s.db.Delete(&model.Script{}, "id = ?", id).Error
}

// ReferenceCount 返回脚本被 Plan 和 Step 引用的次数。
func (s *ScriptStore) ReferenceCount(id uuid.UUID) (planCount, stepCount int64, err error) {
	if err = s.db.Model(&model.PlanScript{}).Where("script_id = ?", id).Count(&planCount).Error; err != nil {
		return
	}
	err = s.db.Model(&model.StepScript{}).Where("script_id = ?", id).Count(&stepCount).Error
	return
}

// --- Plan ↔ Script 关联 ---

func (s *ScriptStore) ListByPlanID(planID uuid.UUID) ([]model.PlanScript, error) {
	var items []model.PlanScript
	err := s.db.Preload("Script").Where("plan_id = ?", planID).Order("seq ASC").Find(&items).Error
	return items, err
}

func (s *ScriptStore) BindToPlan(planID, scriptID uuid.UUID, seq int) error {
	ps := &model.PlanScript{PlanID: planID, ScriptID: scriptID, Seq: seq}
	return s.db.Create(ps).Error
}

func (s *ScriptStore) UnbindFromPlan(planID, scriptID uuid.UUID) error {
	return s.db.Where("plan_id = ? AND script_id = ?", planID, scriptID).Delete(&model.PlanScript{}).Error
}

func (s *ScriptStore) UpdatePlanScriptSeq(planID, scriptID uuid.UUID, seq int) error {
	return s.db.Model(&model.PlanScript{}).
		Where("plan_id = ? AND script_id = ?", planID, scriptID).
		Update("seq", seq).Error
}

// --- Step ↔ Script 关联 ---

func (s *ScriptStore) ListByStepID(stepID uuid.UUID) ([]model.StepScript, error) {
	var items []model.StepScript
	err := s.db.Preload("Script").Where("step_id = ?", stepID).Order("seq ASC").Find(&items).Error
	return items, err
}

func (s *ScriptStore) BindToStep(stepID, scriptID uuid.UUID, seq int) error {
	ss := &model.StepScript{StepID: stepID, ScriptID: scriptID, Seq: seq}
	return s.db.Create(ss).Error
}

func (s *ScriptStore) UnbindFromStep(stepID, scriptID uuid.UUID) error {
	return s.db.Where("step_id = ? AND script_id = ?", stepID, scriptID).Delete(&model.StepScript{}).Error
}

// ScriptsForPlan 加载 Plan 执行时需要的所有脚本，按 phase 和 seq 排序。
func (s *ScriptStore) ScriptsForPlan(planID uuid.UUID, phase string) ([]model.Script, error) {
	var scripts []model.Script
	err := s.db.
		Joins("JOIN plan_scripts ON plan_scripts.script_id = scripts.id").
		Where("plan_scripts.plan_id = ? AND scripts.phase = ?", planID, phase).
		Order("plan_scripts.seq ASC").
		Find(&scripts).Error
	return scripts, err
}

func (s *ScriptStore) ScriptsForStep(stepID uuid.UUID, phase string) ([]model.Script, error) {
	var scripts []model.Script
	err := s.db.
		Joins("JOIN step_scripts ON step_scripts.script_id = scripts.id").
		Where("step_scripts.step_id = ? AND scripts.phase = ?", stepID, phase).
		Order("step_scripts.seq ASC").
		Find(&scripts).Error
	return scripts, err
}
