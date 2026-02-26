package engine

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"watchcat/internal/model"
	"watchcat/internal/store"
)

// SchedulerMetrics is the subset of metrics the scheduler needs.
type SchedulerMetrics interface {
	RecordPlanExecution(planID, planName, cronExpr string, enabled bool, result *ExecResult)
	UpdateRegisteredPlans(registered, enabled int)
}

// Scheduler manages cron-based execution of plans.
type Scheduler struct {
	cron      *cron.Cron
	runner    *Runner
	planStore *store.PlanStore
	logStore  *store.LogStore
	metrics   SchedulerMetrics
	entries   map[uuid.UUID]cron.EntryID // plan ID → cron entry ID
	mu        sync.Mutex
}

// NewScheduler creates a new Scheduler with second-level precision.
func NewScheduler(runner *Runner, planStore *store.PlanStore, logStore *store.LogStore) *Scheduler {
	return &Scheduler{
		cron:      cron.New(cron.WithSeconds()),
		runner:    runner,
		planStore: planStore,
		logStore:  logStore,
		entries:   make(map[uuid.UUID]cron.EntryID),
	}
}

// SetMetrics attaches metrics to the scheduler.
func (s *Scheduler) SetMetrics(m SchedulerMetrics) {
	s.metrics = m
}

// Start loads all enabled plans from DB, registers cron jobs, and starts the scheduler.
func (s *Scheduler) Start() error {
	plans, err := s.planStore.ListEnabled()
	if err != nil {
		return err
	}
	for _, plan := range plans {
		if err := s.addPlan(plan); err != nil {
			log.Printf("[scheduler] failed to register plan %s (%s): %v", plan.Name, plan.ID, err)
		}
	}
	s.cron.Start()
	log.Printf("[scheduler] started with %d plans registered", len(s.entries))
	s.updateMetrics()
	return nil
}

// addPlan registers a single plan's cron job.
func (s *Scheduler) addPlan(plan model.Plan) error {
	planID := plan.ID
	entryID, err := s.cron.AddFunc(plan.Cron, func() {
		s.executePlan(planID)
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.entries[planID] = entryID
	s.mu.Unlock()
	return nil
}

// executePlan fetches the latest plan from DB and runs it, recording results to exec_logs.
func (s *Scheduler) executePlan(planID uuid.UUID) {
	plan, err := s.planStore.GetByID(planID)
	if err != nil {
		log.Printf("[scheduler] failed to load plan %s: %v", planID, err)
		return
	}
	if plan == nil || !plan.Enabled {
		return
	}

	result := s.runner.Execute(context.Background(), plan, plan.Steps, nil)
	TrimResultsIfNeeded(result, plan.DetailLogThreshold)

	stepResultsJSON, _ := json.Marshal(result.StepResults)
	execLog := &model.ExecLog{
		PlanID:      plan.ID,
		Status:      result.Status,
		DurationMs:  int(result.DurationMs),
		StepResults: model.JSON(stepResultsJSON),
		Error:       result.Error,
	}
	if err := s.logStore.Create(execLog); err != nil {
		log.Printf("[scheduler] failed to save exec log for plan %s: %v", planID, err)
	}

	// Record metrics
	if s.metrics != nil {
		s.metrics.RecordPlanExecution(plan.ID.String(), plan.Name, plan.Cron, plan.Enabled, result)
	}
}

// ReloadPlan removes the old cron entry for a plan and re-registers it if still enabled.
func (s *Scheduler) ReloadPlan(planID uuid.UUID) error {
	s.RemovePlan(planID)

	plan, err := s.planStore.GetByID(planID)
	if err != nil {
		return err
	}
	if plan != nil && plan.Enabled {
		if err := s.addPlan(*plan); err != nil {
			return err
		}
	}
	s.updateMetrics()
	return nil
}

// RemovePlan removes the cron entry for a plan.
func (s *Scheduler) RemovePlan(planID uuid.UUID) {
	s.mu.Lock()
	if entryID, ok := s.entries[planID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, planID)
	}
	s.mu.Unlock()
}

// Stop gracefully stops the scheduler, waiting for running jobs to finish.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("[scheduler] stopped")
}

// updateMetrics syncs registered/enabled plan counts to the metrics system.
func (s *Scheduler) updateMetrics() {
	if s.metrics == nil {
		return
	}
	s.metrics.UpdateRegisteredPlans(s.RunningCount(), s.EnabledCount())
}

// RunningCount returns the number of currently registered cron entries.
func (s *Scheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// EnabledCount queries the DB for the count of enabled plans.
func (s *Scheduler) EnabledCount() int {
	plans, err := s.planStore.ListEnabled()
	if err != nil {
		return 0
	}
	return len(plans)
}
