package telemetry

import (
	"go.opentelemetry.io/otel/propagation"
)

type propagationTraceContext = propagation.TraceContext
type mapCarrier = propagation.MapCarrier
type headerCarrier = propagation.HeaderCarrier
