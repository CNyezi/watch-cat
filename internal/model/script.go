package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Script 脚本库中的一个脚本。
type Script struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Phase       string    `gorm:"type:varchar(20);not null" json:"phase"` // "pre" | "post"
	Code        string    `gorm:"type:text;not null" json:"code"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Script) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// PlanScript 关联 Plan 和 Script（多对多中间表）。
type PlanScript struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PlanID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_plan_script" json:"plan_id"`
	ScriptID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_plan_script" json:"script_id"`
	Seq      int       `gorm:"not null;default:0" json:"seq"`

	Script Script `gorm:"foreignKey:ScriptID;constraint:OnDelete:CASCADE" json:"script,omitempty"`
}

func (ps *PlanScript) BeforeCreate(tx *gorm.DB) error {
	if ps.ID == uuid.Nil {
		ps.ID = uuid.New()
	}
	return nil
}

// StepScript 关联 Step 和 Script（多对多中间表）。
type StepScript struct {
	ID       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	StepID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_step_script" json:"step_id"`
	ScriptID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_step_script" json:"script_id"`
	Seq      int       `gorm:"not null;default:0" json:"seq"`

	Script Script `gorm:"foreignKey:ScriptID;constraint:OnDelete:CASCADE" json:"script,omitempty"`
}

func (ss *StepScript) BeforeCreate(tx *gorm.DB) error {
	if ss.ID == uuid.Nil {
		ss.ID = uuid.New()
	}
	return nil
}
