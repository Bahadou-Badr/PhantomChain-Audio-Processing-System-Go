package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var registerOnce sync.Once

var (
	JobsProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goaudio_jobs_processed_total",
			Help: "Total number of processed jobs",
		}, []string{"status", "type"},
	)
	JobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "goaudio_job_duration_seconds",
			Help:    "Job processing time in seconds",
			Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
		}, []string{"type"},
	)
	JobFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "goaudio_job_failures_total",
			Help: "Total job failures",
		},
	)
	CurrentJobs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "goaudio_current_jobs",
			Help: "Number of jobs currently being processed",
		},
	)
	HTTPRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goaudio_http_requests_total",
			Help: "HTTP requests processed",
		}, []string{"path", "method", "status"},
	)
)

// Ensure internal/metrics.Register() has sync.Once guard
func Register() {
	registerOnce.Do(func() {
		prometheus.MustRegister(JobsProcessed, JobDuration, JobFailures, CurrentJobs, HTTPRequests)
	})
}
