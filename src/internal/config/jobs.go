package config

import (
	"fmt"
	"strconv"
	"time"
)

type JobsConfig struct {
	Concurrency       int
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	LeaseDuration     time.Duration
	ExecutionTimeout  time.Duration
	RetryBaseDelay    time.Duration
	RetryMaxDelay     time.Duration
	MaxAttempts       int
	ShutdownTimeout   time.Duration
}

func DefaultJobsConfig() JobsConfig {
	return JobsConfig{Concurrency: 4, PollInterval: time.Second,
		HeartbeatInterval: 5 * time.Second, LeaseDuration: 30 * time.Second,
		ExecutionTimeout: time.Hour, RetryBaseDelay: time.Second,
		RetryMaxDelay: time.Minute, MaxAttempts: 5, ShutdownTimeout: 30 * time.Second}
}

func (o JobsConfig) Validate() error {
	if o.Concurrency < 1 || o.Concurrency > 128 || o.MaxAttempts < 1 || o.MaxAttempts > 100 {
		return fmt.Errorf("jobs concurrency must be 1..128 and max attempts 1..100")
	}
	if o.PollInterval < time.Millisecond || o.HeartbeatInterval < time.Millisecond || o.LeaseDuration < time.Millisecond ||
		o.ExecutionTimeout < time.Millisecond || o.RetryBaseDelay < time.Millisecond || o.RetryMaxDelay < o.RetryBaseDelay || o.ShutdownTimeout < time.Millisecond {
		return fmt.Errorf("jobs durations must be >= 1ms and retry max delay >= base delay")
	}
	if o.HeartbeatInterval > o.LeaseDuration/3 {
		return fmt.Errorf("jobs lease duration must be at least three heartbeat intervals")
	}
	return nil
}

func loadJobsConfig() (JobsConfig, error) {
	cfg := DefaultJobsConfig()
	for _, setting := range []struct {
		name string
		target *int
		fallback string
	}{
		{"JOBS_CONCURRENCY", &cfg.Concurrency, "4"}, {"JOBS_MAX_ATTEMPTS", &cfg.MaxAttempts, "5"},
	} {
		value, err := strconv.Atoi(envOrDefault(setting.name, setting.fallback))
		if err != nil {
			return cfg, fmt.Errorf("%s must be an integer", setting.name)
		}
		*setting.target = value
	}

	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"JOBS_POLL_INTERVAL", &cfg.PollInterval}, {"JOBS_HEARTBEAT_INTERVAL", &cfg.HeartbeatInterval},
		{"JOBS_LEASE_DURATION", &cfg.LeaseDuration}, {"JOBS_EXECUTION_TIMEOUT", &cfg.ExecutionTimeout},
		{"JOBS_RETRY_BASE_DELAY", &cfg.RetryBaseDelay}, {"JOBS_RETRY_MAX_DELAY", &cfg.RetryMaxDelay},
		{"JOBS_SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
	} {
		value, err := time.ParseDuration(envOrDefault(setting.name, setting.target.String()))
		if err != nil {
			return cfg, fmt.Errorf("%s must be a duration", setting.name)
		}
		*setting.target = value
	}
	
	return cfg, cfg.Validate()
}
