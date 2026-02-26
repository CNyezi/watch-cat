package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"watchcat/internal/model"
	"watchcat/internal/store"
)

type DashboardHandler struct {
	logs  *store.LogStore
	plans *store.PlanStore
}

func NewDashboardHandler(logs *store.LogStore, plans *store.PlanStore) *DashboardHandler {
	return &DashboardHandler{logs: logs, plans: plans}
}

func (h *DashboardHandler) Register(e *echo.Echo) {
	e.GET("/", h.Index)
	e.GET("/dashboard/stats", h.Stats)
	e.GET("/dashboard/failures", h.Failures)
	e.GET("/dashboard/trend", h.Trend)
	e.GET("/dashboard/uptime", h.Uptime)
}

// failureEntry holds a recent failure for dashboard display.
type failureEntry struct {
	LogID     uuid.UUID
	PlanID    uuid.UUID
	PlanName  string
	Status    string
	Error     string
	StartedAt interface{ Format(string) string }
}

// trendPoint represents a single data point in the success rate trend.
type trendPoint struct {
	Time string
	Rate float64
}

// uptimeBarData holds per-plan uptime bar display data.
type uptimeBarData struct {
	PlanID      uuid.UUID
	PlanName    string
	Mode        string
	Entries     []store.StatusEntry
	Buckets     []store.BucketEntry
	SuccessRate float64
}

// Index renders the main dashboard page.
func (h *DashboardHandler) Index(c echo.Context) error {
	stats, err := h.logs.GetDashboardStats()
	if err != nil {
		stats = &store.DashboardStats{RecentFailures: []store.RecentFailure{}}
	}

	failures := buildFailures(stats.RecentFailures)
	trend := h.computeTrend(24 * time.Hour)

	data := map[string]any{
		"Nav":   "dashboard",
		"Title": "仪表盘",
		"Stats": map[string]any{
			"Running":  stats.RunningCount,
			"Success":  stats.SuccessCount,
			"Failed":   stats.FailedCount,
			"Disabled": stats.DisabledCount,
			"Trend":    trend,
		},
		"Failures":   failures,
		"TimeRange":  "24h",
		"UptimeBars": h.buildUptimeBars("recent"),
		"UptimeMode": "recent",
	}
	return c.Render(http.StatusOK, "dashboard", data)
}

// Uptime returns the uptime bars partial for all enabled plans.
func (h *DashboardHandler) Uptime(c echo.Context) error {
	mode := c.QueryParam("mode")
	if mode == "" {
		mode = "recent"
	}

	data := map[string]any{
		"Bars": h.buildUptimeBars(mode),
		"Mode": mode,
	}
	return c.Render(http.StatusOK, "dashboard/_uptime", data)
}

// buildUptimeBars constructs uptime bar display data for all enabled plans.
func (h *DashboardHandler) buildUptimeBars(mode string) []uptimeBarData {
	plans, err := h.plans.List("")
	if err != nil {
		return nil
	}

	enabledPlans := make([]model.Plan, 0)
	for _, p := range plans {
		if p.Enabled {
			enabledPlans = append(enabledPlans, p)
		}
	}

	planIDs := make([]uuid.UUID, len(enabledPlans))
	for i, p := range enabledPlans {
		planIDs[i] = p.ID
	}

	var bars []uptimeBarData

	switch mode {
	case "24h":
		bucketMap, err := h.logs.GetTimeBucketStatuses(planIDs, 24*time.Hour, time.Hour)
		if err != nil {
			return nil
		}
		for _, p := range enabledPlans {
			buckets := bucketMap[p.ID]
			total, success := 0, 0
			for _, b := range buckets {
				total += b.Total
				success += b.SuccessCount
			}
			rate := float64(0)
			if total > 0 {
				rate = float64(success) / float64(total) * 100
			}
			bars = append(bars, uptimeBarData{PlanID: p.ID, PlanName: p.Name, Mode: mode, Buckets: buckets, SuccessRate: rate})
		}
	case "7d":
		bucketMap, err := h.logs.GetTimeBucketStatuses(planIDs, 7*24*time.Hour, 6*time.Hour)
		if err != nil {
			return nil
		}
		for _, p := range enabledPlans {
			buckets := bucketMap[p.ID]
			total, success := 0, 0
			for _, b := range buckets {
				total += b.Total
				success += b.SuccessCount
			}
			rate := float64(0)
			if total > 0 {
				rate = float64(success) / float64(total) * 100
			}
			bars = append(bars, uptimeBarData{PlanID: p.ID, PlanName: p.Name, Mode: mode, Buckets: buckets, SuccessRate: rate})
		}
	default:
		statusMap, err := h.logs.GetRecentStatuses(planIDs, 30)
		if err != nil {
			return nil
		}
		for _, p := range enabledPlans {
			entries := statusMap[p.ID]
			total := len(entries)
			success := 0
			for _, e := range entries {
				if e.Status == "success" {
					success++
				}
			}
			rate := float64(0)
			if total > 0 {
				rate = float64(success) / float64(total) * 100
			}
			bars = append(bars, uptimeBarData{PlanID: p.ID, PlanName: p.Name, Mode: "recent", Entries: entries, SuccessRate: rate})
		}
	}

	return bars
}

// Stats returns the dashboard stats partial for htmx auto-refresh.
func (h *DashboardHandler) Stats(c echo.Context) error {
	stats, err := h.logs.GetDashboardStats()
	if err != nil {
		stats = &store.DashboardStats{RecentFailures: []store.RecentFailure{}}
	}

	data := map[string]any{
		"Running":  stats.RunningCount,
		"Success":  stats.SuccessCount,
		"Failed":   stats.FailedCount,
		"Disabled": stats.DisabledCount,
	}
	return c.Render(http.StatusOK, "dashboard/_stats", data)
}

// Failures returns the recent failures partial for htmx auto-refresh.
func (h *DashboardHandler) Failures(c echo.Context) error {
	stats, err := h.logs.GetDashboardStats()
	if err != nil {
		stats = &store.DashboardStats{RecentFailures: []store.RecentFailure{}}
	}

	data := map[string]any{
		"Failures": buildFailures(stats.RecentFailures),
	}
	return c.Render(http.StatusOK, "dashboard/_failures", data)
}

// Trend returns the success rate trend partial.
func (h *DashboardHandler) Trend(c echo.Context) error {
	rangeStr := c.QueryParam("range")
	var duration time.Duration
	switch rangeStr {
	case "1h":
		duration = time.Hour
	case "6h":
		duration = 6 * time.Hour
	default:
		duration = 24 * time.Hour
	}

	trend := h.computeTrend(duration)

	data := map[string]any{
		"Trend": trend,
	}
	return c.Render(http.StatusOK, "dashboard/_trend", data)
}

// buildFailures converts store.RecentFailure to display-friendly entries.
func buildFailures(src []store.RecentFailure) []failureEntry {
	out := make([]failureEntry, 0, len(src))
	for _, f := range src {
		out = append(out, failureEntry{
			LogID:     f.ID,
			PlanID:    f.PlanID,
			PlanName:  f.PlanName,
			Status:    f.Status,
			Error:     f.Error,
			StartedAt: f.CreatedAt,
		})
	}
	return out
}

// computeTrend calculates success rate over time buckets.
func (h *DashboardHandler) computeTrend(duration time.Duration) []trendPoint {
	var buckets int
	var interval time.Duration
	switch {
	case duration <= time.Hour:
		buckets = 6
		interval = 10 * time.Minute
	case duration <= 6*time.Hour:
		buckets = 6
		interval = time.Hour
	default:
		buckets = 8
		interval = 3 * time.Hour
	}

	now := time.Now()
	points := make([]trendPoint, 0, buckets)
	plans, _ := h.plans.List("")

	for i := buckets - 1; i >= 0; i-- {
		start := now.Add(-time.Duration(i+1) * interval)
		end := now.Add(-time.Duration(i) * interval)

		var total, success int
		for _, p := range plans {
			logs, _, _ := h.logs.ListByPlanID(p.ID, "", 1000, "", &start, &end)
			for _, l := range logs {
				total++
				if l.Status == "success" {
					success++
				}
			}
		}

		rate := float64(0)
		if total > 0 {
			rate = float64(success) / float64(total) * 100
		}

		points = append(points, trendPoint{
			Time: end.Format("15:04"),
			Rate: rate,
		})
	}

	return points
}
