package model

import "gorm.io/gorm"

// AutoMigrate runs GORM auto-migration for all models.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Plan{}, &Step{}, &ExecLog{}, &Script{}, &PlanScript{}, &StepScript{})
}
