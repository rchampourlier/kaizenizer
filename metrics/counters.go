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
	statuses := make(map[string]string) // issueKey -> status
	counters := make(map[string]int)    // status -> count
	countMetrics := 0
	for evt := range events {
		key := evt.IssueKey
		pushMetric := false
		if _, ok := statuses[key]; !ok {
			// Issue not in the map yet
			counters[evt.ValueTo]++
			statuses[key] = evt.ValueTo
			pushMetric = true
		} else if statuses[key] != evt.ValueTo {
			counters[statuses[key]]--
			statuses[key] = evt.ValueTo
			counters[evt.ValueTo]++
			pushMetric = true
		}
		if pushMetric {
			for status, count := range counters {
				countMetrics++
				s.WriteMetric(store.Metric{
					Time:    evt.Time,
					Name:    fmt.Sprintf("counter/%s", status),
					Segment: segmentPrefix,
					Value:   float64(count),
				})
			}
		}
	}
	log.Printf("[metrics/counters] pushed %d metrics\n",
		countMetrics,
	)
}
