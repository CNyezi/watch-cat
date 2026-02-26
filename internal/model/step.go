package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Step struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PlanID     uuid.UUID `gorm:"type:uuid;not null;index:idx_steps_plan_seq" json:"plan_id"`
	Seq        int       `gorm:"not null;index:idx_steps_plan_seq" json:"seq"`
	Name       string    `gorm:"type:varchar(255);not null" json:"name"`
	Type       string    `gorm:"type:varchar(20);not null" json:"type"` // http, ws, delay
	Config     JSON      `gorm:"type:jsonb;default:'{}'" json:"config"`
	Captures   JSON      `gorm:"type:jsonb;default:'[]'" json:"captures"`
	Assertions JSON      `gorm:"type:jsonb;default:'[]'" json:"assertions"`
	Timeout        int   `gorm:"default:10" json:"timeout"`
	InheritScripts *bool `gorm:"default:true" json:"inherit_scripts"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (s *Step) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// ConfigJSON returns the Config field as a raw JSON string for use in templates.
func (s *Step) ConfigJSON() string {
	if len(s.Config) == 0 {
		return "{}"
	}
	return string(s.Config)
}

// CapturesJSON returns the Captures field as a raw JSON string for use in templates.
func (s *Step) CapturesJSON() string {
	if len(s.Captures) == 0 {
		return "[]"
	}
	return string(s.Captures)
}

// AssertionsJSON returns the Assertions field as a raw JSON string for use in templates.
func (s *Step) AssertionsJSON() string {
	if len(s.Assertions) == 0 {
		return "[]"
	}
	return string(s.Assertions)
}

// StepConfigSummary holds extracted summary fields from a step's Config JSON.
type StepConfigSummary struct {
	Method   string
	URL      string
	Duration string
}

// ConfigSummary parses the Config JSON and returns summary fields based on the step type.
func (s *Step) ConfigSummary() StepConfigSummary {
	var summary StepConfigSummary
	if len(s.Config) == 0 {
		return summary
	}

	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(s.Config, &cfg); err != nil {
		return summary
	}

	switch s.Type {
	case "http":
		var method, url string
		if raw, ok := cfg["method"]; ok {
			json.Unmarshal(raw, &method)
		}
		if raw, ok := cfg["url"]; ok {
			json.Unmarshal(raw, &url)
		}
		summary.Method = method
		summary.URL = url
	case "ws":
		var url string
		if raw, ok := cfg["url"]; ok {
			json.Unmarshal(raw, &url)
		}
		summary.URL = url
	case "delay":
		var durationMs int
		if raw, ok := cfg["duration_ms"]; ok {
			json.Unmarshal(raw, &durationMs)
		}
		if durationMs >= 1000 {
			summary.Duration = fmt.Sprintf("%.1fs", float64(durationMs)/1000)
		} else {
			summary.Duration = fmt.Sprintf("%dms", durationMs)
		}
	}

	return summary
}
