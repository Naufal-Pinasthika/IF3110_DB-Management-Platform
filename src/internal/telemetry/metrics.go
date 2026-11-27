package telemetry

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsRegistry = prometheus.NewRegistry()

	jobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_migration_jobs_total",
		Help: "Total migration jobs by lifecycle event status.",
	}, []string{"status"})

	jobRetriesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_migration_job_retries_total",
		Help: "Total migration job retries scheduled by worker.",
	}, []string{"reason"})

	queueWaitSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "platform_migration_queue_wait_seconds",
		Help:    "Seconds a migration job waited in queue before worker execution started.",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 180},
	})

	lockWaitSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "platform_migration_lock_wait_seconds",
		Help:    "Seconds spent attempting to acquire the per-database migration advisory lock.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"result"})

	migrationDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "platform_migration_duration_seconds",
		Help:    "Execution duration of migration attempts in seconds.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 15, 30, 60, 300},
	}, []string{"mode", "action", "status"})

	batchesProcessedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_migration_batches_processed_total",
		Help: "Total migration batches processed by action and status.",
	}, []string{"action", "status"})

	rowsProcessedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_migration_rows_processed_total",
		Help: "Total rows processed by migration batches.",
	}, []string{"action"})

	poolCollector = &dbPoolCollector{
		providers: make(map[string]func() map[string]sql.DBStats),
	}
	jobStatusCollector = &migrationJobStatusCollector{}
)

func init() {
	metricsRegistry.MustRegister(
		jobsTotal,
		jobRetriesTotal,
		queueWaitSeconds,
		lockWaitSeconds,
		migrationDurationSeconds,
		batchesProcessedTotal,
		rowsProcessedTotal,
		poolCollector,
		jobStatusCollector,
	)

	for _, s := range []string{"enqueued", "succeeded", "failed", "blocked"} {
		jobsTotal.WithLabelValues(s)
	}
	for _, r := range []string{"retry_safe", "retry_resume", "reconcile"} {
		jobRetriesTotal.WithLabelValues(r)
	}
	for _, res := range []string{"acquired", "busy", "error"} {
		lockWaitSeconds.WithLabelValues(res)
	}
	for _, act := range []string{"forward", "rollback"} {
		rowsProcessedTotal.WithLabelValues(act)
		batchesProcessedTotal.WithLabelValues(act, "committed")
		batchesProcessedTotal.WithLabelValues(act, "failed")
		for _, mode := range []string{"regular", "batch"} {
			for _, st := range []string{"succeeded", "failed"} {
				migrationDurationSeconds.WithLabelValues(mode, act, st)
			}
		}
	}
}

func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})
}

func IncJobEnqueued() {
	jobsTotal.WithLabelValues("enqueued").Inc()
}

func IncJobFinished(status string) {
	switch strings.ToLower(status) {
	case "succeeded", "failed", "blocked":
		jobsTotal.WithLabelValues(strings.ToLower(status)).Inc()
	}
}

func IncJobRetry(reason string) {
	r := strings.ToLower(reason)
	switch r {
	case "retry_safe", "retry_resume", "reconcile":
		jobRetriesTotal.WithLabelValues(r).Inc()
	default:
		jobRetriesTotal.WithLabelValues("reconcile").Inc()
	}
}

func ObserveQueueWait(d time.Duration) {
	if d < 0 {
		d = 0
	}
	queueWaitSeconds.Observe(d.Seconds())
}

func ObserveLockWait(d time.Duration, result string) {
	if d < 0 {
		d = 0
	}
	switch result {
	case "acquired", "busy", "error":
		lockWaitSeconds.WithLabelValues(result).Observe(d.Seconds())
	}
}

func ObserveMigrationDuration(mode, action, status string, d time.Duration) {
	if d < 0 {
		d = 0
	}
	m := "regular"
	if strings.EqualFold(mode, "batch") {
		m = "batch"
	}
	a := "forward"
	if strings.EqualFold(action, "rollback") || strings.EqualFold(action, "revert_batch") {
		a = "rollback"
	}
	s := strings.ToLower(status)
	switch s {
	case "succeeded", "failed", "partial", "partially_reverted", "unknown":
	default:
		s = "failed"
	}
	migrationDurationSeconds.WithLabelValues(m, a, s).Observe(d.Seconds())
}

func IncBatchProcessed(action, status string, rows int64) {
	a := "forward"
	if strings.EqualFold(action, "rollback") || strings.EqualFold(action, "revert_batch") {
		a = "rollback"
	}
	s := "committed"
	if strings.EqualFold(status, "failed") {
		s = "failed"
	}
	batchesProcessedTotal.WithLabelValues(a, s).Inc()
	if s == "committed" && rows > 0 {
		rowsProcessedTotal.WithLabelValues(a).Add(float64(rows))
	}
}

func RegisterDBPool(name string, pool *sql.DB) {
	if pool == nil {
		return
	}
	RegisterDBPoolProvider(name, func() map[string]sql.DBStats {
		return map[string]sql.DBStats{name: pool.Stats()}
	})
}

func RegisterDBPoolProvider(key string, fn func() map[string]sql.DBStats) {
	poolCollector.mu.Lock()
	defer poolCollector.mu.Unlock()
	if fn == nil {
		delete(poolCollector.providers, key)
		return
	}
	poolCollector.providers[key] = fn
}

func RegisterJobStatusCounter(fn func(context.Context) (map[string]int64, error)) {
	jobStatusCollector.mu.Lock()
	defer jobStatusCollector.mu.Unlock()
	jobStatusCollector.queryFn = fn
}

type dbPoolCollector struct {
	mu        sync.RWMutex
	providers map[string]func() map[string]sql.DBStats
}

var (
	dbPoolConnsDesc = prometheus.NewDesc(
		"platform_db_pool_connections",
		"Current database connection pool counts by pool role and state.",
		[]string{"pool", "state"}, nil,
	)
	dbPoolWaitCountDesc = prometheus.NewDesc(
		"platform_db_pool_wait_count_total",
		"Total number of connections waited for by pool role.",
		[]string{"pool"}, nil,
	)
	dbPoolWaitDurationDesc = prometheus.NewDesc(
		"platform_db_pool_wait_duration_seconds_total",
		"Total time blocked waiting for a new database connection in seconds.",
		[]string{"pool"}, nil,
	)
)

func (c *dbPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- dbPoolConnsDesc
	ch <- dbPoolWaitCountDesc
	ch <- dbPoolWaitDurationDesc
}

func (c *dbPoolCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	providers := make([]func() map[string]sql.DBStats, 0, len(c.providers))
	for _, fn := range c.providers {
		providers = append(providers, fn)
	}
	c.mu.RUnlock()

	aggregated := make(map[string]sql.DBStats)
	for _, fn := range providers {
		for poolName, st := range fn() {
			cur := aggregated[poolName]
			cur.MaxOpenConnections += st.MaxOpenConnections
			cur.OpenConnections += st.OpenConnections
			cur.InUse += st.InUse
			cur.Idle += st.Idle
			cur.WaitCount += st.WaitCount
			cur.WaitDuration += st.WaitDuration
			aggregated[poolName] = cur
		}
	}

	for poolName, st := range aggregated {
		ch <- prometheus.MustNewConstMetric(dbPoolConnsDesc, prometheus.GaugeValue, float64(st.InUse), poolName, "in_use")
		ch <- prometheus.MustNewConstMetric(dbPoolConnsDesc, prometheus.GaugeValue, float64(st.Idle), poolName, "idle")
		ch <- prometheus.MustNewConstMetric(dbPoolConnsDesc, prometheus.GaugeValue, float64(st.OpenConnections), poolName, "open")
		ch <- prometheus.MustNewConstMetric(dbPoolConnsDesc, prometheus.GaugeValue, float64(st.MaxOpenConnections), poolName, "max_open")
		ch <- prometheus.MustNewConstMetric(dbPoolWaitCountDesc, prometheus.CounterValue, float64(st.WaitCount), poolName)
		ch <- prometheus.MustNewConstMetric(dbPoolWaitDurationDesc, prometheus.CounterValue, st.WaitDuration.Seconds(), poolName)
	}
}

type migrationJobStatusCollector struct {
	mu      sync.RWMutex
	queryFn func(context.Context) (map[string]int64, error)
}

var jobStatusGaugeDesc = prometheus.NewDesc(
	"platform_migration_jobs_by_status",
	"Current number of migration jobs in each queue status.",
	[]string{"status"}, nil,
)

var allJobStatuses = []string{"PENDING", "RUNNING", "RETRY_WAIT", "SUCCEEDED", "FAILED", "BLOCKED"}

func (c *migrationJobStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- jobStatusGaugeDesc
}

func (c *migrationJobStatusCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	fn := c.queryFn
	c.mu.RUnlock()

	counts := make(map[string]int64, len(allJobStatuses))
	if fn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if res, err := fn(ctx); err == nil {
			for k, v := range res {
				counts[k] = v
			}
		}
	}

	for _, status := range allJobStatuses {
		ch <- prometheus.MustNewConstMetric(jobStatusGaugeDesc, prometheus.GaugeValue, float64(counts[status]), status)
	}
}
