package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/rchampourlier/kaizenizer/store"
)

// CycleTime implements `Generator` for the _Cycle Time_
// metric.
type CycleTime struct{}

// Generate generates _Cycle Time_ metrics.
func (g CycleTime) Generate(events chan store.Event, segmentPrefix string, s *store.PGStore) {
	// m is a map issueKey -> {start, end of cycle time}
	type bounds struct {
		wipSeen    bool
		doneSeen   bool
		start, end time.Time
	}
	m := make(map[string]bounds)

	var countIssues, countEvts, countMetrics int

	for evt := range events {
		countEvts++
		if _, ok := m[evt.IssueKey]; !ok {
			// Issue has not been seen yet
			// Setting `start` in case the issue does not go through
			// a `wip` status
			countIssues++
			m[evt.IssueKey] = bounds{
				wipSeen:  false,
				doneSeen: false,
				start:    evt.Time,
			}
		} else if !m[evt.IssueKey].wipSeen && evt.ValueTo == "wip" {
			// Issue has been seen but `wip` not yet
			// (we don't want to override `start` if the issue
			// goes through `wip` several times)
			m[evt.IssueKey] = bounds{
				wipSeen:  true,
				doneSeen: false,
				start:    evt.Time,
			}
		} else if !m[evt.IssueKey].doneSeen && evt.ValueTo == "done" {
			b := bounds{
				wipSeen:  m[evt.IssueKey].wipSeen,
				doneSeen: true,
				start:    m[evt.IssueKey].start,
				end:      evt.Time,
			}
			m[evt.IssueKey] = b
			segment := fmt.Sprintf("%s/%s", segmentPrefix, evt.IssueKey)
			cycleTime := b.end.Sub(b.start) / (24 * time.Hour)
			t := evt.Time
			name := "cycle_time"
			value := float64(cycleTime)
			countMetrics++
			s.WriteMetric(store.Metric{
				Time:    t,
				Name:    name,
				Segment: segment,
				Value:   value,
			})
		}
	}
	log.Printf("[metrics/cycletime] pushed %d metrics (for %d issues)\n",
		countMetrics,
		countIssues,
	)
}
