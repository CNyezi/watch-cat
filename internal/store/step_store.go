package store

import (
	"encoding/json"

	"watchcat/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StepStore struct {
	db *gorm.DB
}

func NewStepStore(db *gorm.DB) *StepStore {
	return &StepStore{db: db}
}

func (s *StepStore) Create(step *model.Step) error {
	return s.db.Create(step).Error
}

func (s *StepStore) GetByPlanID(planID uuid.UUID) ([]model.Step, error) {
	var steps []model.Step
	err := s.db.Where("plan_id = ?", planID).Order("seq ASC").Find(&steps).Error
	return steps, err
}

func (s *StepStore) GetByID(id uuid.UUID) (*model.Step, error) {
	var step model.Step
	err := s.db.First(&step, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &step, nil
}

func (s *StepStore) Update(step *model.Step) error {
	return s.db.Save(step).Error
}

func (s *StepStore) Delete(id uuid.UUID) error {
	return s.db.Delete(&model.Step{}, "id = ?", id).Error
}

// UpdateSeqBatch updates step sequence numbers in a transaction.
// ids defines the desired order: ids[0] gets seq=0, ids[1] gets seq=1, etc.
func (s *StepStore) UpdateSeqBatch(planID uuid.UUID, ids []uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for seq, id := range ids {
			if err := tx.Model(&model.Step{}).
				Where("id = ? AND plan_id = ?", id, planID).
				Update("seq", seq).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// captureEntry represents a single capture definition in step's captures JSONB.
type captureEntry struct {
	Name string `json:"as"`
	Path string `json:"path"`
}

// GetAvailableVariables returns variable names available to a step at a given seq.
// This includes plan-level variables and all captures from preceding steps (seq < stepSeq).
// The returned map keys are variable names; values indicate the source ("plan" or "step:<step_name>").
func (s *StepStore) GetAvailableVariables(planID uuid.UUID, stepSeq int) (map[string]string, error) {
	vars := make(map[string]string)

	// Plan-level variables
	var plan model.Plan
	if err := s.db.First(&plan, "id = ?", planID).Error; err != nil {
		return nil, err
	}
	if len(plan.Variables) > 0 {
		var planVars map[string]interface{}
		if err := json.Unmarshal(plan.Variables, &planVars); err == nil {
			for k := range planVars {
				vars[k] = "plan"
			}
		}
	}

	// Captures from preceding steps
	var steps []model.Step
	if err := s.db.Where("plan_id = ? AND seq < ?", planID, stepSeq).
		Order("seq ASC").Find(&steps).Error; err != nil {
		return nil, err
	}
	for _, step := range steps {
		if len(step.Captures) == 0 {
			continue
		}
		var captures []captureEntry
		if err := json.Unmarshal(step.Captures, &captures); err != nil {
			continue
		}
		for _, c := range captures {
			if c.Name != "" {
				vars[c.Name] = "step:" + step.Name
			}
		}
	}

	return vars, nil
}
