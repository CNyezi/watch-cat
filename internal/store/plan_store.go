package store

import (
	"watchcat/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PlanStore struct {
	db *gorm.DB
}

func NewPlanStore(db *gorm.DB) *PlanStore {
	return &PlanStore{db: db}
}

func (s *PlanStore) Create(plan *model.Plan) error {
	return s.db.Create(plan).Error
}

func (s *PlanStore) GetByID(id uuid.UUID) (*model.Plan, error) {
	var plan model.Plan
	err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("seq ASC")
	}).First(&plan, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

// List returns all plans ordered by created_at DESC.
// If search is non-empty, filters by name ILIKE.
func (s *PlanStore) List(search string) ([]model.Plan, error) {
	q := s.db.Order("created_at DESC")
	if search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}
	var plans []model.Plan
	err := q.Find(&plans).Error
	return plans, err
}

func (s *PlanStore) Update(plan *model.Plan) error {
	return s.db.Save(plan).Error
}

func (s *PlanStore) Delete(id uuid.UUID) error {
	return s.db.Delete(&model.Plan{}, "id = ?", id).Error
}

func (s *PlanStore) ListEnabled() ([]model.Plan, error) {
	var plans []model.Plan
	err := s.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("seq ASC")
	}).Where("enabled = ?", true).Find(&plans).Error
	return plans, err
}

// ToggleEnabled flips the enabled flag and returns the updated plan.
func (s *PlanStore) ToggleEnabled(id uuid.UUID) (*model.Plan, error) {
	var plan model.Plan
	err := s.db.First(&plan, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	plan.Enabled = !plan.Enabled
	if err := s.db.Model(&plan).Update("enabled", plan.Enabled).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

// Clone deep-copies a plan and all its steps inside a transaction.
// The cloned plan has a new UUID, name suffixed with " (副本)", and enabled=false.
func (s *PlanStore) Clone(id uuid.UUID) (*model.Plan, error) {
	src, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}

	cloned := &model.Plan{
		Name:        src.Name + " (副本)",
		Description: src.Description,
		Cron:        src.Cron,
		Timeout:     src.Timeout,
		Enabled:     false,
		Variables:   src.Variables,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(cloned).Error; err != nil {
			return err
		}
		for _, step := range src.Steps {
			newStep := model.Step{
				PlanID:     cloned.ID,
				Seq:        step.Seq,
				Name:       step.Name,
				Type:       step.Type,
				Config:     step.Config,
				Captures:   step.Captures,
				Assertions: step.Assertions,
				Timeout:    step.Timeout,
			}
			if err := tx.Create(&newStep).Error; err != nil {
				return err
			}
		}

		// 复制 Plan ↔ Script 关联
		var planScripts []model.PlanScript
		if err := tx.Where("plan_id = ?", src.ID).Find(&planScripts).Error; err != nil {
			return err
		}
		for _, ps := range planScripts {
			newPS := model.PlanScript{
				PlanID:   cloned.ID,
				ScriptID: ps.ScriptID,
				Seq:      ps.Seq,
			}
			if err := tx.Create(&newPS).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetByID(cloned.ID)
}
