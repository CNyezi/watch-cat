package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ExecLog struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	PlanID      uuid.UUID `gorm:"type:uuid;not null;index:idx_logs_plan_created,priority:1" json:"plan_id"`
	Status      string    `gorm:"type:varchar(20);not null" json:"status"` // success, failed, timeout
	DurationMs  int       `gorm:"not null" json:"duration_ms"`
	StepResults JSON      `gorm:"type:jsonb;default:'[]'" json:"step_results"`
	Error       string    `gorm:"type:text" json:"error"`
	CreatedAt   time.Time `gorm:"index:idx_logs_plan_created,priority:2" json:"created_at"`
}

func (e *ExecLog) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// StartedAt returns CreatedAt for template compatibility.
func (e *ExecLog) StartedAt() time.Time {
	return e.CreatedAt
}

// Duration returns a formatted duration string for template display.
func (e *ExecLog) Duration() string {
	if e.DurationMs < 1000 {
		return fmt.Sprintf("%dms", e.DurationMs)
	}
	return fmt.Sprintf("%.1fs", float64(e.DurationMs)/1000)
}

// Trigger returns the execution trigger type.
func (e *ExecLog) Trigger() string {
	return "cron"
}

// StepResultEntry is a parsed step result for template rendering.
type StepResultEntry struct {
	StepName        string  `json:"name"`
	StepType        string  `json:"type"`
	Status          string  `json:"status"`
	DurationMs      int64   `json:"duration_ms"`
	Error           string  `json:"error"`
	Duration        string  `json:"-"`
	DurationPercent float64 `json:"-"`
}

// StepResultsPrettyJSON returns the step_results as a pretty-printed JSON string for raw display.
func (e *ExecLog) StepResultsPrettyJSON() string {
	if len(e.StepResults) == 0 {
		return "[]"
	}
	var parsed json.RawMessage
	if err := json.Unmarshal(e.StepResults, &parsed); err != nil {
		return string(e.StepResults)
	}
	pretty, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return string(e.StepResults)
	}
	return string(pretty)
}

// ParsedStepResults returns step results as a typed slice for template iteration.
func (e *ExecLog) ParsedStepResults() []StepResultEntry {
	if len(e.StepResults) == 0 {
		return nil
	}
	var results []StepResultEntry
	if err := json.Unmarshal(e.StepResults, &results); err != nil {
		return nil
	}

	// Compute display fields
	var maxDur int64
	for _, r := range results {
		if r.DurationMs > maxDur {
			maxDur = r.DurationMs
		}
	}
	for i := range results {
		if results[i].DurationMs < 1000 {
			results[i].Duration = fmt.Sprintf("%dms", results[i].DurationMs)
		} else {
			results[i].Duration = fmt.Sprintf("%.1fs", float64(results[i].DurationMs)/1000)
		}
		if maxDur > 0 {
			results[i].DurationPercent = float64(results[i].DurationMs) / float64(maxDur) * 100
		}
	}
	return results
}
