package migration

import (
	"context"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/internal/contracts"
)

type Dispatcher interface {
	Enqueue(ctx context.Context, ref contracts.ExecutionRef) (contracts.JobReceipt, error)
}
