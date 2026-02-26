package store

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"watchcat/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const defaultPageSize = 20

type LogStore struct {
	db *gorm.DB
}

func NewLogStore(db *gorm.DB) *LogStore {
	return &LogStore{db: db}
}

func (s *LogStore) Create(log *model.ExecLog) error {
	return s.db.Create(log).Error
}

func (s *LogStore) GetByID(id uuid.UUID) (*model.ExecLog, error) {
	var log model.ExecLog
	err := s.db.First(&log, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// encodeCursor encodes a (time, uuid) pair into a base64 cursor string.
func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%s|%s", t.UTC().Format(time.RFC3339Nano), id.String())
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

// parseCursor decodes a base64 cursor string into a (time, uuid) pair.
func parseCursor(cursor string) (time.Time, uuid.UUID, error) {
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	parts := strings.SplitN(string(data), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor time: %w", err)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("invalid cursor uuid: %w", err)
	}
	return t, id, nil
}

// ListByPlanID returns exec logs for a plan with cursor-based pagination.
// cursor: empty string for the first page.
// limit: page size, defaults to 50 if <= 0.
// status: filter by status if non-empty.
// from/to: optional time range filter.
// Returns (logs, nextCursor, error). nextCursor is empty if no more pages.
func (s *LogStore) ListByPlanID(planID uuid.UUID, cursor string, limit int, status string, from, to *time.Time) ([]model.ExecLog, string, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}

	q := s.db.Where("plan_id = ?", planID)

	if status != "" {
		q = q.Where("status = ?", status)
	}
	if from != nil {
		q = q.Where("created_at >= ?", *from)
	}
	if to != nil {
		q = q.Where("created_at <= ?", *to)
	}

	if cursor != "" {
		cursorTime, cursorID, err := parseCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		q = q.Where("(created_at, id) < (?, ?)", cursorTime, cursorID)
	}

	var logs []model.ExecLog
	err := q.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&logs).Error
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(logs) > limit {
		last := logs[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		logs = logs[:limit]
	}

	return logs, nextCursor, nil
}

// ListFailed returns failed/timeout logs across all plans with cursor-based pagination.
// planID: optional filter by specific plan.
// from/to: optional time range filter.
func (s *LogStore) ListFailed(cursor string, limit int, planID *uuid.UUID, from, to *time.Time) ([]model.ExecLog, string, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}

	q := s.db.Where("status IN ?", []string{"failed", "timeout"})

	if planID != nil {
		q = q.Where("plan_id = ?", *planID)
	}
	if from != nil {
		q = q.Where("created_at >= ?", *from)
	}
	if to != nil {
		q = q.Where("created_at <= ?", *to)
	}

	if cursor != "" {
		cursorTime, cursorID, err := parseCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		q = q.Where("(created_at, id) < (?, ?)", cursorTime, cursorID)
	}

	var logs []model.ExecLog
	err := q.Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&logs).Error
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(logs) > limit {
		last := logs[limit-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
		logs = logs[:limit]
	}

	return logs, nextCursor, nil
}

// DashboardStats holds summary statistics for the dashboard.
type DashboardStats struct {
	RunningCount   int             `json:"running_count"`
	SuccessCount   int             `json:"success_count"`
	FailedCount    int             `json:"failed_count"`
	DisabledCount  int             `json:"disabled_count"`
	RecentFailures []RecentFailure `json:"recent_failures"`
}

// RecentFailure represents a recent failed execution for the dashboard.
type RecentFailure struct {
	ID        uuid.UUID `json:"id" gorm:"column:id"`
	PlanID    uuid.UUID `json:"plan_id"`
	PlanName  string    `json:"plan_name"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *LogStore) GetDashboardStats() (*DashboardStats, error) {
	stats := &DashboardStats{}
	since := time.Now().Add(-24 * time.Hour)

	// Enabled plans = running
	var enabledCount int64
	if err := s.db.Model(&model.Plan{}).Where("enabled = ?", true).Count(&enabledCount).Error; err != nil {
		return nil, err
	}
	stats.RunningCount = int(enabledCount)

	// Disabled plans
	var disabledCount int64
	if err := s.db.Model(&model.Plan{}).Where("enabled = ?", false).Count(&disabledCount).Error; err != nil {
		return nil, err
	}
	stats.DisabledCount = int(disabledCount)

	// Distinct plans with at least one success in the last 24h
	var successCount int64
	if err := s.db.Model(&model.ExecLog{}).
		Where("created_at >= ? AND status = ?", since, "success").
		Distinct("plan_id").
		Count(&successCount).Error; err != nil {
		return nil, err
	}
	stats.SuccessCount = int(successCount)

	// Distinct plans with at least one failure in the last 24h
	var failedCount int64
	if err := s.db.Model(&model.ExecLog{}).
		Where("created_at >= ? AND status IN ?", since, []string{"failed", "timeout"}).
		Distinct("plan_id").
		Count(&failedCount).Error; err != nil {
		return nil, err
	}
	stats.FailedCount = int(failedCount)

	// Recent failures (last 10) with plan name via JOIN
	var failures []RecentFailure
	err := s.db.Model(&model.ExecLog{}).
		Select("exec_logs.id, exec_logs.plan_id, plans.name as plan_name, exec_logs.status, exec_logs.error, exec_logs.created_at").
		Joins("JOIN plans ON plans.id = exec_logs.plan_id").
		Where("exec_logs.status IN ? AND exec_logs.created_at >= ?", []string{"failed", "timeout"}, since).
		Order("exec_logs.created_at DESC").
		Limit(10).
		Scan(&failures).Error
	if err != nil {
		return nil, err
	}
	stats.RecentFailures = failures
	if stats.RecentFailures == nil {
		stats.RecentFailures = []RecentFailure{}
	}

	return stats, nil
}

// FillLastRunStatus populates the LastRunStatus field on each plan
// by querying the most recent exec log status per plan_id.
func (s *LogStore) FillLastRunStatus(plans []model.Plan) {
	if len(plans) == 0 {
		return
	}

	ids := make([]uuid.UUID, len(plans))
	for i := range plans {
		ids[i] = plans[i].ID
	}

	// Use a subquery to get the latest log per plan_id
	type result struct {
		PlanID uuid.UUID `gorm:"column:plan_id"`
		Status string    `gorm:"column:status"`
	}

	var results []result
	// For each plan, get the status of the log with the max created_at.
	// Using DISTINCT ON (PostgreSQL) for efficiency.
	err := s.db.Raw(`
		SELECT DISTINCT ON (plan_id) plan_id, status
		FROM exec_logs
		WHERE plan_id IN ?
		ORDER BY plan_id, created_at DESC
	`, ids).Scan(&results).Error
	if err != nil {
		return
	}

	statusMap := make(map[uuid.UUID]string, len(results))
	for _, r := range results {
		statusMap[r.PlanID] = r.Status
	}

	for i := range plans {
		if st, ok := statusMap[plans[i].ID]; ok {
			plans[i].LastRunStatus = st
		}
	}
}

// CleanupOlderThan deletes exec logs older than the given number of days.
// Returns the number of deleted rows.
func (s *LogStore) CleanupOlderThan(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	result := s.db.Where("created_at < ?", cutoff).Delete(&model.ExecLog{})
	return result.RowsAffected, result.Error
}

// StatusEntry represents a single execution status for uptime bar display.
type StatusEntry struct {
	ID         uuid.UUID `json:"id" gorm:"column:id"`
	Status     string    `json:"status" gorm:"column:status"`
	DurationMs int       `json:"duration_ms" gorm:"column:duration_ms"`
	Error      string    `json:"error" gorm:"column:error"`
	CreatedAt  time.Time `json:"created_at" gorm:"column:created_at"`
}

// GetRecentStatuses returns the most recent `limit` execution statuses for each plan.
// Results are ordered from oldest to newest (left-to-right for display).
func (s *LogStore) GetRecentStatuses(planIDs []uuid.UUID, limit int) (map[uuid.UUID][]StatusEntry, error) {
	if len(planIDs) == 0 {
		return map[uuid.UUID][]StatusEntry{}, nil
	}
	if limit <= 0 {
		limit = 30
	}

	// Use a lateral join to get the top N rows per plan efficiently.
	// Fallback: simple query per plan (acceptable for small plan counts).
	result := make(map[uuid.UUID][]StatusEntry, len(planIDs))
	for _, pid := range planIDs {
		var entries []StatusEntry
		err := s.db.Model(&model.ExecLog{}).
			Select("id, status, duration_ms, error, created_at").
			Where("plan_id = ?", pid).
			Order("created_at DESC").
			Limit(limit).
			Find(&entries).Error
		if err != nil {
			return nil, err
		}
		// Reverse to oldest-first for left-to-right display
		for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
			entries[i], entries[j] = entries[j], entries[i]
		}
		result[pid] = entries
	}
	return result, nil
}

// BucketEntry represents an aggregated time bucket for uptime bar display.
type BucketEntry struct {
	BucketStart  time.Time `json:"bucket_start"`
	Total        int       `json:"total"`
	SuccessCount int       `json:"success_count"`
	Rate         float64   `json:"rate"` // 0-1
}

// GetTimeBucketStatuses aggregates execution statuses into time buckets for each plan.
// duration: total time range (e.g., 24h, 7d)
// bucketSize: size of each bucket (e.g., 1h, 6h)
func (s *LogStore) GetTimeBucketStatuses(planIDs []uuid.UUID, duration, bucketSize time.Duration) (map[uuid.UUID][]BucketEntry, error) {
	if len(planIDs) == 0 {
		return map[uuid.UUID][]BucketEntry{}, nil
	}

	now := time.Now()
	start := now.Add(-duration)
	bucketCount := int(duration / bucketSize)

	result := make(map[uuid.UUID][]BucketEntry, len(planIDs))

	for _, pid := range planIDs {
		var logs []model.ExecLog
		err := s.db.
			Select("status, created_at").
			Where("plan_id = ? AND created_at >= ?", pid, start).
			Order("created_at ASC").
			Find(&logs).Error
		if err != nil {
			return nil, err
		}

		buckets := make([]BucketEntry, bucketCount)
		for i := range buckets {
			buckets[i].BucketStart = start.Add(time.Duration(i) * bucketSize)
		}

		for _, l := range logs {
			idx := int(l.CreatedAt.Sub(start) / bucketSize)
			if idx < 0 {
				idx = 0
			}
			if idx >= bucketCount {
				idx = bucketCount - 1
			}
			buckets[idx].Total++
			if l.Status == "success" {
				buckets[idx].SuccessCount++
			}
		}

		for i := range buckets {
			if buckets[i].Total > 0 {
				buckets[i].Rate = float64(buckets[i].SuccessCount) / float64(buckets[i].Total)
			}
		}

		result[pid] = buckets
	}
	return result, nil
}
