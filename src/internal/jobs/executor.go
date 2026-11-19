// Package jobs defines asynchronous execution and worker-facing contracts.
package jobs

import (
	"context"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
)

type Executor interface {
	Execute(ctx context.Context, ref contracts.ExecutionRef) (contracts.ExecutionResult, error)

	Inspect(ctx context.Context, ref contracts.ExecutionRef) (contracts.ExecutionResult, error)
}
