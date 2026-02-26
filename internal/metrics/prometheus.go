package metrics

import (
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"watchcat/internal/engine"
)

const namespace = "watchcat"

var defaultBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}

// Metrics holds all Prometheus metrics for the application.
type Metrics struct {
	Registry *prometheus.Registry

	// Plan-level
	PlanUp        *prometheus.GaugeVec
	PlanExecTotal *prometheus.CounterVec
	PlanDuration  *prometheus.HistogramVec
	PlanInfo      *prometheus.GaugeVec

	// Step-level
	StepExecTotal     *prometheus.CounterVec
	StepDuration      *prometheus.HistogramVec
	StepHTTPResponses *prometheus.CounterVec
	StepWSUp          *prometheus.GaugeVec

	// Runtime
	PlansRunning    prometheus.Gauge
	PlansRegistered prometheus.Gauge
	PlansEnabled    prometheus.Gauge
	BuildInfo       *prometheus.GaugeVec
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics(version, commit, goVersion string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	factory := promauto.With(reg)

	m := &Metrics{
		Registry: reg,

		// Plan-level metrics
		PlanUp: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "plan_up",
			Help:      "Whether the last execution of a plan was successful (1=success, 0=failure).",
		}, []string{"plan_id", "plan_name"}),

		PlanExecTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "plan_executions_total",
			Help:      "Total number of plan executions.",
		}, []string{"plan_id", "plan_name", "result"}),

		PlanDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "plan_duration_seconds",
			Help:      "Duration of plan executions in seconds.",
			Buckets:   defaultBuckets,
		}, []string{"plan_id", "plan_name"}),

		PlanInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "plan_info",
			Help:      "Static info about plans (always 1). Labels carry metadata.",
		}, []string{"plan_id", "plan_name", "cron", "enabled"}),

		// Step-level metrics
		StepExecTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "step_executions_total",
			Help:      "Total number of step executions.",
		}, []string{"plan_id", "step", "type", "result"}),

		StepDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "step_duration_seconds",
			Help:      "Duration of step executions in seconds.",
			Buckets:   defaultBuckets,
		}, []string{"plan_id", "step", "type"}),

		StepHTTPResponses: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "step_http_responses_total",
			Help:      "Total HTTP responses by status class.",
		}, []string{"plan_id", "step", "status_class"}),

		StepWSUp: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "step_ws_up",
			Help:      "Whether the last WebSocket step was successful (1=success, 0=failure).",
		}, []string{"plan_id", "step"}),

		// Runtime metrics
		PlansRunning: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "plans_running",
			Help:      "Number of plans currently executing.",
		}),

		PlansRegistered: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "plans_registered",
			Help:      "Number of plans registered in the scheduler.",
		}),

		PlansEnabled: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "plans_enabled",
			Help:      "Number of enabled plans in the database.",
		}),

		BuildInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_info",
			Help:      "Build information (always 1).",
		}, []string{"version", "commit", "go_version"}),
	}

	// Set build info
	m.BuildInfo.WithLabelValues(version, commit, goVersion).Set(1)

	return m
}

// RecordPlanExecution updates all metrics after a plan execution completes.
func (m *Metrics) RecordPlanExecution(planID, planName, cronExpr string, enabled bool, result *engine.ExecResult) {
	// Plan up/down
	upVal := float64(0)
	if result.Status == "success" {
		upVal = 1
	}
	m.PlanUp.WithLabelValues(planID, planName).Set(upVal)

	// Execution counter
	m.PlanExecTotal.WithLabelValues(planID, planName, result.Status).Inc()

	// Duration histogram (convert ms to seconds)
	durationSec := float64(result.DurationMs) / 1000.0
	m.PlanDuration.WithLabelValues(planID, planName).Observe(durationSec)

	// Plan info
	m.PlanInfo.WithLabelValues(planID, planName, cronExpr, strconv.FormatBool(enabled)).Set(1)

	// Step-level metrics
	for _, sr := range result.StepResults {
		m.StepExecTotal.WithLabelValues(planID, sr.Name, sr.Type, sr.Status).Inc()

		stepDuration := float64(sr.DurationMs) / 1000.0
		m.StepDuration.WithLabelValues(planID, sr.Name, sr.Type).Observe(stepDuration)

		// HTTP status class tracking
		if sr.Type == "http" && sr.Response != nil {
			if respMap, ok := sr.Response.(map[string]any); ok {
				if statusCode, ok := respMap["status_code"]; ok {
					class := statusClass(statusCode)
					m.StepHTTPResponses.WithLabelValues(planID, sr.Name, class).Inc()
				}
			}
		}

		// WebSocket up/down
		if sr.Type == "ws" {
			wsUp := float64(0)
			if sr.Status == "success" {
				wsUp = 1
			}
			m.StepWSUp.WithLabelValues(planID, sr.Name).Set(wsUp)
		}
	}
}

// IncPlansRunning increments the running plans gauge.
func (m *Metrics) IncPlansRunning() {
	m.PlansRunning.Inc()
}

// DecPlansRunning decrements the running plans gauge.
func (m *Metrics) DecPlansRunning() {
	m.PlansRunning.Dec()
}

// UpdateRegisteredPlans updates the registered and enabled plan gauges.
func (m *Metrics) UpdateRegisteredPlans(registered, enabled int) {
	m.PlansRegistered.Set(float64(registered))
	m.PlansEnabled.Set(float64(enabled))
}

// statusClass converts an HTTP status code to its class string (e.g. "2xx", "5xx").
func statusClass(code any) string {
	var statusInt int
	switch v := code.(type) {
	case float64:
		statusInt = int(v)
	case int:
		statusInt = v
	case int64:
		statusInt = int(v)
	default:
		return "unknown"
	}
	if statusInt >= 100 && statusInt < 600 {
		return fmt.Sprintf("%dxx", statusInt/100)
	}
	return "unknown"
}
