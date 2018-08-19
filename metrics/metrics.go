package metrics

import (
	"github.com/rchampourlier/kaizenizer/store"
)

// Generator is the interface for the generators of metrics.
type Generator interface {
	// Generate generates the metrics from the events received
	// through the `events` chan and write them using
	// `PGStore.WriteMetric(..)`
	Generate(events chan store.Event, segmentPrefix string, s *store.PGStore)
}
