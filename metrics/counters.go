package metrics

import (
	"fmt"
	"log"

	"github.com/rchampourlier/kaizenizer/store"
)

// Counters implements `Generator` for the _Counters_
// metric.
type Counters struct{}

// Generate generates metrics for counts of issues in the different
// statuses.
func (g Counters) Generate(events chan store.Event, segmentPrefix string, s *store.PGStore) {
	statuses := make(map[string]string)         // issueKey -> status
	counters := make(map[string]map[string]int) // statusGroup -> issueTypeGroup -> count
	countMetrics := 0

	for evt := range events {
		key := evt.IssueKey
		pushMetric := false

		// Initialize the map for the event's issue type if
		// it does not exist yet
		if _, counterExists := counters[evt.IssueType]; !counterExists {
			counters[evt.IssueType] = make(map[string]int)
		}

		if _, ok := statuses[key]; !ok {
			// Issue not in the map yet
			counters[evt.IssueType][evt.ValueTo]++
			statuses[key] = evt.ValueTo
			pushMetric = true

		} else if statuses[key] != evt.ValueTo {
			counters[evt.IssueType][statuses[key]]--
			statuses[key] = evt.ValueTo
			counters[evt.IssueType][evt.ValueTo]++
			pushMetric = true
		}

		if pushMetric {
			for statusGroup, types := range counters {
				for issueTypeGroup, count := range types {
					countMetrics++
					s.WriteMetric(store.Metric{
						Time:    evt.Time,
						Name:    "counter",
						Segment: fmt.Sprintf("%s/%s/%s", segmentPrefix, statusGroup, issueTypeGroup),
						Value:   float64(count),
					})
				}
			}
		}
	}
	log.Printf("[metrics/counters] pushed %d metrics\n",
		countMetrics,
	)
}
