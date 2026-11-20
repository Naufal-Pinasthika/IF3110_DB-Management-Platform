package jobs

import "github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/config"

// Options reuses worker configuration without import cycles
type Options = config.JobsConfig

func DefaultOptions() Options { return config.DefaultJobsConfig() }
