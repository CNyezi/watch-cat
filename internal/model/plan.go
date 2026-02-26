package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Plan struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Cron        string    `gorm:"type:varchar(100);not null" json:"cron"`
	Timeout     int       `gorm:"default:30" json:"timeout"`
	Enabled     bool      `gorm:"default:true;index" json:"enabled"`
	Variables          JSON      `gorm:"type:jsonb;default:'{}'" json:"variables"`
	DetailLogThreshold int       `gorm:"default:0" json:"detail_log_threshold"` // 秒，0=始终记录详情
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Steps []Step `gorm:"foreignKey:PlanID;constraint:OnDelete:CASCADE" json:"steps,omitempty"`

	LastRunStatus string `gorm:"-" json:"last_run_status,omitempty"`
}

func (p *Plan) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// VariablesMap returns the plan variables as a map for template rendering.
func (p *Plan) VariablesMap() map[string]any {
	m := make(map[string]any)
	if len(p.Variables) > 0 {
		_ = json.Unmarshal(p.Variables, &m)
	}
	return m
}

// VariablesJSON returns the raw JSON string of variables for template script injection.
func (p *Plan) VariablesJSON() string {
	if len(p.Variables) == 0 {
		return "{}"
	}
	return string(p.Variables)
}
